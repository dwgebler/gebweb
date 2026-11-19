# WebSockets and SSE

Two decorators promote a route to a non-JSON response shape. `@WebSocket`
turns the route into a WebSocket upgrade; `@Sse` turns it into a
buffered `text/event-stream` reply. For push-as-you-go SSE, prefer
`gebweb.stream(handler)` from [Responses](04-responses.md).

## `@WebSocket`

```gb
import web.websocket as ws;

class ChatController {
    @Get("/chat")
    @WebSocket
    func chat(ws.Connection conn): void {
        try {
            while (true) {
                conn.sendText("echo: " + conn.readText());
            }
        } catch (IOError e) {
            /* The client disconnected. */
        }
    }
}
```

The handler must take a single `web.websocket.Connection` parameter.
Normal argument binding is skipped (path / query / body don't apply to
the upgrade dance). The handler runs after the client completes the
upgrade handshake; you own the read/write loop.

`readText()` blocks until a message arrives and raises an `IOError`
when the client disconnects or the connection is closed. An empty
string is an ordinary (empty) message, not an end-of-stream marker, so
end the loop by catching the error rather than testing the message.
The connection is closed when the handler returns.

Aliasing tip: avoid `import web.websocket as websocket;` - the
identifier collides with the native `websocket` module. `as ws`,
`as wsmod`, or any non-colliding alias is safer.

`Connection` exposes:

| Method                  | Purpose                            |
|-------------------------|------------------------------------|
| `readText(): string`    | block until a text frame arrives   |
| `sendText(string)`      | send a text frame                  |
| `readBytes(): bytes`    | block until a binary frame arrives |
| `sendBytes(bytes)`      | send a binary frame                |
| `readJson(): dict<...>` | read a text frame as JSON          |
| `sendJson(any)`         | send a JSON-encoded text frame     |
| `close()`               | close the connection               |
| `echoText()`            | convenience: text echo loop        |

Incoming messages are limited to 1 MiB each. A larger message closes the
connection and `readText()` / `readBytes()` raises an error. Raise or
remove the limit with the `maxWebSocketMessageBytes` server option (`0`
means no limit), passed to `gebweb.serve` / `gebweb.listen` or in
`opts.server`:

```gb
gebweb.serve(app, ":8080", {"maxWebSocketMessageBytes": 8 * 1024 * 1024});
```

The server also pings each connection every 25 seconds, ends a read
that sees no message or pong for 60 seconds, and fails a send that
cannot complete in 10 seconds; each of these raises `TimeoutError`, an
`IOError`, so the read loops in this chapter end cleanly. Tune them with
the `webSocketPingIntervalMs`, `webSocketIdleTimeoutMs` and
`webSocketWriteTimeoutMs` server options (`0` disables one).

## `@Sse`

```gb
import web.sse as sse;

class FeedController {
    @Get("/events")
    @Sse
    func events(): list<string> {
        return [
            sse.data("first"),
            sse.named("ping", "1"),
            sse.data("last"),
        ];
    }
}
```

The handler returns a `list<string>` of pre-formatted SSE frames. The
framework joins them and serves the result with the right headers:
`Content-Type: text/event-stream; charset=utf-8`,
`Cache-Control: no-cache`, `Connection: keep-alive`.

Build frames with `web.sse`:

| Helper                                  | Frame produced |
|-----------------------------------------|----------------|
| `sse.data(body)`                        | `data: <body>` |
| `sse.event(name, body, options)`        | `id: ...\nevent: name\nretry: ...\ndata: body` |
| `sse.named(name, body)`                 | `event: name\ndata: body` |
| `sse.comment(text)`                     | `: <text>` |
| `sse.retry(ms)`                         | `retry: <ms>` |

## Live SSE push

`@Sse` buffers the whole response - fine for short bursts but not
for long-lived feeds. Use `gebweb.stream` for true server-push:

```gb
import gebweb;
import http;
import time;

@Get("/ticks")
func ticks(): dict<string, any> {
    return gebweb.stream(func(int handle): void {
        for (i in 1..1000) {
            http.streamWrite(handle, "data: tick " + (i as string) + "\n\n");
            http.streamFlush(handle);
            time.sleep(1.0);
        }
        http.streamClose(handle);
    }, {"contentType": "text/event-stream; charset=utf-8"});
}
```

## Chat rooms and broadcasts

A WebSocket handler runs once per connection. To send messages
between connections (a chat room, a live notification feed, a
multiplayer-game lobby) you need somewhere to keep track of
who's connected.

`gebweb.Hub` is that "somewhere". Register it once, inject it
into your controller, and use `hub.broadcast(msg)` to send a
message to every connected client:

```gb
import gebweb;
import web.websocket as ws;

class ChatController {
    gebweb.Hub hub;
    func ChatController(gebweb.Hub hub) {
        this.hub = hub;
    }

    @Get("/ws/chat")
    @WebSocket
    func chat(ws.Connection conn): void {
        let sub = this.hub.join(conn);
        try {
            while (true) {
                this.hub.broadcast(conn.readText());
            }
        } catch (IOError e) {
            /* The client disconnected. */
        } finally {
            this.hub.leave(sub);
        }
    }
}

let app = gebweb.app();
gebweb.register(app, gebweb.Hub, func(): gebweb.Hub { return gebweb.newHub(); });
gebweb.addControllers(app, [ChatController]);
```

That's a single shared chat room. For multiple rooms, register
a hub per room (e.g. one hub per `roomId` looked up from a dict)
and call `join` / `broadcast` on the right one.

The hub's other methods:

- `hub.broadcastExcept(msg, sub)` sends to everyone except the
  caller, useful for "echo to others but not me".
- `hub.size()` returns the connected count.
- `hub.leave(sub)` is safe to call more than once.

A connection whose send fails is dropped from the hub automatically.

## Concurrent handlers

A handler often has more to do than answer the client: push
notifications, forward messages from a broker, send a heartbeat. Run
that work in tasks with `async.run` alongside the read loop. The rules:

- **Sends are safe from any task.** `sendText`, `sendBytes` and
  `sendJson` on one connection are serialised, so a task, another
  connection's handler or a hub can all send at once.
- **Reads belong to one task.** Keep a single read loop per connection,
  normally the handler itself. Two tasks reading the same connection at
  once is not supported.
- **Stop the reader by closing the connection.** `conn.close()` from
  any task makes a pending `readText()` raise, which ends the read loop.
  Call it once; a second `close()` raises.
- **Clean up in `finally`.** Close channels and await the tasks you
  started so none outlive the connection.
- **A client that stops reading stalls its senders.** A send blocks
  once the client's network buffer is full, until the write timeout
  (10 seconds by default) fails it, and `hub.broadcast` sends to each
  subscriber in turn from the caller's task, so one stalled client
  holds up the broadcast for everyone.

The last point is why busy fan-out should not send to the connection
directly. Give each client a bounded outbox drained by its own writer
task, and drop the client when the outbox overflows. A hub subscriber
only needs a `sendText(string)` method, so the outbox can join the hub
in place of the connection:

```gb
import async;
import async.channel as ch;
import gebweb;
import web.websocket as ws;

/** Buffers outgoing messages for one client. */
class ClientOutbox {
    ws.Connection conn;
    ch.Channel<string> queue;

    func ClientOutbox(ws.Connection conn) {
        this.conn = conn;
        this.queue = ch.Channel<string>(64);
    }

    func sendText(string msg): void {
        if (!this.queue.trySend(msg)) {
            this.conn.close();
            throw RuntimeError("client is not keeping up");
        }
    }

    func drain(): void {
        try {
            for (msg in this.queue) {
                this.conn.sendText(msg as string);
            }
        } catch (IOError e) {
            /* The connection closed mid-send. */
        }
    }
}

class ChatController {
    gebweb.Hub hub;
    func ChatController(gebweb.Hub hub) {
        this.hub = hub;
    }

    @Get("/ws/chat")
    @WebSocket
    func chat(ws.Connection conn): void {
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

`broadcast` now only queues, so a slow client costs the sender nothing.
When a client falls 64 messages behind, `sendText` closes its
connection: that client's read loop raises, its `finally` block leaves
the hub and stops the writer, and the hub drops it on the failed send.
The same outbox works for any other producer task; send to the outbox,
never to the connection, and the writer stays the only task that
blocks on the client.

## TestClient and streaming

The in-process `TestClient` doesn't drive the WebSocket handshake or
chunked-encoding stream. The dispatcher returns the raw upgrade /
stream dict with the handler still under the `websocket` / `stream`
key. For wire-level testing, use a real `gebweb.serve` / `listen` and
a WebSocket client like the stdlib `websocket.connect(url)`.

## Reference

- `@WebSocket` - handler must take one `web.websocket.Connection`.
- `@Sse` - handler returns `list<string>` of pre-formatted frames.
- `gebweb.stream(handler, opts?): dict<string, any>` - see
  [Responses](04-responses.md).
- `gebweb.Hub` / `gebweb.newHub()` - broadcast registry; `join`,
  `leave`, `broadcast`, `broadcastExcept`, `size`.

`gebweb.streaming` exports the underlying helpers used by the
framework: `isWebSocketRoute(handler): bool`,
`isSseRoute(handler): bool`, `upgradeRoute(handler): dict`,
`sseRoute(frames): dict`. Most apps won't need these directly.
