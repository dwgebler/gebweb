# WebSockets and SSE

<!-- doctest:strict -->

Two ways to push data to clients:

- **Server-Sent Events (SSE)** stream events from server to browser over an
  ordinary HTTP response. The browser's `EventSource` reconnects on its own.
  Use it for one-way feeds: notifications, progress, live dashboards.
- **WebSockets** keep a two-way connection open. Use them when the client
  also sends messages: chat, collaborative editing, games.

## Server-Sent Events

A route marked `@Sse` returns a list of event frames, built with `web.sse`,
and Gebweb sends them as a `text/event-stream` response:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import web.sse as sse;

@Controller("/jobs")
class JobEvents {
    @Get("/{id}/events")
    @Sse
    func events(string id): list<string> {
        return [
            sse.named("progress", "50"),
            sse.event("done", "job ${id} finished", {"id": "2"}),
        ];
    }
}

let r = gebweb.TestClient(gebweb.app()).get("/jobs/42/events");
io.println(r.headers["Content-Type"]);
io.print(r.text());
```

Output:

```text
text/event-stream; charset=utf-8
event: progress
data: 50

id: 2
event: done
data: job 42 finished

```

| Helper | Frame |
|--------|-------|
| `sse.data(body)` | `data: body` |
| `sse.named(name, body)` | `event: name` and `data: body` |
| `sse.event(name, body, {"id": ..., "retry": ...})` | an event with an id and a reconnect delay |
| `sse.comment(text)` | `: text`, ignored by clients; useful as a keep-alive |
| `sse.retry(ms)` | `retry: ms`, the client's reconnect delay |

`@Sse` sends the frames it is given and ends the response, which suits a
snapshot of recent events. For a feed that stays open and pushes events as
they happen, stream them yourself:

<!-- doctest:fragment context=controller -->
```gb
@Get("/ticks")
func ticks(): dict<string, any> {
    return this.stream(func(int handle): void {
        for (i in 1..4) {
            http.streamWrite(handle, "data: tick ${i}\n\n");
            http.streamFlush(handle);
        }
        http.streamClose(handle);
    }, {"contentType": "text/event-stream; charset=utf-8"});
}
```

A streaming handler holds its connection, and a server slot, until it
returns, so end it when the client disconnects or the feed is done.

## WebSockets

A route marked `@WebSocket` upgrades the connection and calls the handler
with it. The handler owns the conversation: it reads and sends until the
client goes away.

<!-- doctest:fragment context=script -->
```gb
import web.websocket as ws;

@Controller("/ws")
class EchoController {
    @Get("/echo")
    @WebSocket
    func echo(ws.Connection conn): void {
        try {
            while (true) {
                conn.sendText("echo: ${conn.readText()}");
            }
        } catch (IOError e) {
            /* The client disconnected. */
        }
    }
}
```

- The handler takes a single `web.websocket.Connection`; path, query and body
  binding do not apply.
- `readText()` blocks until a message arrives and raises an `IOError` when
  the client disconnects, so end the loop by catching it. An empty string is
  an ordinary message.
- The connection closes when the handler returns.
- Import the module under an alias such as `ws`: the name `websocket` is the
  native module.

| Method | Purpose |
|--------|---------|
| `readText(): string` / `readBytes(): bytes` | Wait for the next text or binary message. |
| `readJson(): any` | Read a text message as JSON. |
| `sendText(string)` / `sendBytes(bytes)` / `sendJson(any)` | Send a message. |
| `close()` | Close the connection; call it once. |

### Limits and timeouts

| Server option | Default | Effect |
|---------------|---------|--------|
| `maxWebSocketMessageBytes` | 1 MiB | A larger incoming message closes the connection; `0` means no limit. |
| `webSocketPingIntervalMs` | 25000 | How often the server pings each client. |
| `webSocketIdleTimeoutMs` | 60000 | A read with no message or pong for this long fails. |
| `webSocketWriteTimeoutMs` | 10000 | A send that cannot complete in this time fails. |

Set them in the `server` options of `gebweb.cli` (or the options of `serve`
and `listen`); `0` disables a timeout. A timeout raises `TimeoutError`, which
is an `IOError`, so the read loops in this chapter end cleanly. Each open
WebSocket holds a server slot for its whole life; see
[Database connections](06b-database-connections.md#server-limit).

## Broadcasting with a hub

A WebSocket handler runs once per connection. To send a message to many
connections, such as everyone in a chat room, keep them in a `gebweb.Hub`.
A hub accepts any subscriber with a `sendText(string)` method:

<!-- doctest:run -->
```gb
import gebweb;
import io;

class Inbox {
    string name;
    list<string> received = [];

    func Inbox(string name) {
        this.name = name;
    }

    func sendText(string message): void {
        this.received.push(message);
    }
}

let hub = gebweb.Hub();
let ada = Inbox("ada");
let lin = Inbox("lin");
let adaSub = hub.join(ada);
hub.join(lin);

hub.broadcast("welcome");
hub.broadcastExcept("ada: hi", adaSub);
hub.leave(adaSub);
hub.broadcast("ada left");

io.println("${hub.size()} connected");
io.println("ada: ${ada.received}");
io.println("lin: ${lin.received}");
```

Output:

```text
1 connected
ada: ["welcome"]
lin: ["welcome", "ada: hi", "ada left"]
```

A chat handler joins its connection to the hub, broadcasts what it reads and
leaves in `finally`. The hub is a shared service, so the container builds one
and gives the same instance to every controller that asks for it:

<!-- doctest:fragment context=script -->
```gb
import web.websocket as ws;

@Controller("/ws")
class ChatController {
    gebweb.Hub hub;

    func ChatController(gebweb.Hub hub) {
        this.hub = hub;
    }

    @Get("/chat")
    @WebSocket
    func chat(ws.Connection conn): void {
        let sub = this.hub.join(conn);
        try {
            while (true) {
                this.hub.broadcastExcept(conn.readText(), sub);
            }
        } catch (IOError e) {
            /* The client disconnected. */
        } finally {
            this.hub.leave(sub);
        }
    }
}
```

`leave` is safe to call twice, and a subscriber whose send fails is dropped
automatically. For several rooms, keep a hub per room in a shared service,
using a `store.Store` so concurrent handlers can create rooms safely.

## Concurrent work in a handler

A WebSocket handler often has other work to do alongside reading: forward
messages from a broker, send heartbeats, push notifications. Run that work in
tasks started with `async.run`, under these rules:

- **Send from any task.** Sends on one connection are serialised, so tasks,
  other handlers and hubs can all send at once.
- **Read from one task.** Keep a single read loop per connection, normally the
  handler itself.
- **Stop the reader by closing.** `conn.close()` from any task makes a waiting
  `readText()` raise, ending the read loop.
- **Clean up in `finally`.** Close the channels you created and await the
  tasks you started, so none outlive the connection.

One slow client can stall everyone: a send blocks once the client's network
buffer is full, until the write timeout fails it, and `hub.broadcast` sends to
subscribers one at a time. For busy fan-out, give each client a bounded
outbox drained by its own writer task, and drop the client when it falls too
far behind. The outbox has a `sendText` method, so it joins the hub in place
of the connection:

<!-- doctest:fragment context=script -->
```gb
import async;
import async.channel as ch;
import web.websocket as ws;

class ClientOutbox {
    ws.Connection conn;
    ch.Channel<string> queue;

    func ClientOutbox(ws.Connection conn) {
        this.conn = conn;
        this.queue = ch.Channel<string>(64);
    }

    func sendText(string message): void {
        if (!this.queue.trySend(message)) {
            this.conn.close();
            throw RuntimeError("client is not keeping up");
        }
    }

    func drain(): void {
        try {
            for (message in this.queue) {
                this.conn.sendText(message as string);
            }
        } catch (IOError e) {
            /* The connection closed while sending. */
        }
    }
}

@Controller("/ws")
class FeedController {
    gebweb.Hub hub;

    func FeedController(gebweb.Hub hub) {
        this.hub = hub;
    }

    @Get("/feed")
    @WebSocket
    func feed(ws.Connection conn): void {
        let outbox = ClientOutbox(conn);
        let writer = async.run(func(): void {
            outbox.drain();
        });
        let sub = this.hub.join(outbox);
        try {
            while (true) {
                this.hub.broadcast(conn.readText());
            }
        } catch (IOError e) {
            /* The client disconnected or was dropped. */
        } finally {
            this.hub.leave(sub);
            outbox.queue.close();
            async.await(writer);
        }
    }
}
```

`broadcast` now only queues, so a slow client costs the sender nothing. A
client 64 messages behind has its connection closed: its read loop raises,
its `finally` leaves the hub and stops the writer.

## Testing

`TestClient` runs `@Sse` routes, as above, but it cannot perform a WebSocket
handshake or read a stream as it is written. Test hubs and outboxes directly
with in-memory subscribers, as in the hub example, and test the wire
behaviour against a real server started with `gebweb.listen`, using
`websocket.connect(url)` or `http.fetchStream(url)` as the client.

## Reference

- `@Sse`: the handler returns `list<string>` frames built with `web.sse`.
- `@WebSocket`: the handler takes one `web.websocket.Connection`.
- `this.stream(handler, opts)` / `gebweb.stream(handler, opts)`: write a
  response as it is produced; see [Responses](04-responses.md#streaming).
- `gebweb.Hub`: `join(subscriber)`, `leave(sub)`, `broadcast(message)`,
  `broadcastExcept(message, sub)`, `size()`; also `gebweb.newHub()`.
