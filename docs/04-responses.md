# Responses

<!-- doctest:strict -->

A handler answers a request in one of three ways:

- **Return data.** Gebweb encodes it as JSON with a 200 status. This is the
  usual answer for a read.
- **Return a response** built with a controller helper such as
  `this.created(...)`, when the status, headers or format matter.
- **Throw an HTTP exception** such as `this.notFound(...)` for an expected
  failure. Gebweb turns it into an RFC 9457 Problem Details response.

## Returning data

<!-- doctest:run -->
```gb
import gebweb;
import io;

class Book {
    int id;
    string title;

    func Book(int id, string title) {
        this.id = id;
        this.title = title;
    }
}

@Controller("/data")
class DataController {
    @Get("/book")
    func book(): Book {
        return Book(1, "Dune");
    }

    @Get("/books")
    func books(): list<Book> {
        return [Book(1, "Dune"), Book(2, "Emma")];
    }

    @Get("/stats")
    func stats(): dict<string, any> {
        return {"status": "ok", "books": 2};
    }

    @Get("/count")
    func count(): int {
        return 2;
    }

    @Get("/nothing")
    func nothing(): void {}
}

let client = gebweb.TestClient(gebweb.app());
for (path in ["/data/book", "/data/books", "/data/stats", "/data/count", "/data/nothing"]) {
    let r = client.get(path);
    io.println("${path} ${r.status} ${r.text()}");
}
```

Output:

```text
/data/book 200 {"id":1,"title":"Dune"}
/data/books 200 [{"id":1,"title":"Dune"},{"id":2,"title":"Emma"}]
/data/stats 200 {"books":2,"status":"ok"}
/data/count 200 2
/data/nothing 204 
```

| Returned value | Response |
|----------------|----------|
| An instance of your class | 200 with the object's fields as JSON, filtered by any `@Groups` on the handler ([Serialization groups](07-serialization-groups.md)) |
| A `list` | 200 with a JSON array |
| A `dict` | 200 with a JSON object |
| A string, number or bool | 200 with that JSON value |
| `null`, or nothing from a `void` handler | 204 with an empty body |
| A response built with a helper | as built |

Declare the real return type, such as `Book` or `list<Book>`, rather than
`dict<string, any>`. The handler stays type-checked, and the OpenAPI document
gets the response schema from it.

## Building a response

Controllers that extend `gebweb.Controller` have helpers that return a
`gebweb.Response`:

| Helper | Response |
|--------|----------|
| `this.json(data, status = 200)` | `data` as JSON with the given status |
| `this.created(data, location)` | 201, `data` as JSON, `Location: location` |
| `this.accepted(data = null)` | 202, with `data` as JSON if given |
| `this.noContent()` | 204, empty body |
| `this.text(body, status = 200)` | `text/plain` |
| `this.html(body, status = 200, headers = null)` | `text/html; charset=utf-8` |
| `this.redirect(location, status = 302)` | a redirect; use 303 after a form post |
| `this.back(request, fallback = "/")` | 303 to the `Referer`, or `fallback` |
| `this.problem(status, title, detail, extras = null)` | a Problem Details response, without throwing |
| `this.view(request, name, ctx, status = 200)` | a rendered template ([Views](04a-views.md)) |
| `this.partial(request, name, ctx)` | a rendered fragment with `Vary: HX-Request` |
| `this.stream(handler, opts = null)` | a streaming response (see below) |

A `gebweb.Response` is immutable. `withStatus`, `withHeader` and `withBody`
return a changed copy, so a helper's result can be adjusted before it is
returned:

<!-- doctest:run -->
```gb
import gebweb;
import io;

class NewInvite {
    @Assert.email
    string email;
}

@Controller("/invites")
class InviteController extends gebweb.Controller {
    @Post("")
    func create(@Body NewInvite input): gebweb.Response {
        return this.created({"id": 7, "email": input.email}, "/invites/7")
            .withHeader("Cache-Control", "no-store")
            .withHeader("Set-Cookie", "invited=1; Path=/; Max-Age=3600; HttpOnly; Secure; SameSite=Lax");
    }

    @Get("/old")
    func old(): gebweb.Response {
        return this.redirect("/invites", 301);
    }
}

let client = gebweb.TestClient(gebweb.app());
let r = client.post("/invites", {"email": "ada@example.com"});
io.println("${r.status} ${r.headers["Location"]} ${r.headers["Cache-Control"]}");
io.println(r.headers["Set-Cookie"]);
io.println(r.text());
let moved = client.get("/invites/old");
io.println("${moved.status} ${moved.headers["Location"]}");
```

Output:

```text
201 /invites/7 no-store
invited=1; Path=/; Max-Age=3600; HttpOnly; Secure; SameSite=Lax
{"email":"ada@example.com","id":7}
301 /invites
```

For a signed-in session, use the session store rather than writing the
cookie yourself; see [Authentication](08-auth.md). Header names are stored in
canonical form (`X-Request-ID` becomes `X-Request-Id`), and `header(name)` on
a response reads them case-insensitively.

## Errors

Throw an HTTP exception for an expected failure. The controller helpers
`this.badRequest`, `this.unauthorized`, `this.forbidden`, `this.notFound`,
`this.conflict` and `this.unprocessable` throw, so the code after them only
runs when the check passed. Outside a controller, throw the matching factory:
`gebweb.badRequest(detail)`, `gebweb.notFound(detail)` and so on. For any
other status, throw `errors.HttpException(status, detail, title)`:

<!-- doctest:run -->
```gb
import gebweb;
import gebweb.errors as errors;
import io;

@Controller("/accounts")
class AccountController extends gebweb.Controller {
    @Get("/{id}")
    func show(int id): dict<string, any> {
        if (id > 100) {
            this.notFound("no account ${id}");
        }
        return {"id": id};
    }

    @Post("/{id}/close")
    func close(int id): dict<string, any> {
        if (id == 1) {
            throw errors.HttpException(423, "account 1 is locked for audit", "Locked");
        }
        return {"closed": id};
    }
}

let client = gebweb.TestClient(gebweb.app());
let missing = client.get("/accounts/404");
io.println("${missing.status} ${missing.headers["Content-Type"]}");
io.println(missing.text());
io.println(client.post("/accounts/1/close", {}).text());
```

Output:

```text
404 application/problem+json
{"detail":"no account 404","status":404,"title":"Not Found","type":"about:blank"}
{"detail":"account 1 is locked for audit","status":423,"title":"Locked","type":"about:blank"}
```

Catch `errors.HttpException` to handle every HTTP exception at once; each
factory's exception extends it.

| Thrown by | Status |
|-----------|--------|
| `this.badRequest(detail)` / `gebweb.badRequest(detail)` | 400 |
| `this.unauthorized(detail)` / `gebweb.unauthorized(detail)` | 401 |
| `this.forbidden(detail)` / `gebweb.forbidden(detail)` | 403 |
| `this.notFound(detail)` / `gebweb.notFound(detail)` | 404 |
| `this.conflict(detail)` / `gebweb.conflict(detail)` | 409 |
| `this.unprocessable(detail, errors)` / `gebweb.unprocessableEntity(detail, errors)` | 422 |
| `gebweb.internalServerError(detail)` | 500 |
| `errors.HttpException(status, detail, title)` | any |

### Unexpected errors

Any other exception escaping a handler is a 500 that reveals nothing about the
error. In the dev environment (`--dev`, or `GEBWEB_ENV=dev` or `development`)
the 500 is a Problem Details response carrying the exception, and the error
and its stack trace are logged through the
[framework logger](37-deployment.md#logging):

```json
{
  "type": "about:blank",
  "title": "Internal Server Error",
  "status": 500,
  "detail": "card declined",
  "exception": {
    "class": "PaymentFailed",
    "message": "card declined",
    "trace": [
      {"function": "chargeCard", "line": 19},
      {"function": "CheckoutController.pay", "line": 25}
    ]
  }
}
```

Every other environment, including an unset `GEBWEB_ENV`, keeps the generic
500. Responses for `HttpException`s are the same in every environment.

## HTML

`this.html(body, status, headers)` and `gebweb.html(body, status, headers)`
send HTML with `Content-Type: text/html; charset=utf-8`, merging any extra
headers. For pages, render templates with `this.view` instead, which escapes
values and adds the CSRF token and flash messages; see
[Views and templates](04a-views.md).

## Files

`gebweb.file(path, opts)` sends a file from disk without reading it into
memory. The server handles `HEAD`, `Range` requests (206), and conditional
requests through `If-None-Match` and `If-Modified-Since` (304), and derives an
`ETag` from the file's size and modification time unless you set one. A
missing file is a 404.

<!-- doctest:fragment context=controller -->
```gb
@Get("/reports/{name}")
func report(string name): dict<string, any> {
    if (!re.test("^[a-z0-9-]+$", name)) {
        this.notFound("no report ${name}");
    }
    return gebweb.file("reports/${name}.pdf", {"attachment": true, "filename": "${name}.pdf"});
}
```

| Option | Effect |
|--------|--------|
| `contentType` | The `Content-Type`; by default it is inferred from the extension. |
| `attachment` | `true` adds `Content-Disposition: attachment`, so browsers download it. |
| `filename` | The download's suggested name; defaults to the file's own name. |
| `status` | The response status; default 200. |
| `headers` | Extra response headers. |

`gebweb.file` serves whatever path it is given, so never build the path from
client input without checking it, as the example does. A client cannot forge a
file response by returning data; only `gebweb.file` creates one.

`TestClient` returns the whole file and ignores `Range` and conditional
headers; test those against a real server. A response middleware that only
changes headers or status still applies to a file, but one that replaces the
body replaces the file. The `compress` middleware does not compress files;
precompress static assets instead ([Assets](26-assets.md)).

## Streaming

`this.stream(handler, opts)` (or `gebweb.stream`) sends the status and
headers at once, then calls `handler` with a stream handle to write the body
as it is produced:

<!-- doctest:fragment context=controller -->
```gb
@Get("/export.csv")
func export(): dict<string, any> {
    return this.stream(func(int handle): void {
        http.streamWrite(handle, "id,name\n");
        for (i in 1..4) {
            http.streamWrite(handle, "${i},item ${i}\n");
            http.streamFlush(handle);
        }
        http.streamClose(handle);
    }, {"contentType": "text/csv"});
}
```

Its options are `contentType` (default `text/plain; charset=utf-8`),
`status` and `headers`. `TestClient` cannot run a stream handler; test
streams against a real server. For Server-Sent Events, `@Sse` is simpler; see
[WebSockets and SSE](11-websockets-and-sse.md).

## Content negotiation with @Produces

`@Produces` lets one handler serve its data as JSON, CSV or XML, chosen by the
request's `Accept` header:

<!-- doctest:run -->
```gb
import gebweb;
import io;

@Controller("/reports")
class ReportController {
    @Get("/sales")
    @Produces("json", "csv")
    func sales(): list<dict<string, any>> {
        return [{"region": "North", "units": 120}, {"region": "South", "units": 95}];
    }
}

let client = gebweb.TestClient(gebweb.app());
for (accept in ["application/json", "text/csv", "application/xml"]) {
    let r = client.request("GET", "/reports/sales", null, {"Accept": accept});
    io.println("${accept} -> ${r.status} ${r.headers["Content-Type"]}");
    io.println(r.text().trim());
}
```

Output:

```text
application/json -> 200 application/json
[{"region":"North","units":120},{"region":"South","units":95}]
text/csv -> 200 text/csv
region,units
North,120
South,95
application/xml -> 406 application/problem+json
{"detail":"this endpoint produces: application/json, text/csv","status":406,"title":"Not Acceptable","type":"about:blank"}
```

- Formats can be named `json`, `csv` and `xml`, or by MIME type
  (`application/json`, `text/csv`, `application/xml`). Any other name fails at
  startup.
- The best match by `Accept` quality wins, with ties going to the format
  listed first. No `Accept` header, or `*/*`, picks the first format.
- An `Accept` header that matches none of the formats is a 406 listing them.
- CSV needs a list of flat dicts or objects; other data is a 500 explaining
  that it cannot be written as CSV. XML wraps a dict in `<response>` and a
  list in `<items>`, one `<item>` per element.
- `@Groups` on the handler filters fields the same way in every format.
- A handler that returns a built response (`this.json(...)` and so on) is
  sent as built; `@Produces` only applies to returned data.

## Hand-built response dicts

Underneath the helpers, a response is a dict with an int `status` and
optional `headers` and `body`. A returned dict counts as a response only when
its `status` is an int and it has no keys other than `status`, `headers`,
`body` and `stream`; any other dict is data, even one with a `status` field.
Prefer the helpers, which are easier to read and to get right; the raw form
is for code that already works with response dicts.

## Reference

| API | Returns |
|-----|---------|
| `gebweb.html(body, status = 200, headers = null)` | HTML response dict |
| `gebweb.htmlView(app, request, name, ctx, status = 200, headers = null)` | rendered template response dict |
| `gebweb.file(path, opts = null)` | file response; options above |
| `gebweb.stream(handler, opts = null)` | streaming response; options above |
| `gebweb.Response` | the response type returned by the controller helpers |
| `Response.withStatus(int)` / `withHeader(name, value)` / `withBody(body)` | a changed copy |
| `Response.status()` / `header(name)` / `headers()` / `text()` / `json()` | read the response |
| `@Produces(formats...)` | content negotiation between `json`, `csv` and `xml` |

Controller helpers and HTTP exceptions are listed in their sections above.
