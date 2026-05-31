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
        while (true) {
            let msg = conn.readText();
            if (msg == "") { break; }
            conn.sendText("echo: " + msg);
        }
        conn.close();
    }
}
```

The handler must take a single `web.websocket.Connection` parameter.
Normal argument binding is skipped (path / query / body don't apply to
the upgrade dance). The handler runs after the client completes the
upgrade handshake; you own the read/write loop and the close.

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
class ChatController {
    gebweb.Hub hub;
    func ChatController(gebweb.Hub hub) { this.hub = hub; }

    @WebSocket("/ws/chat")
    func chat(websocket.Connection conn): void {
        let sub = this.hub.join(conn);
        try {
            while (true) {
                let msg = conn.readText();
                this.hub.broadcast(msg);
            }
        } catch (RuntimeError e) {
            /* The client disconnected. */
        } finally {
            this.hub.leave(sub);
        }
    }
}

let app = gebweb.app([ChatController]);
gebweb.register(app, gebweb.Hub, func(): gebweb.Hub {
    return gebweb.newHub();
});
```

That's a single shared chat room. For multiple rooms, register
a hub per room (e.g. one hub per `roomId` looked up from a dict)
and call `join` / `broadcast` on the right one.

The hub's other methods:

- `hub.broadcastExcept(msg, sub)` sends to everyone except the
  caller, useful for "echo to others but not me".
- `hub.size()` returns the connected count.
- `hub.leave(sub)` is safe to call more than once.

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
