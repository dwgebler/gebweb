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
    urlPrefix:    "/assets",                                  // default
    cacheControl: "public, max-age=31536000, immutable",     // default
    fingerprint:  true,                                       // default
    dev:          false                                       // default
});
```

- `urlPrefix` - the public mount point. Routes registered under
  this prefix by your own controllers still work; the asset handler
  only fires when no other route claims the path.
- `cacheControl` - the `Cache-Control` header sent with every asset
  response. The default is appropriate for fingerprinted assets.
- `fingerprint` - set to `false` to serve unhashed names verbatim.
  Useful for legacy paths or when the front-end build system already
  fingerprints.
- `dev` - in dev mode the manifest is recomputed on every request
  if the source file's mtime is newer than the cached hash. The
  raw (un-fingerprinted) URL is also accepted so live-reloading
  works without re-rendering templates.

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
returns the unprefixed input unchanged (defensively, so a typo
shows up as a broken link rather than crashing the render).

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

## Production deployment

In prod, the recommended pattern is:

1. Have your build pipeline (Sass, esbuild, etc.) write final
   files into `public/`.
2. Call `useStaticAssets(app, "public")` once at startup; the
   manifest is built before the first request lands.
3. Put a reverse proxy or CDN in front for further caching; the
   `immutable` directive makes this safe at any depth.

For static-only sites, you can also pre-generate the manifest
offline and skip the framework entirely. The fingerprinting logic
lives in `src/assets.gb` and the helper functions are
exported, so a build script can produce the same hashes the
framework would.
