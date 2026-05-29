# File storage

Gebweb's storage abstraction sits between application code and
the actual persistence backend. Application code calls
`gebweb.put`, `gebweb.get`, `gebweb.storageExists`,
`gebweb.storageDelete`, and `gebweb.storageUrl`; the configured
backend handles the IO.

v1 ships three built-in backends:

| Backend | When | Behaviour |
|---------|------|-----------|
| `gebweb.memoryStorage()` | Tests | Process-local dict; survives only as long as the app. |
| `gebweb.localStorage(dir)` | Dev / single-server prod | Files under `dir`; sub-directories auto-created. |
| `gebweb.s3Storage(opts)` | Prod | AWS S3 (and S3-compatible: MinIO, R2, B2) with sigv4. |

Other object stores (Azure Blob, GCS) plug in by implementing
the `StorageContract` interface against the SDK or REST API of
your choice.

## Wiring

```gb
import gebweb;

let app = gebweb.app([UploadController()]);

if (env.get("ENV") == "test") {
    gebweb.useStorage(app, gebweb.memoryStorage());
} else {
    gebweb.useStorage(app, gebweb.localStorage("/srv/uploads")
        .withUrlPrefix("/uploads"));
}
```

`withUrlPrefix` configures what `storageUrl(app, name)` returns
for `LocalStorage` - typically set this when you serve the
storage root through a static-file route at the matching path.

## Reading and writing

```gb
gebweb.put(app, "avatars/123.jpg", uploaded.bytes());
let raw = gebweb.get(app, "avatars/123.jpg");  /* ?bytes */
gebweb.storageDelete(app, "avatars/123.jpg");
let isAvailable = gebweb.storageExists(app, "avatars/123.jpg");
let publicUrl = gebweb.storageUrl(app, "avatars/123.jpg");
```

`put` accepts `string` or `bytes` content. `get` returns
`?bytes` (null when the name doesn't exist).

## UploadedFile.saveToStorage

The multipart-upload wrapper has a convenience helper:

```gb
class UploadController {
    @Post("/avatar")
    func upload(dict<string, gebweb.UploadedFile> files): dict<string, any> {
        let avatar = files["avatar"];
        avatar.saveToStorage(app, "avatars/" + currentUser.id + ".jpg");
        return {"ok": true};
    }
}
```

This is shorthand for `gebweb.put(app, name, avatar.bytes())`.

## Path safety

Names are forward-slash-separated paths relative to the backend
root. Both built-in backends reject:

- Empty names.
- Absolute names (`name.startsWith("/")`).
- Any segment equal to `..` or `.`.

These checks defend against accidental escapes; callers should
still sanitise user-provided components (e.g. the `avatars/<userId>`
example above relies on the caller controlling `currentUser.id`).

## S3 and S3-compatible

`gebweb.s3Storage(opts)` returns a backend that talks to AWS S3
(or any sigv4-compatible service) using the same `put`/`get`/
`exists`/`delete_` surface as the other backends.

```gb
gebweb.useStorage(app, gebweb.s3Storage({
    "bucket":    "acme-uploads",
    "region":    "us-east-1",
    "accessKey": env.get("AWS_ACCESS_KEY_ID"),
    "secretKey": env.get("AWS_SECRET_ACCESS_KEY")
}));
```

Options:

| Key | Required | Notes |
|-----|----------|-------|
| `bucket` | yes | Bucket name; path-style URLs are used. |
| `region` | yes | Used for the sigv4 credential scope. |
| `accessKey` | yes | Read from env in production. |
| `secretKey` | yes | Read from env in production. |
| `endpoint` | no | Default `https://s3.<region>.amazonaws.com`. Set to point at MinIO / R2 / B2 / a custom test server. |

MinIO example:

```gb
gebweb.useStorage(app, gebweb.s3Storage({
    "bucket":    "uploads",
    "region":    "us-east-1",
    "accessKey": "minioadmin",
    "secretKey": "minioadmin",
    "endpoint":  "http://minio:9000"
}));
```

Cloudflare R2 example:

```gb
gebweb.useStorage(app, gebweb.s3Storage({
    "bucket":    "uploads",
    "region":    "auto",
    "accessKey": env.get("R2_ACCESS_KEY_ID"),
    "secretKey": env.get("R2_SECRET_ACCESS_KEY"),
    "endpoint":  "https://" + env.get("R2_ACCOUNT_ID") + ".r2.cloudflarestorage.com"
}));
```

The signing chain (`storage.signingKey`, `storage.canonicalRequest`,
`storage.stringToSignForSigv4`) is also exported as free
functions so application code can sign one-off requests
directly, or build pre-signed URLs.

## Custom backends

Implement `gebweb.StorageContract`:

```gb
class S3Storage implements gebweb.StorageContract {
    string bucket;
    string region;
    string accessKey;
    string secretKey;

    func S3Storage(string bucket, string region, string accessKey, string secretKey) {
        this.bucket = bucket;
        this.region = region;
        this.accessKey = accessKey;
        this.secretKey = secretKey;
    }

    func put(string name, any content): void { /* signed PUT */ }
    func get(string name): ?bytes { /* signed GET */ }
    func exists(string name): bool { /* signed HEAD */ }
    func delete_(string name): void { /* signed DELETE */ }
    func url(string name): string {
        return "https://" + this.bucket + ".s3." + this.region + ".amazonaws.com/" + name;
    }
}

gebweb.useStorage(app, S3Storage("acme-uploads", "us-east-1", key, secret));
```

## Reference

| Helper | Purpose |
|--------|---------|
| `gebweb.StorageContract` | Backend interface: `put`, `get`, `exists`, `delete_`, `url`. |
| `gebweb.useStorage(app, backend)` | Attach a backend. |
| `gebweb.memoryStorage()` | In-process dict storage (tests). |
| `gebweb.localStorage(dir)` | Filesystem storage. `.withUrlPrefix(prefix)` for serving via static-file route. |
| `gebweb.s3Storage(opts)` | S3 / S3-compatible with sigv4. See "S3 and S3-compatible" above for opts. |
| `gebweb.put(app, name, content)` | Write (`content` is string or bytes). |
| `gebweb.get(app, name)` | Read (`?bytes`). |
| `gebweb.storageExists(app, name)` | Check. |
| `gebweb.storageDelete(app, name)` | Delete. |
| `gebweb.storageUrl(app, name)` | Public URL for serving. |
| `UploadedFile.saveToStorage(app, name)` | Convenience: write the upload's bytes. |
