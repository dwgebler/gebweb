# File storage

<!-- doctest:strict -->

Storage separates application object names from their persistence backend.
Configure one backend, then use the same `put`, `get`, existence, delete and URL
helpers from controllers or services. Memory storage suits tests, local storage
writes files, and S3 storage sends signed requests to an object service.

## Reading and writing

This complete example round-trips binary data through memory storage, then
deletes it. No filesystem directory or external service is needed:

<!-- doctest:run -->
```gb
import bytes;
import gebweb;
import io;

let app = gebweb.app();
gebweb.useStorage(app, gebweb.memoryStorage());
gebweb.put(app, "avatars/u-1.bin", bytes.fromHex("00ff4142"));
io.println(gebweb.storageExists(app, "avatars/u-1.bin"));
io.println(bytes.toHex(gebweb.get(app, "avatars/u-1.bin") as bytes));
io.println(gebweb.storageUrl(app, "avatars/u-1.bin"));
gebweb.storageDelete(app, "avatars/u-1.bin");
io.println(gebweb.storageExists(app, "avatars/u-1.bin"));
io.println(gebweb.get(app, "avatars/u-1.bin") == null);
```

```text
true
00ff4142
memory:///avatars/u-1.bin
false
true
```

| Operation | Result / behavior |
|-----------|-------------------|
| `gebweb.useStorage(app, backend)` | Configure the app's backend; return the app. |
| `gebweb.put(app, name, content)` | Write string or bytes content under a name; return `void`. Existing names can be overwritten. |
| `gebweb.get(app, name)` | Read `?bytes`; missing objects return `null`. |
| `gebweb.storageExists(app, name)` | Ask the backend whether the name exists. Without a backend, return `false`. |
| `gebweb.storageDelete(app, name)` | Delete an object; missing objects are a no-op for the built-in backends. |
| `gebweb.storageUrl(app, name)` | Build a backend-specific location string without checking existence. |

All operations except `storageExists` require a configured backend and raise an
error when one is absent. String content is converted to bytes; `get` returns
bytes even when the original content was a string. Decode text deliberately.
The abstraction does not carry content-type metadata or an object version.

## Choosing a backend

| Factory | Persistence / use |
|---------|-------------------|
| `gebweb.memoryStorage()` | Process-local data held by that backend instance; useful for isolated tests. |
| `gebweb.localStorage(dir)` | Files under an application-owned directory; useful on a server with persistent disk. |
| `gebweb.s3Storage(opts)` | Path-style signed object requests to S3 or a compatible service. |
| Custom `gebweb.StorageContract` | Application-defined persistence or read-only content. |

Configure storage in `build(conn)`, before constructing a `TestClient` or
starting the server. A controller needing the backend can take a
`gebweb.GebwebApp` constructor parameter and use the facade helpers. See
[Getting started](01-getting-started.md) and
[Dependency injection](06a-dependency-injection.md).

Memory storage uses a `store.Store` for shared writes. It does not survive
process restart and is not shared automatically with another app or worker
process. Its backend-specific `keys()` method lists stored names; do not assume
that listing is part of every storage backend's contract.

## Local files and URLs

Local storage creates parent directories when writing. This example uses a
private temporary root and a URL prefix:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import path;

let root = io.tempDir("storage-example-");
let app = gebweb.app();
gebweb.useStorage(app, gebweb.localStorage(root).withUrlPrefix("/uploads"));
gebweb.put(app, "reports/summary.txt", "ready");
io.println((gebweb.get(app, "reports/summary.txt") as bytes).toString());
io.println(io.exists(path.join(root, "reports/summary.txt")));
io.println(gebweb.storageUrl(app, "reports/summary.txt"));
gebweb.storageDelete(app, "reports/summary.txt");
io.println(gebweb.storageExists(app, "reports/summary.txt"));
```

```text
ready
true
/uploads/reports/summary.txt
false
```

| Local configuration | Default | Effect |
|---------------------|---------|--------|
| `localStorage(dir)` | Required root | Resolve object names beneath this filesystem directory. |
| `.withUrlPrefix(prefix)` | Empty prefix | Build `prefix + "/" + name` instead of a `file://` location. Trailing slashes are trimmed. |

`withUrlPrefix` does not mount a server route. If these are public assets,
configure a matching static-file mount separately:

<!-- doctest:fragment context=app -->
```gb
gebweb.useStorage(app, gebweb.localStorage("/srv/public-uploads").withUrlPrefix("/uploads"));
gebweb.useStaticAssets(app, "/srv/public-uploads", {"urlPrefix": "/uploads"});
```

See [Assets](26-assets.md). For private content, use an authorized controller
or another access-controlled delivery mechanism instead of publishing the entire
storage directory. A `file://` or `memory:///` location is not a client-facing
HTTP download route.

Local writes replace file contents directly; the abstraction does not promise
atomic replacement, exclusive creation or cross-operation transactions. Keep
persistent data outside generated build directories and provision the required
filesystem permissions and retention policy.

## Saving uploaded files

`UploadedFile.saveToStorage(app, name)` writes the upload's bytes to the selected
backend. It is equivalent to `gebweb.put(app, name, upload.bytes())`.

This controller requires an `avatar` part, uses a server-chosen name and returns
a typed receipt. The demo deliberately replaces the same object on each upload:

<!-- doctest:run -->
```gb
import bytes;
import gebweb;
import io;

class StoredUpload {
    string name;
    int size;

    func StoredUpload(string name, int size) {
        this.name = name;
        this.size = size;
    }
}

@Controller("/avatar")
class AvatarController {
    gebweb.GebwebApp app;

    func AvatarController(gebweb.GebwebApp app) {
        this.app = app;
    }

    @Post("")
    @MaxBody(1048576)
    func upload(dict<string, gebweb.UploadedFile> files): StoredUpload {
        if (!files.contains("avatar")) {
            throw gebweb.badRequest("avatar file is required");
        }
        let avatar = files["avatar"];
        string name = "avatars/demo.bin";
        avatar.saveToStorage(this.app, name);
        return StoredUpload(name, avatar.size());
    }
}

let app = gebweb.app();
gebweb.useStorage(app, gebweb.memoryStorage());
let response = gebweb.TestClient(app).multipart(
    "/avatar",
    {
        "avatar": {
            "filename": "client-name.bin",
            "contentType": "application/octet-stream",
            "content": bytes.fromHex("00ff4142"),
        },
    },
    {}
);
io.println(response.status);
io.println(response.json()["name"]);
io.println(response.json()["size"]);
io.println(bytes.toHex(gebweb.get(app, "avatars/demo.bin") as bytes));
```

```text
200
avatars/demo.bin
4
00ff4142
```

For an application, derive an authorized user's storage key from validated
server-side identity, or generate a unique object name. Do not trust the
client's filename, declared content type or extension as proof of content.
Validate the permitted size and actual format before writing. `@MaxBody` limits
the whole multipart request, including its overhead; it is not an exact maximum
for the file alone. See [Parameter binding](03-parameter-binding.md) and
[Security](27-security.md).

Uploads and `get` materialize complete byte values. S3 removes the HTTP client's
response-size cap for object bodies, but that does not make reads streaming or
memory usage bounded. Set application upload/download limits appropriate to the
objects you intend to handle.

## Names and path safety

Use forward-slash-separated relative names that the application controls.
Built-in `put` and `get` reject an empty name, a leading `/`, and any slash
segment exactly `.` or `..`:

<!-- doctest:run -->
```gb
import gebweb;
import io;

let app = gebweb.app();
gebweb.useStorage(app, gebweb.memoryStorage());
try {
    gebweb.put(app, "../outside.txt", "blocked");
} catch (RuntimeError error) {
    io.println(error.message);
}
```

```text
storage: name segment '..' is not allowed: ../outside.txt
```

Local and S3 existence/delete operations perform the same validation. Memory
existence/delete are direct key operations, and URL builders concatenate names
without applying those path checks or URL-encoding the generated location.
Validate keys at the application boundary instead of treating a successful
`storageUrl` call as a path-validation check. Prefer simple URL-safe object
names when exposing generated locations.

These are lexical checks, not a filesystem sandbox. Keep the local root and
its directory structure under trusted control, including symbolic links. Do not
allow another actor to replace directories beneath that root. Custom backends
must define and enforce the name policy their persistence mechanism needs.

## S3 and compatible services

Construction is local; object operations make the network requests. This
example inspects configuration without contacting a service:

<!-- doctest:run -->
```gb
import gebweb;
import io;

let backend = gebweb.s3Storage({
    "bucket": "uploads",
    "region": "us-east-1",
    "accessKey": "EXAMPLE_ACCESS_KEY",
    "secretKey": "example-only-secret",
    "endpoint": "http://127.0.0.1:9000/",
    "timeoutMs": 30000,
});
io.println(backend.endpoint);
io.println(backend.timeoutMs);
io.println(backend.url("avatars/u-1.bin"));
```

```text
http://127.0.0.1:9000
30000
http://127.0.0.1:9000/uploads/avatars/u-1.bin
```

| S3 option | Default | Effect |
|-----------|---------|--------|
| `bucket` | Required | Bucket used in path-style requests. |
| `region` | Required | SigV4 signing region and default endpoint region. |
| `accessKey` | Required | Signing access key. |
| `secretKey` | Required | Signing secret. |
| `endpoint` | `https://s3.<region>.amazonaws.com` | Scheme and host of the object service; trailing slashes are trimmed. |
| `timeoutMs` | `600000` | Deadline per object request in milliseconds; `0` disables it. |

The factory uses explicit credentials; it does not discover an SDK credential
chain or accept a session-token option. Load supported secrets through your
application configuration. A local MinIO service can use a custom HTTP endpoint;
remote services should use the HTTPS endpoint and signing region they require.
Compatibility depends on support for these path-style SigV4 operations rather
than on a provider name alone.

| S3 operation | Response behavior |
|--------------|-------------------|
| `put` | Throw on an HTTP status of 400 or higher. |
| `get` | Return `null` on 404; throw on other statuses of 400 or higher. |
| `exists` | Return `true` for 2xx/3xx, otherwise `false`; a false result alone does not distinguish missing objects from permission or server errors. |
| `delete_` | Treat 404 as success; throw on other statuses of 400 or higher. |

Transport failures can still throw. There is no application-level retry policy
in this backend. `storageUrl` constructs an unsigned location, not a presigned
URL, and does not grant read access or guarantee that a bucket/object is public.
The configured credentials authorize signed requests made by this backend.

`gebweb.storage` also exports `signingKey`, `canonicalRequest` and
`stringToSignForSigv4` for lower-level signing work. They are not a ready-made
presigned-download helper; callers would own URL construction, expiry and the
complete signing protocol.

## Custom backends

Implement `gebweb.StorageContract` with all five methods below. This runnable
read-only catalog is a small example of a backend that does not accept writes:

<!-- doctest:run -->
```gb
import bytes;
import gebweb;
import io;

class ReadOnlyCatalog implements gebweb.StorageContract {
    func put(string name, any content): void {
        throw RuntimeError("catalog is read-only");
    }

    func get(string name): ?bytes {
        return name == "readme.txt" ? bytes.fromString("Catalog help") : null;
    }

    func exists(string name): bool {
        return name == "readme.txt";
    }

    func delete_(string name): void {
        throw RuntimeError("catalog is read-only");
    }

    func url(string name): string {
        return "catalog:///" + name;
    }
}

let app = gebweb.app();
gebweb.useStorage(app, ReadOnlyCatalog());
io.println((gebweb.get(app, "readme.txt") as bytes).toString());
io.println(gebweb.storageExists(app, "missing.txt"));
try {
    gebweb.put(app, "readme.txt", "replacement");
} catch (RuntimeError error) {
    io.println(error.message);
}
```

```text
Catalog help
false
catalog is read-only
```

A writable custom backend should preserve byte content, return `null` for a
missing read and define its error behavior explicitly. Keep shared state safe
for concurrent calls. The app can share one backend across requests and jobs;
individually safe methods do not make a read-modify-write sequence transactional.

## Reference

| API | Contract |
|-----|----------|
| `gebweb.StorageContract` | `put(string, any): void`, `get(string): ?bytes`, `exists(string): bool`, `delete_(string): void`, `url(string): string`. |
| `gebweb.useStorage(app, backend)` | Configure a storage instance. |
| `gebweb.memoryStorage()` | In-memory backend, with backend-specific `keys()`. |
| `gebweb.localStorage(dir)` | Filesystem backend; optional `.withUrlPrefix(prefix)`. |
| `gebweb.s3Storage(opts)` | Signed object-service backend; options listed above. |
| `gebweb.put`, `gebweb.get` | Write content and read bytes through the app's backend. |
| `gebweb.storageExists`, `storageDelete`, `storageUrl` | Existence, deletion and location helpers. |
| `UploadedFile.saveToStorage(app, name)` | Save the current upload's bytes under a chosen key. |

Related chapters: [Parameter binding](03-parameter-binding.md),
[Testing](14-testing.md), [Assets](26-assets.md),
[Security](27-security.md), [Configuration](29-services-yaml.md), and
[Deployment](37-deployment.md).
