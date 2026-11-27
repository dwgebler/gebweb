# Asset pipeline

<!-- doctest:strict -->

`gebweb.useStaticAssets` serves a public directory and registers the template
`asset` filter. By default, setup hashes each file and indexes its URL once.
A changed file gets a new URL after the application is rebuilt or restarted,
so browsers can cache the old URL without checking it again.

## Serve a stylesheet

Create `public/app.css`:

<!-- doctest:file public/app.css -->
```css
body { color: navy; }
```

Create `templates/page.html`. Use the filter rather than hard-coding a hash:

<!-- doctest:file templates/page.html -->
```html
<link rel="stylesheet" href="{{ 'app.css' | asset }}">
<h1>Assets</h1>
```

This complete program renders the page and fetches its stylesheet in process:

<!-- doctest:run -->
```gb
import gebweb;
import gebweb.assets as assets;
import io;

@Controller("/page")
class AssetPage extends gebweb.Controller {
    @Get("")
    func page(gebweb.Request request): gebweb.Response {
        return this.view(request, "page.html", {});
    }
}

let app = gebweb.app();
gebweb.useViews(app, "templates");
gebweb.useStaticAssets(app, "public");
let client = gebweb.TestClient(app);
string url = assets.assetUrl(app.assetConfig as assets.AssetConfig, "app.css");
let response = client.get(url);
io.println(url);
io.println(client.get("/page").text().contains(url));
io.println(response.status);
io.println(response.headers["Content-Type"]);
io.println(response.headers["Cache-Control"]);
io.println(response.text().trim());
io.println(client.get("/assets/app.css").status);
```

```text
/assets/app-b697f407.css
true
200
text/css
public, max-age=31536000, immutable
body { color: navy; }
404
```

The eight hexadecimal characters are the start of the file's SHA-256 digest,
including its trailing newline. Subdirectories are preserved:
`img/logo.svg` becomes `img/logo-<hash>.svg`. A missing logical name falls back
to `/assets/<name>`; it does not add a file to the production manifest. In the
normal fingerprinted mode, an unhashed name returns 404.

In a real application, place setup in `build(conn)` and use the
[standard application entry point](01-getting-started.md). Create the asset
directory before calling `useStaticAssets`.

## Options

The optional third argument is a dictionary:

| Option | Default | Behavior |
|--------|---------|----------|
| `urlPrefix` | `"/assets"` | Owns requests beneath this URL prefix, including missing files. Do not put controller routes there. |
| `cacheControl` | `"public, max-age=31536000, immutable"` | Header string or `func(string relativePath): string`, called after successful file resolution. |
| `fingerprint` | `true` | Index content-hashed URLs at setup. Set false if another build tool supplies versioned names. |
| `dev` | `false` | Use logical URLs and read current files without rebuilding a manifest. |
| `precompressed` | `false` | Negotiate existing neighboring gzip sidecars. |
| `middleware` | `true` | Run the app's before/after middleware for asset requests. |

The cache callback receives the requested path relative to `urlPrefix`, such
as `app-b697f407.css`, including its hash when that URL was requested.

`dev` and `fingerprint: false` do not change the cache header. For editable,
unversioned files, set an appropriate policy explicitly:

<!-- doctest:run -->
```gb
import gebweb;
import io;

let app = gebweb.app([], {"docs": false});
gebweb.useStaticAssets(app, "public", {"dev": true, "cacheControl": "no-cache"});
let client = gebweb.TestClient(app);
io.println(client.get("/assets/app.css").headers["Cache-Control"]);
io.writeText("public/app.css", "body { color: red; }");
io.println(client.get("/assets/app.css").text());
```

```text
no-cache
body { color: red; }
```

This reads an edited output file immediately. It does not compile TypeScript
or Sass, and it does not change the template reload policy.

## Middleware

Assets run app-level before middleware in registration order, stopping at the
first response, then after middleware in reverse order. Login gates, rate
limits and WAF checks therefore also cover CSS and JavaScript. For a public
asset directory, a login gate can explicitly allow its prefix:

<!-- doctest:run -->
```gb
import gebweb;
import io;

let app = gebweb.app([], {"docs": false});
gebweb.useStaticAssets(app, "public");
gebweb.before(app, func(gebweb.Request request): any {
    if (request.path().startsWith("/assets/")) {
        return null;
    }
    return gebweb.html("Sign in", 401);
});
gebweb.use(app, func(gebweb.Request request, gebweb.Response response): gebweb.Response {
    return response.withHeader("X-Asset-Policy", "applied");
});
let client = gebweb.TestClient(app);
io.println(client.get("/private").status);
io.println(client.get("/assets/missing.css").status);
io.println(client.get("/assets/missing.css").headers["X-Asset-Policy"]);
```

```text
401
404
applied
```

Use `{"middleware": false}` in `useStaticAssets` to bypass those app middleware
chains for assets. That also bypasses their security headers, rate limits and
WAF checks. Request lifecycle hooks still apply. Middleware registered directly
on the underlying stdlib router does not apply to assets. See
[Middleware](09-middleware.md).

## File delivery and containment

Both indexing and delivery resolve symlinks and require the target to remain
inside the configured public root. Paths that escape the configured root return
404, including a previously indexed symlink retargeted outside the root.
Delivery uses the checked canonical path. Keep the directory tree owned by a
trusted deployment account: path checks are not filesystem isolation against
an attacker replacing files or directories while files are being opened.

The manifest is a URL index, not a snapshot of file bytes. Keep production
files immutable throughout a deployment; changing bytes under an old hashed
URL breaks its cache contract. Deploy a new application instance with its new
asset set, and retain old URLs at your proxy/CDN if old pages need them. An
unresolved root needs application setup again after it becomes available.

Only GET and HEAD are accepted; other methods return 405 with `Allow: GET,
HEAD`. Known MIME types include CSS, JavaScript, HTML, JSON, text, SVG, PNG,
JPEG, GIF, WebP, ICO, WOFF, WOFF2 and TTF. Unknown extensions, including XML,
use `application/octet-stream`.

The HTTP server streams files and handles HEAD, byte ranges and conditional
requests. `TestClient` buffers the file for assertions; use a real HTTP server
test to verify transport behavior such as an empty HEAD body or a 206 range.

## Precompressed files

Generate `app.css.gz` after `app.css` during the build, then deploy both.
Enable `precompressed` to serve the sidecar when the client accepts gzip:

<!-- doctest:run -->
```gb
import bytes;
import compress;
import gebweb;
import io;

let original = bytes.fromString("body { color: navy; }");
io.writeBytes("public/app.css", original);
io.writeBytes("public/app.css.gz", compress.gzip(original));
let app = gebweb.app([], {"docs": false});
gebweb.useStaticAssets(app, "public", {
    "fingerprint": false,
    "cacheControl": "no-cache",
    "precompressed": true,
});
let client = gebweb.TestClient(app);
let packed = client.request("GET", "/assets/app.css", null, {"Accept-Encoding": "gzip"});
io.println(packed.headers["Content-Encoding"]);
io.println(packed.headers["Content-Type"]);
io.println(packed.headers["Vary"]);
io.println(compress.gunzip(packed.body as bytes).toString());
let plain = client.request("GET", "/assets/app.css", null, {"Accept-Encoding": "gzip;q=0, *;q=1"});
io.println(plain.headers["Content-Encoding"]);
```

```text
gzip
text/css
Accept-Encoding
body { color: navy; }
null
```

Both variants send `Vary: Accept-Encoding`; gzip retains the source MIME type.
Missing, stale or out-of-root sidecars fall back to the original. On disk,
freshness uses file modification times. Bundled sidecars do not use extraction
times for freshness. This option streams prepared files; it does not compress
files during a request or configure response compression middleware.

## Build pipeline and bundling

`gebweb build` can compile assets and embed them with templates. Configure
`geblang.yaml` at the project root:

```yaml
name: myapp
source: src
assets:
  sourceDir: assets
  outDir: build/assets
  entryPoints:
    - app.ts
    - app.scss
  templatesDir: templates
  publicDir: public
```

| Field | Default | Meaning |
|-------|---------|---------|
| `sourceDir` | No implicit directory | Entry-point paths are relative to this directory. |
| `outDir` | Required when compiling entry points | Destination for compiled files; embedded when present. |
| `entryPoints` | Empty | Files to compile. Empty means no compilation. |
| `templatesDir` | `templates` | Templates to stage and embed when present. |
| `publicDir` | `public` | Additional files to embed unchanged when present. |

JavaScript/JSX/TypeScript/TSX entries are bundled and minified by esbuild into
`outDir/<basename>.js`. CSS is bundled and minified into `<basename>.css`.
Sass entries require `sass` or `dart-sass` on PATH, then their CSS is minified.
Output uses the entry's basename, so give entries distinct output names.
HTML templates are minified in a staging copy with template tags preserved;
the originals are left alone. Other template files are copied.

`--no-minify` disables asset and template minification. `--no-sass` permits
skipping Sass when no compiler is installed; an installed compiler still runs.
Without the flag, missing Sass tooling fails a build that has Sass entries.
Without an `assets` block, existing default `templates` and `public`
directories can still be embedded.

Serve the compiled output, not the source directory:

<!-- doctest:fragment context=app -->
```gb
gebweb.useViews(app, "templates");
gebweb.useStaticAssets(app, "build/assets");
```

The same paths work from source and in a built binary, where the framework
resolves embedded resources through `sys.bundleDir()`. `gebweb dev` compiles
entry points once, unminified; restart it after editing asset sources. Its
source watcher does not rebuild CSS/TypeScript when those files change. See
[CLI](15-cli.md) and [Configuration and deployment](37-deployment.md).

## Reference

The `gebweb.assets` module exposes:

| API | Result |
|-----|--------|
| `newConfig(sourceDir, opts)` | Creates a configuration and builds its production manifest. |
| `assetUrl(cfg, logical)` | Looks up the hashed URL, with a logical-URL fallback for missing names. |
| `serveAsset(cfg, request)` | Handles a request dictionary and returns a response/file marker. |
| `makeAssetHandler(cfg)` | Returns a callable wrapping `serveAsset`; it does not export a manifest file. |

Prefer `useStaticAssets` for an application: it connects routing and view
filters. Use `newConfig` when integrating the lower-level helpers; constructing
`AssetConfig` directly does not build a production manifest.
