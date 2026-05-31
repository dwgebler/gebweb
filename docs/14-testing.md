# Testing

Gebweb integrates with the stdlib `test` framework via an in-process
`TestClient`. Dispatching synthetic requests through the same router
the production server uses gives end-to-end coverage without binding
a socket.

## Hello-world test

```gb
import test;
import gebweb;

class HelloController {
    @Get("/")
    func index(): dict<string, any> {
        return {"message": "hello"};
    }
}

class HelloTest extends test.Test {
    gebweb.TestClient client;

    func setUp(): void {
        this.client = gebweb.TestClient(gebweb.app([HelloController()]));
    }

    @test
    func indexReturnsGreeting(): void {
        let r = this.client.get("/");
        r.assertStatus(200);
        this.assertEquals("hello", r.json()["message"]);
    }
}
```

Run with `geblang test tests/`.

## `TestClient` API

| Method                                        | Notes                              |
|-----------------------------------------------|------------------------------------|
| `get(path)`                                   | GET; no body                       |
| `post(path, body)`                            | POST with JSON-ified body          |
| `put(path, body)`                             | PUT                                |
| `patch(path, body)`                           | PATCH                              |
| `delete(path)`                                | DELETE; no body                    |
| `request(method, path, body, headers)`        | full control                       |
| `send(method, path, body, headers)`           | alias of `request`                 |

Bodies that aren't strings or null are JSON-stringified and a default
`Content-Type: application/json` header is added when missing.

## `TestResponse`

The return value of every `TestClient` call:

```gb
class TestResponse {
    int status;
    dict<string, any> headers;
    any body;

    func json(): any;          # parse body as JSON if string
    func text(): string;       # body as string (json.stringify when not already)
    func assertStatus(want);   # raises RuntimeError on mismatch
}
```

Use `r.assertStatus(200)` to fail-fast, then assert on the parsed body
via `r.json()`.

## Stubbing services

Use `gebweb.registerInstance` to inject test doubles into the DI
container before constructing the `TestClient`:

```gb
class WidgetRepoTest extends test.Test {
    WidgetRepo repo;
    gebweb.TestClient client;

    func setUp(): void {
        this.repo = WidgetRepo();
        let app = gebweb.app([Widget]);
        gebweb.registerInstance(app, WidgetRepo, this.repo);
        this.client = gebweb.TestClient(app);
    }

    @test
    func listReturnsSeededWidgets(): void {
        this.repo.save(Widget("w1", "alpha", 10));
        this.repo.save(Widget("w2", "beta", 20));
        let r = this.client.get("/widgets");
        r.assertStatus(200);
        this.assertEquals(2, (r.json()["items"] as list<any>).length());
    }
}
```

The same pattern works for any constructor-injected service: register
an instance before building the `TestClient`, and the framework uses
the stub instead of resolving a fresh one.

## Cookies and headers

For session-auth flows, drive cookies via the `request` overload:

```gb
let id = sessionStore.save({}, {"id": "u-1"}, {"path": "/", "maxAge": 3600})["headers"]["Set-Cookie"]
    .split(";")[0].substring(8);   /* extract "geb_sid=" value */

let r = client.request("GET", "/me", null, {"Cookie": "geb_sid=" + id});
r.assertStatus(200);
```

For JWT flows, set `Authorization`:

```gb
let token = gebweb.jwtIssue("secret", {"sub": "u-1", "roles": ["admin"]}, 3600);
client.request("GET", "/me", null, {"Authorization": "Bearer " + token});
```

## Asserting response shape

`TestResponse` exposes the underlying dict so you can drill in:

```gb
let r = client.post("/users", {"email": "ada"});
r.assertStatus(422);

/* Set-Cookie inspection. */
let headers = r.headers as dict<string, any>;
this.assertTrue(headers.contains("Set-Cookie"));
let cookie = headers["Set-Cookie"] as string;
this.assertTrue(cookie.contains("HttpOnly"));

/* RFC 9457 Problem Details body. */
let body = r.json();
this.assertEquals(422, body["status"]);
this.assertTrue((body["detail"] as string).indexOf("email") >= 0);
let errors = body["errors"] as list<any>;
this.assertEquals("email", (errors[0] as dict<string, any>)["field"]);

/* Custom redirect. */
let red = client.post("/login", {"username": "ada"});
red.assertStatus(303);
this.assertEquals("/dashboard", (red.headers as dict<string, any>)["Location"]);
```

For exception flows, the framework formats the thrown
`HttpException` into a Problem Details body, so assert on
`status`, `title`, and `detail` keys directly without catching
the exception in the test.

## What `TestClient` doesn't do

- It doesn't drive the WebSocket handshake (`@WebSocket` handlers
  return the upgrade dict but the handler isn't invoked).
- It doesn't drive chunked-encoding streams (`gebweb.stream` /
  `@Sse` handlers return the response dict with the `stream` /
  buffered body present, but the handler isn't called for live
  push). The buffered `@Sse` return (a list of pre-formatted frames)
  IS exercised in full because frames are joined synchronously.

For wire-level testing of WebSockets / chunked SSE, start a real
server with `gebweb.listen(app, addr)` and connect via the stdlib
`websocket.connect(url)` or `http.fetchStream(url)`.

## Reference

- `gebweb.TestClient(app): TestClient` - build a client.
- `TestClient.get / post / put / patch / delete / request / send` -
  see the table above.
- `TestResponse.status: int`, `headers: dict<string, any>`,
  `body: any`.
- `TestResponse.json(): any`, `text(): string`,
  `assertStatus(want)`.
