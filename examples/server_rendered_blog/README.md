# Server-rendered blog

Tiny in-memory blog that exercises every server-rendered UX feature
in Gebweb: CSRF, flash messages, form-state rehydration,
fingerprinted static assets, and CSP with a per-request nonce.

Each feature has its own dedicated test in the framework's `tests/`
suite; this sample is a readable reference showing how the pieces
wire together in one place.

## Run

```sh
geblang main.gb
```

Open http://127.0.0.1:8080 in a browser.

## How the pieces fit

Reading `main.gb` top-to-bottom:

1. `Post` and `PostForm` are POJO model + validated DTO.
2. `BlogController` exposes a `GET /` list page and a
   `POST /posts` create handler. The create handler accepts a
   `PostForm`; `@Assert.*` on its fields drives the validation.
3. The wiring block at the bottom registers, in order, the view
   engine, the session store, CSRF protection, the static-asset
   pipeline, and the security-headers + CSP-with-nonce policy.
4. `gebweb.htmlView(app, request, ...)` threads the request
   into the template, which is what makes `{{ csrf }}`,
   `{{ flashes }}`, `{{ old }}`, `{{ errors }}`, and
   `{{ cspNonce }}` available without per-handler boilerplate.

## What to try

- Submit the form with the title field empty. The page redirects
  back, the title input is highlighted, and the previously typed
  body text is preserved.
- Submit a valid post. A green flash banner confirms success on
  the next page.
- View page source: the stylesheet `<link>` points at a content
  -hashed URL like `/assets/app-XXXXXXXX.css`. The inline
  `<script>` carries a fresh `nonce` matched by the CSP header.
- Run `curl -i http://127.0.0.1:8080/` to see all five security
  headers + the CSP nonce string per request.

## Try with curl

```sh
# CSRF token round-trip: GET to mint, POST to consume
curl -c cookies.txt -s http://127.0.0.1:8080/ > /dev/null
TOKEN=$(curl -b cookies.txt -s http://127.0.0.1:8080/ | grep -oP 'name="_csrf" value="\K[^"]+')
curl -b cookies.txt -c cookies.txt -i \
  -d "_csrf=${TOKEN}&title=hello&body=world" \
  http://127.0.0.1:8080/posts
```

The first POST returns 303 (HTML client redirect). Follow the
Location to see the flash + the newly created post in the list.
