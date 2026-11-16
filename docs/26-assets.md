# Asset pipeline

`gebweb.useStaticAssets(app, dir, opts?)` mounts a static-file
serving directory under a URL prefix and gives templates a content
-hashed `asset()` filter for cache-busting.

A fingerprinted URL like `/assets/app-7b2f9c4d.css` carries an
eight-character SHA-256 hash of the file content. When the file
changes, the hash changes, so the URL changes, so browsers and CDNs
treat it as a fresh resource. That is the whole reason fingerprinting
exists: it makes `Cache-Control: max-age=31536000, immutable` safe.

## Setup

```gb
import gebweb;

let app = gebweb.app([HomeController()]);
gebweb.useViews(app, "templates");
gebweb.useStaticAssets(app, "public");
```

By default the directory is walked once at startup. Every file gets
hashed and indexed under a manifest so the URL helper is a single
dict lookup at render time.

## Options

```gb
gebweb.useStaticAssets(app, "public", {
    "urlPrefix":    "/assets",
    "cacheControl": "public, max-age=31536000, immutable",
    "fingerprint":  true,
    "dev":          false,
    "middleware":   true
});
```

- `urlPrefix` - the public mount point. Every request under this
  prefix is answered by the asset handler instead of controller
  routing (a missing file is a `404`), so avoid registering controller
  routes under the asset prefix.
- `cacheControl` - a header string, or `func(string relativePath): string`.
  The callback receives the requested path relative to the asset URL prefix.
  Use it to give independently hashed subdirectories a longer lifetime. It runs
  only for successful asset resolution. The default is appropriate for
  fingerprinted assets.
- `precompressed` - opt in to serving a neighboring `.gz` file when the client
  accepts gzip and the sidecar is at least as new as the source. The original
  MIME type is retained, both variants send `Vary: Accept-Encoding`, and gzip
  responses set `Content-Encoding: gzip`. Missing, stale or out-of-root sidecars
  fall back to the original. Generate sidecars during your build, after their
  source files, and embed/deploy both. Bundled resources are immutable and
  do not use extraction timestamps for freshness. This streams existing compressed files;
  it does not compress files on a request or change response middleware.
- `fingerprint` - set to `false` to serve unhashed names verbatim.
  Useful for legacy paths or when the front-end build system already
  fingerprints.
- `dev` - in dev mode `asset` returns the plain logical URL (no
  content hash), which maps straight to the source file on disk, so
  edits are picked up live without recomputing a manifest or
  re-rendering templates.
- `middleware` - set to `false` to serve assets without running the
  app's middleware (see below).

## Middleware

Asset requests run the app's `gebweb.before` and `gebweb.use` /
`gebweb.after` middleware exactly like routed requests: before
middleware in registration order, stopping at the first that returns a
response, then after middleware in reverse order on the response. So
an app-wide login gate, rate limit or WAF also covers asset URLs, and
security headers are added to asset responses, including `404`s.

If a before gate rejects signed-out users, it now rejects their CSS
and JavaScript too. Let asset paths through in the gate:

```gb
gebweb.before(app, func(gebweb.Request req): any {
    if (req.path().startsWith("/assets/")) {
        return null;
    }
    return requireSignedIn(req);
});
```

or serve assets that are always public without any middleware:

```gb
gebweb.useStaticAssets(app, "public", {"middleware": false});
```

With `"middleware": false` no app middleware runs for asset requests,
so they get no security headers, rate limiting or WAF checks either.
Middleware registered directly on the underlying router
(`gebweb.router(app)`) never applies to assets.

In dev and non-fingerprint mode the handler resolves the requested file,
following symlinks, and refuses anything that escapes the configured
asset root with a `404`. A symlink inside the root that points outside it,
or a `..` traversal, is rejected; a filename that merely contains `..`
(for example `my..file.css`) is served normally. A path that cannot be
resolved at all (a removed symlink target, or an asset root that does not
resolve) also returns `404` rather than erroring. Fingerprint mode already
serves only files in the manifest built at startup, so it is unaffected.

## Rendering URLs

The `asset` filter is auto-registered as a view-engine filter when
you call `useStaticAssets`. In a template:

```twig
<link rel="stylesheet" href="{{ 'app.css' | asset }}">
<img src="{{ 'img/logo.png' | asset }}">
```

Output:

```html
<link rel="stylesheet" href="/assets/app-7b2f9c4d.css">
<img src="/assets/img/logo-a1b2c3d4.png">
```

If the requested logical name is not in the manifest, the filter
returns the name under the mount prefix (`urlPrefix` + name), so a
typo shows up as a broken link under the asset path rather than
crashing the render.

## Subdirectories

Directory structure is preserved in the URL. `public/img/logo.png`
maps to `/assets/img/logo-<hash>.png`. The full relative path is
the manifest key, so name collisions across directories are
impossible by design.

## Content types

The handler guesses the `Content-Type` header from the file
extension. Common web extensions (`.css`, `.js`, `.html`, `.svg`,
`.png`, `.jpg`, `.gif`, `.webp`, `.ico`, `.woff`, `.woff2`, `.json`,
`.txt`, `.xml`) are recognised. Unknown extensions fall back to
`application/octet-stream`.

Assets stream straight from disk rather than being buffered in memory,
and the server answers `HEAD`, `Range` (`206 Partial Content`), and
conditional (`If-None-Match` / `If-Modified-Since`, `304 Not Modified`)
requests automatically alongside the long-cache header.

## Build pipeline and bundling

`gebweb build` can compile, minify, and embed your assets and templates into the
single-binary release so the deployed binary is self-contained. Declare an
`assets:` block in `geblang.yaml`:

```yaml
name: myapp
source: src
assets:
  sourceDir: assets        # raw JS/TS/JSX/CSS/SCSS sources
  outDir: build/assets     # compiled output (this is what you serve)
  entryPoints:
    - app.ts
    - app.scss
  templatesDir: templates  # optional, default "templates"
  publicDir: public        # optional, default "public"
```

At build time each entry point is processed:

- `.js` / `.jsx` / `.ts` / `.tsx` are bundled, tree-shaken, and minified with
  esbuild into `outDir/<name>.js`.
- `.css` is bundled (resolving `@import`) and minified with esbuild into
  `outDir/<name>.css`.
- `.scss` / `.sass` are compiled with dart-sass (must be on `PATH`), then
  minified, into `outDir/<name>.css`.
- HTML templates under `templatesDir` are minified (template tags such as
  `{{ }}` and `{% %}` are preserved).

The compiled `outDir`, the (minified) templates, and `publicDir` are then
embedded in the binary. At run time the framework resolves them through
`sys.bundleDir()`: a built binary serves the embedded copies, while `gebweb dev`
and a normal `geblang` run serve the files from disk unchanged. Your code path
is identical in both cases:

```gb
gebweb.useViews(app, "templates");
gebweb.useStaticAssets(app, "build/assets");
```

Flags:

- `--no-minify` skips minification (assets and templates) for faster, readable
  builds.
- `--no-sass` skips SASS compilation when dart-sass is not installed. Without
  it, a `.scss`/`.sass` entry point with no dart-sass on `PATH` fails the build
  with an actionable message.

`gebweb dev` compiles the asset entry points once (unminified) so the dev server
serves them from disk; restart the dev server to recompile after editing an
asset source.

## Production deployment

The simplest production path is `gebweb build`: the asset pipeline above
compiles, minifies, and embeds everything, and you ship one binary.

If you prefer to manage assets yourself:

1. Have your build pipeline (Sass, esbuild, etc.) write final
   files into `public/`.
2. Call `useStaticAssets(app, "public")` once at startup; the
   manifest is built before the first request lands.
3. Put a reverse proxy or CDN in front for further caching; the
   `immutable` directive makes this safe at any depth.

For static-only sites, you can also pre-generate the manifest
offline and skip the framework entirely. The fingerprinting logic
is exposed as three helpers in `gebweb.assets`:

- `assets.newConfig(sourceDir, opts)` builds an `AssetConfig`
  with the manifest computed.
- `assets.assetUrl(cfg, "app.css")` returns the fingerprinted
  URL the same way the view filter does.
- `assets.makeAssetHandler(cfg)` returns the route handler the
  framework mounts. A custom build script can call this directly
  to produce a static manifest file.
