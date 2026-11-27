# Testing

<!-- doctest:strict -->

`gebweb.TestClient` sends requests through the app's router in the current
process. It exercises binding, dependency injection, validation, auth,
middleware and response formatting without opening a socket. Use your real
app factory and controllers in application tests; replace only the external
collaborators that the test is not meant to exercise.

## A first route test

This self-contained test module defines a small controller so it can be copied
into a new project. In an existing app, import the controller instead of
copying its implementation into the test:

<!-- doctest:file hello_test.gb -->
```gb
import gebweb;
import test;

class Greeting {
    string message;

    func Greeting(string message) {
        this.message = message;
    }
}

@Controller("/hello")
class HelloController {
    @Get("")
    func greet(string name = "Ada"): Greeting {
        return Greeting("Hello, ${name}");
    }
}

class HelloTest extends test.Test {
    gebweb.TestClient client;

    func setUp(): void {
        this.client = gebweb.TestClient(gebweb.app());
    }

    @test
    func greetsTheRequestedName(): void {
        let response = this.client.get("/hello?name=Lin");

        response.assertStatus(200);
        this.assertEquals("Hello, Lin", response.json()["message"]);
    }
}

export func runGreetingExample(): void {
    let example = HelloTest();
    example.setUp();
    example.greetsTheRequestedName();
}
```

Save the test module as `tests/hello_test.gb` and use the test runner:

```sh
geblang test tests/
geblang test --runtime=evaluator tests/
```

`setUp` prepares a fresh app for each test, and assertion failures fail the
case. A case is also an ordinary method: this small smoke-test driver runs the
same assertions and only prints success if they all pass:

<!-- doctest:run -->
```gb
import hello_test;
import io;

hello_test.runGreetingExample();
io.println("greeting test passed");
```

Output:

```text
greeting test passed
```

Use `tearDown` to close resources a case opened. For tests of repository SQL,
use a real SQLite connection, often `:memory:`, or the intended database
service; replacing the database with a mock cannot verify a query.

## Request methods and bodies

| Method | What it sends |
|--------|---------------|
| `get(path)` | GET without a body. |
| `post(path, body)` | POST with the supplied body. |
| `put(path, body)` | PUT with the supplied body. |
| `patch(path, body)` | PATCH with the supplied body. |
| `delete(path)` | DELETE without a body. |
| `request(method, path, body, headers)` | Any method, explicit body and headers. |
| `send(method, path, body, headers)` | The same operation as `request`. |
| `multipart(path, parts, extraHeaders)` | POST with a constructed multipart body; see below. |

Query strings belong in `path`, as in `/hello?name=Lin`. A dict, list or other
non-string/non-bytes body is JSON-encoded, with `Content-Type: application/json`
added if `Content-Type` is not supplied. Strings and bytes are sent unchanged;
set their content type explicitly. A null body becomes an empty request body.
Use `request` for headers on GET or for a DELETE body.

### Client address

`TestClient(app, opts = {})` has one option:

| Option | Type | Default | Effect |
|--------|------|---------|--------|
| `clientIp` | `string` | `127.0.0.1` | Request address, also used to construct `remoteAddr`. |

Use a different client address when testing IP-based policies. This is a
synthetic request address; no connection is made to it:

<!-- doctest:run -->
```gb
import gebweb;
import io;

@Controller("/address")
class AddressController {
    @Get("")
    func show(gebweb.Request request): dict<string, any> {
        return {"ip": request.clientIp()};
    }
}

let app = gebweb.app();
io.println(gebweb.TestClient(app).get("/address").text());
io.println(gebweb.TestClient(app, {"clientIp": "203.0.113.5"}).get("/address").text());
io.println(gebweb.TestClient(app, {"clientIp": "2001:db8::7"}).get("/address").text());
```

Output:

```text
{"ip":"127.0.0.1"}
{"ip":"203.0.113.5"}
{"ip":"2001:db8::7"}
```

Two clients for one app still share its services and middleware state. Build a
new app when a case needs an empty cache, fresh rate-limit buckets or different
configuration. Do not rely on a previous case's app or execution order.

## Replacing collaborators

Register fakes before constructing `TestClient`: the facade performs controller
discovery when it creates the client. An explicit `gebweb.app([Controller])`
list constructs controllers even earlier, during app creation.

For an interface dependency, register its implementation with
`di.registerInterfaceInstance`. Here the fake holds seeded objects and returns
them through the same interface the controller uses:

<!-- doctest:run -->
```gb
import gebweb;
import gebweb.di as di;
import io;

class User {
    string id;
    string name;

    func User(string id, string name) {
        this.id = id;
        this.name = name;
    }
}

interface UserRepository {
    func find(string id): ?User;
}

class MemoryUsers implements UserRepository {
    dict<string, User> users = {};

    func add(User user): void {
        this.users[user.id] = user;
    }

    func find(string id): ?User {
        return this.users[id] ?? null;
    }
}

@Controller("/users")
class UserController extends gebweb.Controller {
    UserRepository users;

    func UserController(UserRepository users) {
        parent();
        this.users = users;
    }

    @Get("/{id}")
    func show(string id): User {
        let user = this.users.find(id);
        if (user == null) {
            this.notFound("user not found");
        }
        return user as User;
    }
}

let users = MemoryUsers();
users.add(User("u1", "Ada"));
let app = gebweb.app();
di.registerInterfaceInstance(app.container, "UserRepository", users);
let client = gebweb.TestClient(app);

io.println(client.get("/users/u1").json()["name"]);
io.println(client.get("/users/missing").status);
```

Output:

```text
Ada
404
```

For a class-typed dependency, use
`gebweb.registerInstance(app, DependencyClass, fakeInstance)` instead. Import
and export the actual classes used by the app; a same-named class from another
module is a different type. See [Dependency injection](06a-dependency-injection.md)
for qualified interface names and scope rules.

Assert on behavior, such as the response or stored record, rather than a
fake's invocation count. A service fake isolates an unavailable external
service; an integration test should still exercise a real or realistic
backend when that backend's behavior is the subject of the test.

## Errors, response shapes and redirects

A test client receives formatted HTTP errors. It does not require a try/catch
around a handler that throws an `HttpException`. Start with the status, then
check the parts of the body and headers that form your API contract:

<!-- doctest:run -->
```gb
import gebweb;
import io;

class NewUser {
    @Assert.email
    string email;
}

@Controller("/users")
class UserController extends gebweb.Controller {
    @Post("")
    func create(NewUser input): gebweb.Response {
        return this.created(input, "/users/u1");
    }

    @Get("/missing")
    func missing(): void {
        this.notFound("user not found");
    }
}

let client = gebweb.TestClient(gebweb.app());
let invalid = client.post("/users", {"email": "not-an-email"});
invalid.assertStatus(422);
io.println(invalid.json()["errors"][0]["field"]);
let missing = client.get("/users/missing");
missing.assertStatus(404);
io.println(missing.json()["detail"]);
let created = client.post("/users", {"email": "ada@example.com"});
created.assertStatus(201);
io.println(created.headers["Location"]);
io.println(created.json()["email"]);
```

Output:

```text
email
user not found
/users/u1
ada@example.com
```

`TestResponse` is a wrapper with these members, not the raw response dict:

| Member | Type | Meaning |
|--------|------|---------|
| `status` | `int` | HTTP response status. |
| `headers` | `dict<string, any>` | Response headers; some values, such as multiple cookies, can be lists. |
| `body` | `any` | Raw body value. |
| `json()` | `any` | Parse a string body as JSON; otherwise return the body unchanged. |
| `text()` | `string` | String body as-is, bytes converted to a string, other values JSON-encoded. |
| `assertStatus(want)` | `void` | Throw `RuntimeError` if the status differs. |

Prefer assertions on meaningful fields to snapshots of an entire response
whose unrelated fields can change. Avoid depending on JSON object key order.

## Cookies and headers

The client neither stores cookies nor follows redirects. Copy the cookie's
`name=value` pair into a later `Cookie` header yourself; do not send attributes
such as `Path` or `HttpOnly` back as request cookies:

<!-- doctest:run -->
```gb
import gebweb;
import io;

@Controller("/preferences")
class PreferenceController extends gebweb.Controller {
    @Post("/dark")
    func chooseDark(): gebweb.Response {
        return this.redirect("/preferences", 303)
            .withHeader("Set-Cookie", "theme=dark; Path=/; HttpOnly; SameSite=Lax");
    }

    @Get("")
    func show(gebweb.Request request): dict<string, any> {
        return {
            "theme": request.cookie("theme") ?? "light",
            "trace": request.header("X-Trace-Id") ?? "none",
        };
    }
}

let client = gebweb.TestClient(gebweb.app());
let redirect = client.post("/preferences/dark", null);
redirect.assertStatus(303);
string cookie = (redirect.headers["Set-Cookie"] as string).split(";")[0];

io.println(redirect.headers["Location"]);
io.println(client.get("/preferences").text());
io.println(
    client.request("GET", "/preferences", null, {"Cookie": cookie, "X-Trace-Id": "test-42"}).text()
);
```

Output:

```text
/preferences
{"theme":"light","trace":"none"}
{"theme":"dark","trace":"test-42"}
```

For multiple `Set-Cookie` values, inspect the list and send the needed
`name=value` pairs joined by `; `. For session authentication, obtain the
cookie from the real session flow rather than inventing a session id. Send
`Authorization` or `X-API-Key` through the same `request` headers for API auth;
see [Auth and permissions](08-auth.md). For a CSRF-protected write, also send
the matching token header or form field; see [CSRF](23-csrf.md).

## Multipart and binary bodies

`multipart(path, parts, extraHeaders)` constructs the boundary and
`Content-Type` header. A string part is a text field; a file part is a dict
with `filename`, `content` and optional `contentType` (default
`application/octet-stream`). File content can be a string or bytes:

<!-- doctest:run -->
```gb
import bytes;
import crypt;
import gebweb;
import io;

class UploadForm {
    string title;
}

class UploadReceipt {
    string title;
    string hash;

    func UploadReceipt(string title, string hash) {
        this.title = title;
        this.hash = hash;
    }
}

@Controller("/uploads")
class UploadController extends gebweb.Controller {
    @Post("")
    func upload(UploadForm form, dict<string, gebweb.UploadedFile> files): UploadReceipt {
        return UploadReceipt(form.title, crypt.sha256(files["file"].bytes()));
    }
}

let client = gebweb.TestClient(gebweb.app());
let content = bytes.fromHex("89504e470d0a1a0aff00");
let response = client.multipart(
    "/uploads",
    {
        "title": "binary fixture",
        "file": {"filename": "image.png", "contentType": "image/png", "content": content},
    },
    {}
);
response.assertStatus(200);
io.println(response.json()["title"]);
io.println(response.json()["hash"] == crypt.sha256(content));
```

Output:

```text
binary fixture
true
```

The hash comparison catches a byte-to-string conversion that could corrupt
non-UTF-8 content. For a raw binary or custom encoded request body, use
`request` with a bytes value and explicit content-type headers. See
[File uploads](12-file-uploads.md) for validation and storage policy.

## What needs a real server

`TestClient` does not exercise TLS, socket disconnects, HTTP chunking, browser
behavior or the WebSocket handshake. A WebSocket upgrade response is returned
without running the WebSocket handler. A live stream handler is not driven;
a buffered `@Sse` return is joined synchronously and can be checked in-process.
File responses are read into the client's body rather than streamed over a
socket.

Keep fast in-process tests for route behavior, real-backend integration tests
for persistence and services, and a few wire-level tests for critical flows.
A wire-level test must own its server, await readiness and close it explicitly;
do not coordinate with sleeps. See [WebSockets and SSE](11-websockets-and-sse.md)
for connection examples.

## Reference

- `gebweb.TestClient(app, opts = {}): TestClient`, where `opts.clientIp` defaults
  to `127.0.0.1`.
- Requests: `get`, `post`, `put`, `patch`, `delete`, `request`, `send`, `multipart`.
- Results: `TestResponse.status`, `headers`, `body`, `json()`, `text()` and
  `assertStatus(want)`.
- Runner: `geblang test tests/`; repeat with `--runtime=evaluator` for parity.
