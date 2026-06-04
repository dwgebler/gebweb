# Responses

A handler's return value is auto-shaped into the response the
framework sends to the client. The auto-shaping covers the common
JSON-API case; helpers exist for HTML, file downloads, and streaming
when JSON isn't right.

## The Request and Response objects

Handlers receive a rich `gebweb.Request` (declare a parameter typed
`gebweb.Request`) and the controller response builders return a rich
`Response`. The `Request` exposes `method()`, `path()`, `scheme()`,
`isSecure()`, `host()`, `clientIp()`, `clientCert()`, `header(name)`,
`cookie(name)`, the typed query getters (`query`, `queryInt`, `queryBool`,
`queryAll`), `isJson()`, `text()`, `json()`, and framework context
(`routeParam`, `locale`, `tenant`, `user`, `csrfToken`, `cspNonce`). Both
objects stay index-compatible (`req["headers"]`, `resp["status"]`) for
migration.

```gb
import gebweb;

@Get("/whoami")
func whoami(gebweb.Request req): gebweb.Response {
    return this.json({"ip": req.clientIp(), "agent": req.header("User-Agent")});
}
```

A handler may also build a response directly with the body-first builders
`http.response(body, status = 200)`, `http.jsonResponse(value, status = 200)`,
and `http.redirect(url, status = 302)`. Response header names are canonicalized
(e.g. `X-Request-ID` is stored as `X-Request-Id`); read them case-insensitively
with `resp.header(name)`.

## Automatic JSON wrapping

Anything that isn't already a response-shaped dict is wrapped as a
200 JSON response:

- `null` becomes `{"status": 204, "body": ""}`.
- `dict<string, any>` becomes `{"status": 200, "headers": {"Content-Type":
  "application/json"}, "body": <dict>}`.
- `list<any>` is wrapped the same way.
- A class instance, string, or primitive is stringified via
  `json.stringify` and wrapped.

A dict that already has a `"status"` key is treated as a fully
pre-shaped response and passed through unchanged. This is the
contract every helper below relies on.

## HTML responses

```gb
import gebweb;

@Get("/page")
func page(): dict<string, any> {
    return gebweb.html("<h1>Hello</h1>");
}

@Get("/created")
func created(): dict<string, any> {
    return gebweb.html("<p>ok</p>", 201, {"Cache-Control": "no-store"});
}
```

The second and third arguments are optional. Custom headers merge on
top of the default `Content-Type: text/html; charset=utf-8`.

## File downloads

```gb
@Get("/download/{id}")
func download(string id): dict<string, any> {
    return gebweb.file("./uploads/" + id + ".pdf",
        {"attachment": true, "filename": "report.pdf"});
}
```

The body is the raw bytes read from disk via `io.readBytes`.
Content-Type is inferred from the file extension via a small built-in
map; pass `opts.contentType` to override. `opts.attachment = true`
adds `Content-Disposition: attachment`, with the suggested filename
from `opts.filename` (defaulting to the file's base name).

## Streaming responses

`gebweb.stream(handler, opts?)` returns a response dict the lower-level
HTTP server recognises as a streaming response. The handler runs after
status + headers are flushed and gets a raw stream handle:

```gb
import http;

@Get("/events")
func events(): dict<string, any> {
    return gebweb.stream(func(int h): void {
        for (i in 1..6) {
            http.streamWrite(h, "tick " + (i as string) + "\n");
            http.streamFlush(h);
            time.sleep(1.0);
        }
        http.streamClose(h);
    });
}
```

For Server-Sent Events specifically, `@Sse` is more ergonomic - see
[WebSockets and SSE](11-websockets-and-sse.md).

## Custom responses

The auto-wrapping behaviour is built on a single contract: any dict
your handler returns that already has a `"status"` key is treated
as a fully pre-shaped response and passed through untouched. That
means every status code, content type, cookie, and header
combination is reachable by returning the dict directly:

```gb
@Post("/teapots")
func brew(): dict<string, any> {
    return {
        "status":  418,
        "headers": {"Content-Type": "text/plain"},
        "body":    "I'm a teapot",
    };
}

@Post("/users")
func create(UserDto body): dict<string, any> {
    let user = repo.insert(body);
    return {
        "status":  201,
        "headers": {"Content-Type": "application/json",
                    "Location":     "/users/" + user.id},
        "body":    json.stringify(user),
    };
}
```

The minimum shape is `{"status": N, "body": <string or bytes>}`.
The `"headers"` dict is optional; a missing or empty `"body"` is
treated as the empty string.

### Setting cookies

Cookies are headers under the hood. Build a `Set-Cookie` string
and pass it through `"headers"`:

```gb
return {
    "status":  200,
    "headers": {"Set-Cookie": "session=abc; HttpOnly; Path=/; Max-Age=3600"},
    "body":    json.stringify({"ok": true}),
};
```

For session-backed cookies, prefer the session store API in the
[Authentication](08-auth.md) chapter rather than hand-rolling the
cookie.

### Adding headers to a helper response

The helpers return the same dict shape, so you can mutate the
result before returning it:

```gb
let r = gebweb.html("<p>ok</p>", 200);
(r["headers"] as dict<string, any>)["Cache-Control"] = "no-store";
return r;
```

## HTTP exceptions

Throwing one of the framework's HTTP-shaped exceptions short-circuits
the response with an RFC 9457 Problem Details body at the matching
status:

```gb
@Get("/users/{id}")
func get(string id): dict<string, any> {
    if (id == "") {
        throw gebweb.badRequest("id is required");
    }
    throw gebweb.notFound("no user with id " + id);
}
```

The shipped factories cover the eight most common cases. For any
status code outside that set, construct an `HttpException`
directly:

```gb
import gebweb.errors as errors;

throw errors.HttpException(418, "I'm a teapot", "Teapot");
throw errors.HttpException(429, "rate limit exceeded", "Too Many Requests");
throw errors.HttpException(503, "database is down", "Service Unavailable");
```

`HttpException(status, detail, title)` is the underlying
constructor; the framework's Problem Details renderer formats it
the same way it formats the factory-built exceptions.

The factory functions all return objects that extend
`gebweb.errors.HttpException`. Use them for catch dispatch:

```gb
import gebweb.errors as errors;

try {
    /* ... */
} catch (errors.HttpException e) {
    /* ... */
}
```

## Inside a controller class

Controllers that extend `gebweb.Controller` gain instance methods
that wrap the same primitives more ergonomically:

```gb
class UserController extends gebweb.Controller {
    @Get("/users/{id}")
    func get(string id): dict<string, any> {
        if (id == "") { this.badRequest("id is required"); }
        let u = this.repo.find(id);
        if (u == null)  { this.notFound("no user with id " + id); }
        return u as dict<string, any>;
    }

    @Post("/login")
    func login(LoginDto body): dict<string, any> {
        if (auth.verify(body)) {
            return this.redirect("/dashboard");
        }
        return this.redirect("/login?error=1", 303);
    }
}
```

The methods (`this.badRequest`, `this.notFound`, etc.) throw the
matching exception so the handler body reads as straight-line
code. `this.redirect(location, status?)`, `this.back(request)`,
`this.view(request, name, ctx)`, `this.partial(request, name)`,
`this.flash(...)`, and `this.redirectWithFlash(...)` are the
remaining surfaces; see the
[Controller base class](#controller-base-class-summary) below.

## Reference

### Response helpers

| Helper | Returns | Notes |
|--------|---------|-------|
| `gebweb.html(body, status?, headers?)` | dict | Default status 200, Content-Type `text/html; charset=utf-8`. |
| `gebweb.htmlView(app, request, name, ctx, status?, headers?)` | dict | Renders a template via the registered view engine; threads request through the view-context injectors. |
| `gebweb.file(path, opts?)` | dict | `opts.contentType`, `opts.attachment`, `opts.filename`, `opts.status`, `opts.headers`. |
| `gebweb.stream(handler, opts?)` | dict | `opts.contentType`, `opts.headers`, `opts.status`. Handler takes a stream handle (int). |
| Raw dict `{"status": N, "headers": {...}, "body": "..."}` | dict | Any status / content type. The lowest-level contract every helper compiles to. |

### HTTP exception factories

All return a class extending `errors.HttpException`; throwing
short-circuits with a Problem Details body at the matching status.

| Factory | Status |
|---------|--------|
| `gebweb.badRequest(detail)` | 400 |
| `gebweb.unauthorized(detail)` | 401 |
| `gebweb.forbidden(detail)` | 403 |
| `gebweb.notFound(detail)` | 404 |
| `gebweb.conflict(detail)` | 409 |
| `gebweb.unprocessableEntity(detail, errors)` | 422 |
| `gebweb.internalServerError(detail)` | 500 |
| `errors.HttpException(status, detail, title)` | any | Direct constructor for status codes outside the factory set. |

### Controller base class summary

`gebweb.Controller` methods that build a response or throw an
exception:

| Method | Returns / throws |
|--------|------------------|
| `this.json(data, status = 200)` | JSON response dict. |
| `this.text(body, status = 200)` | Plain-text response dict (`Content-Type: text/plain`). |
| `this.html(body, status = 200, headers = null)` | Same as `gebweb.html(...)`. |
| `this.created(data, location)` | 201 with `Location` header and JSON body. |
| `this.accepted(data = null)` | 202; body is JSON-encoded `data` or empty. |
| `this.stream(handler, opts = null)` | Streaming response dict. |
| `this.redirect(location, status = 302)` | 3xx redirect dict. |
| `this.back(request, fallback = "/")` | 303 redirect to `Referer` or fallback. |
| `this.view(request, name, ctx, status = 200)` | HTML rendered from the view engine. |
| `this.partial(request, name, ctx)` | HTML fragment with `Vary: HX-Request`. |
| `this.flash(request, response, category, message)` | Returns `response` with a session flash attached. |
| `this.redirectWithFlash(request, location, category, message)` | 303 redirect + flash. |
| `this.badRequest(detail)` / `this.unauthorized(detail)` / `this.forbidden(detail)` / `this.notFound(detail)` / `this.conflict(detail)` / `this.unprocessable(detail, errs)` | Throws the matching `HttpException`. |
| `this.problem(status, title, detail, extras = null)` | Builds a Problem Details response at any status without throwing. |
