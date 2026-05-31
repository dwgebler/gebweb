# File uploads

A handler that declares a `dict<string, gebweb.UploadedFile>`
parameter receives the parsed file map from a `multipart/form-data`
request body. The framework calls `web.parseMultipart(request)`
once, wraps each file in an `UploadedFile`, and binds the resulting
dict to the parameter.

## Handler signature

```gb
import gebweb.uploads as uploads;

class AssetController {
    @Post("/upload")
    func upload(dict<string, uploads.UploadedFile> files): dict<string, any> {
        let avatar = files["avatar"];
        avatar.saveTo("/srv/uploads/" + avatar.filename);
        return {
            "filename": avatar.filename,
            "contentType": avatar.contentType,
            "size": avatar.size(),
        };
    }
}
```

The dict is keyed by the form-field name. Non-file form fields are
NOT auto-bound - they're available via the raw `web.parseMultipart`
call or by adding a raw-request parameter alongside the file map (in
which case the handler reads `request["body"]` and parses manually).

A request without `Content-Type: multipart/form-data` fails the
upload-bind step with a 400 Bad Request.

## `UploadedFile`

```gb
class UploadedFile {
    string filename;        # client-provided name
    string contentType;     # client-provided MIME type
    bytes data;             # raw payload

    func bytes(): bytes;
    func size(): int;
    func saveTo(string path): void;
    func saveToStorage(gebweb.GebwebApp app, string name): string;
}
```

`bytes()` returns the payload, `size()` reports its length,
`saveTo(path)` writes to a local file in one call. If you've
wired a storage backend with `gebweb.useStorage` (memory, local
disk, or S3), `saveToStorage(app, "users/" + id + ".png")` writes
straight to that backend instead. See
[Storage](21-storage.md) for the backend setup.

## Multiple files

A single multipart request may contain several files under different
field names; all of them appear in the `dict<string, UploadedFile>`:

```gb
@Post("/upload")
func upload(dict<string, uploads.UploadedFile> files): dict<string, any> {
    list<string> names = [];
    for (k, v in files.items()) {
        names = names.push(k);
    }
    return {"count": files.length(), "fields": names.sort()};
}
```

If a client posts the same field name multiple times, the last entry
wins. For one field with many values, use a raw handler and call
`web.parseMultipart` directly to inspect the iteration order.

## Working with the raw parse result

`web.parseMultipart(request)` returns a dict shaped as:

```geblang
{
    "fields": dict<string, string>,
    "files":  dict<string, dict>,
}
```

Each entry in `files` is `{filename, contentType, bytes}`. The Gebweb
binder transforms `files` through `uploads.fromParseResult(...)` to
build typed `UploadedFile` instances; you can call the helper yourself
on a raw parse result to get the same shape from outside the
framework.

```gb
import web;

let parsed = web.parseMultipart(request);
let files = uploads.fromParseResult(parsed["files"] as dict<string, any>);
let name = (parsed["fields"] as dict<string, any>)["name"] as string;
```

## Limits and streaming

Gebweb's multipart parsing reads the whole body into memory before
binding. There's no built-in streaming-upload variant in 1.0; for
very large files, expose a separate route that takes a raw request
and writes the body to disk incrementally with `io.writeBytes`.

## Reference

- Parameter type: `dict<string, gebweb.uploads.UploadedFile>` (bare
  `dict<string, UploadedFile>` and module-qualified forms both
  recognised; see `gebweb.types.isUploadsMapType`).
- `gebweb.uploads.UploadedFile` - `filename`, `contentType`, `data`;
  methods `bytes()`, `size()`, `saveTo(path)`.
- `gebweb.uploads.fromParseResult(files): dict<string, UploadedFile>`
- Native: `web.parseMultipart(request): dict<string, any>` - returns
  `{"fields", "files"}`. Throws on non-multipart bodies (the binder
  wraps this as a 400).
