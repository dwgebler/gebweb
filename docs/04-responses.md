# Responses

A handler's return value is auto-shaped into the response the
framework sends to the client. The auto-shaping covers the common
JSON-API case; helpers exist for HTML, file downloads, and streaming
when JSON isn't right.

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

## Reference

| Helper | Returns | Notes |
|--------|---------|-------|
| `gebweb.html(body, status?, headers?)` | dict | Default status 200, Content-Type `text/html; charset=utf-8`. |
| `gebweb.file(path, opts?)` | dict | `opts.contentType`, `opts.attachment`, `opts.filename`, `opts.status`, `opts.headers`. |
| `gebweb.stream(handler, opts?)` | dict | `opts.contentType`, `opts.headers`, `opts.status`. Handler takes a stream handle (int). |

HTTP exception factories (all return a class extending
`errors.HttpException`):

- `gebweb.badRequest(detail)` - 400
- `gebweb.unauthorized(detail)` - 401
- `gebweb.forbidden(detail)` - 403
- `gebweb.notFound(detail)` - 404
- `gebweb.conflict(detail)` - 409
- `gebweb.unprocessableEntity(detail, errors)` - 422
- `gebweb.internalServerError(detail)` - 500
