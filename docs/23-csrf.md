# CSRF protection

<!-- doctest:strict -->

Cross-Site Request Forgery protection checks that a browser submitting a
state-changing request also supplies the application's CSRF token. Enable it
with `gebweb.useCsrf(app, opts)`. It is opt-in: without that call, the CSRF
wrapper passes requests through.

Gebweb controller routes and generated resource routes check POST, PUT, PATCH
and DELETE unless marked `@CsrfExempt`. GET, HEAD and OPTIONS do not require a
submitted token. Keep state changes out of those safe methods.

## A protected form

Put this template in `templates/posts/form.html`:

<!-- doctest:file templates/posts/form.html -->
```html
<form method="post" action="/posts">
    <input type="hidden" name="_csrf" value="{{ csrf }}">
    <input name="title" required>
    <button>Publish</button>
</form>
```

The app below renders the form and accepts a typed, validated body. The upload
route will also be used later in this chapter. Put it in `src/post_forms.gb`:

<!-- doctest:file post_forms.gb -->
```gb
import gebweb;

class PostInput {
    @Assert.notBlank
    string title;
}

class SavedPost {
    string title;

    func SavedPost(string title) {
        this.title = title;
    }
}

class UploadReceipt {
    int size;

    func UploadReceipt(int size) {
        this.size = size;
    }
}

@Controller("/posts")
class PostController extends gebweb.Controller {
    @Get("")
    func form(gebweb.Request request): gebweb.Response {
        return this.view(request, "posts/form.html", {});
    }

    @Post("")
    func submit(PostInput body): SavedPost {
        return SavedPost(body.title);
    }

    @Post("/upload")
    @MaxBody(1048576)
    func upload(dict<string, gebweb.UploadedFile> files): UploadReceipt {
        if (!files.contains("document")) {
            throw gebweb.badRequest("document is required");
        }
        return UploadReceipt(files["document"].size());
    }
}

export func build(): gebweb.GebwebApp {
    let app = gebweb.app();
    gebweb.useViews(app, "templates");
    gebweb.useCsrf(app, {"secret": "example-only-csrf-signing-secret"});
    return app;
}
```

The signing secret above is only for the example. Load a dedicated, stable
secret from application configuration in production. Configure CSRF during
`build(conn)` alongside views and other setup, before serving with `gebweb.cli`;
see [Getting started](01-getting-started.md).

For tests, extract only the CSRF cookie's `name=value` pair from `Set-Cookie`.
This helper handles both string and list response headers; put it in
`src/csrf_test_helpers.gb`:

<!-- doctest:file csrf_test_helpers.gb -->
```gb
export func cookiePair(any header, string name = "geb_csrf"): string {
    let entries = header instanceof list ? header as list<any> : [header];
    for (entry in entries) {
        string first = (entry as string).split(";")[0];
        if (first.startsWith(name + "=")) {
            return first;
        }
    }
    throw RuntimeError("CSRF cookie missing");
}
```

`TestClient` does not keep a browser cookie jar. The test must send the cookie
and the hidden-field value explicitly:

<!-- doctest:run -->
```gb
import csrf_test_helpers;
import encoding;
import gebweb;
import io;
import post_forms;

let client = gebweb.TestClient(post_forms.build());
let page = client.get("/posts");
string cookie = csrf_test_helpers.cookiePair(page.headers["Set-Cookie"]);
string token = cookie.substring("geb_csrf=".length());
io.println(page.status);
io.println(page.text().contains('name="_csrf" value="' + token + '"'));

let missing = client.post("/posts", {"title": "Notebook"});
io.println(missing.status);
io.println(missing.text());
let saved = client.request("POST", "/posts", "title=Notebook&_csrf=" + encoding.urlEncode(token), {
    "Content-Type": "application/x-www-form-urlencoded",
    "Cookie": cookie,
});
io.println(saved.status);
io.println(saved.json()["title"]);
```

```text
200
true
403
CSRF token invalid
200
Notebook
```

## Token flow and validation

1. A wrapped route reads the configured cookie. When it is empty, the wrapper
   creates a JWT signed with `secret`, containing a random `csrf` value.
2. The token is available to that request as the view-context value `csrf`.
   A successful wrapped response sets a fresh cookie when one was needed.
3. For POST/PUT/PATCH/DELETE, validation requires an existing cookie whose JWT
   signature verifies and a matching submitted token. Comparison is constant-time.
4. A non-empty `X-CSRF-Token` header takes precedence. Otherwise validation
   reads `_csrf` from an URL-encoded or multipart form. A `_csrf` key in a JSON
   body is not a form token.

Missing or invalid tokens return a plain-text 403 with body `CSRF token invalid`
before the route handler runs. The early rejection does not issue the freshly
minted cookie, so start by loading a successful protected GET page rather than
expecting an initial failed POST to establish the token.

A nonempty cookie is reused on subsequent requests, not rotated on every GET.
The token is not bound to an authentication session or user, and the issued JWT
has no automatic expiry claim. Cookie `MaxAge` controls browser retention, not
a server-side token expiry. Rotating the secret invalidates existing signatures;
nonempty stale cookies are not automatically replaced by a safe request, so
account for clearing them during such a change.

The wrapper applies to controller/resource routes, not every arbitrary route
registered directly on a native router. `useCsrf` can be configured before or
after those framework routes are registered because they look up the active
configuration at request time. Configure it before accepting traffic.

## JSON requests and JavaScript

A JSON request sends the token in a header, together with its cookie. A header
with a wrong value is rejected even when the cookie is valid:

<!-- doctest:run -->
```gb
import csrf_test_helpers;
import gebweb;
import io;
import post_forms;

let client = gebweb.TestClient(post_forms.build());
let page = client.get("/posts");
string cookie = csrf_test_helpers.cookiePair(page.headers["Set-Cookie"]);
string token = cookie.substring("geb_csrf=".length());
let valid = client.request("POST", "/posts", {"title": "JSON post"}, {
    "Cookie": cookie,
    "X-CSRF-Token": token,
});
let wrong = client.request("POST", "/posts", {"title": "Wrong token"}, {
    "Cookie": cookie,
    "X-CSRF-Token": "wrong",
});
let noCookie = client.request("POST", "/posts", {"title": "Missing cookie"}, {
    "X-CSRF-Token": token,
});
io.println(valid.status);
io.println(wrong.status);
io.println(noCookie.status);
```

```text
200
403
403
```

The cookie defaults to HttpOnly, so JavaScript should obtain the token from the
rendered page, not `document.cookie`. For example, on a page using the form
above, same-origin code can read its hidden field:

```html
<script>
document.querySelector("form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    const token = form.querySelector('input[name="_csrf"]').value;
    try {
        const response = await fetch("/posts", {
            method: "POST",
            credentials: "same-origin",
            headers: {
                "Content-Type": "application/json",
                "X-CSRF-Token": token
            },
            body: JSON.stringify({title: form.elements.namedItem("title").value})
        });
        if (!response.ok) {
            throw new Error(`Publish failed: ${response.status}`);
        }
        const saved = await response.json();
        console.log(saved.title);
    } catch (error) {
        console.error(error);
    }
});
</script>
```

Keep the token out of URLs and logs. The cookie and submitted value must both
reach the same application; changing cookie domains, HTTPS settings or fetch
credentials can prevent that round trip. A browser integration test is still
needed for those deployment settings: `TestClient` sends the supplied headers
and does not enforce browser cookie policy.

## Multipart upload forms

Use the same hidden `_csrf` field in a form with
`enctype="multipart/form-data"`. It is read alongside the file parts; it does
not have to be moved into a query parameter or exempted from protection.

This example submits the upload route from `post_forms` with binary content:

<!-- doctest:run -->
```gb
import bytes;
import csrf_test_helpers;
import gebweb;
import io;
import post_forms;

let client = gebweb.TestClient(post_forms.build());
string cookie = csrf_test_helpers.cookiePair(client.get("/posts").headers["Set-Cookie"]);
string token = cookie.substring("geb_csrf=".length());
let filePart = {
    "filename": "sample.bin",
    "contentType": "application/octet-stream",
    "content": bytes.fromHex("00ff01"),
};
let missing = client.multipart("/posts/upload", {"document": filePart}, {"Cookie": cookie});
let valid = client.multipart("/posts/upload", {"document": filePart, "_csrf": token}, {
    "Cookie": cookie,
});
io.println(missing.status);
io.println(valid.status);
io.println(valid.json()["size"]);
```

```text
403
200
3
```

CSRF acceptance does not validate the file's content. Apply upload size, format,
authorization and storage-name checks separately; see [File storage](21-storage.md).

## Exempt routes

Place `@CsrfExempt` on a method to exempt that route, or on a controller class
to exempt its routes. Generated `@ApiResource` routes honor the decorator on
the resource class.

Exemption does not authenticate the caller. This small callback example uses a
separate shared-token check; a real provider integration must verify its own
signature or authentication protocol:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import secrets;

@CsrfExempt
@Controller("/callback")
class CallbackController {
    @Post("")
    func accept(@Header("X-Callback-Token") ?string token): dict<string, bool> {
        if (!secrets.constantTimeEqual(token ?? "", "example-callback-token")) {
            throw gebweb.unauthorized("callback token required");
        }
        return {"accepted": true};
    }
}

let app = gebweb.app();
gebweb.useCsrf(app, {"secret": "example-only-csrf-signing-secret"});
let client = gebweb.TestClient(app);
io.println(client.post("/callback", null).status);
io.println(
    client.request("POST", "/callback", null, {"X-Callback-Token": "example-callback-token"}).status
);
```

```text
401
200
```

Use narrow exemptions for routes protected by an appropriate independent
mechanism. Do not exempt a browser session-authenticated route merely because it
accepts JSON. Exempt routes bypass token handling, so they do not themselves
seed a CSRF cookie for another protected form.

## Configuration

| Option | Default | Effect |
|--------|---------|--------|
| `secret` | Required | JWT signing secret, shared by app instances validating the same cookies. |
| `cookieName` | `"geb_csrf"` | Cookie used for issuance and validation; the form/header names stay unchanged. |
| `cookieOptions` | `{}` | Cookie attributes listed below, with exact case-sensitive keys. |

| Cookie option | Default | Effect |
|---------------|---------|--------|
| `SameSite` | `"Lax"` | Attribute written to the cookie, such as `Lax`, `Strict` or `None`. |
| `Secure` | `false` | Restrict browser transmission to HTTPS when enabled. |
| `HttpOnly` | `true` | Keep the cookie inaccessible through JavaScript. |
| `MaxAge` | Omitted | Retention in seconds; omitted produces a session cookie. |
| `Domain` | Omitted | Cookie domain; omitted leaves a host-only cookie. |

The cookie path is always `/`; `Path` is not a configurable attribute here.
For an HTTPS application, load the secret and explicitly enable `Secure`:

<!-- doctest:fragment context=app -->
```gb
let secret = sys.getenv("CSRF_SECRET");
if (secret == null || (secret as string).isEmpty()) {
    throw ValueError("CSRF_SECRET is required");
}
gebweb.useCsrf(app, {
    "secret": secret as string,
    "cookieName": "app_csrf",
    "cookieOptions": {"SameSite": "Lax", "Secure": true, "HttpOnly": true},
});
```

Changing `cookieName` does not rename `_csrf` or `X-CSRF-Token`. Keep the signing
secret separate from other token protocols. Avoid widening the cookie domain
across hosts that do not share the same trust boundary.

CSRF tokens complement authentication, authorization and XSS prevention; they
do not replace them. SameSite cookies are an additional browser defense, not a
reason to remove checks from cookie-authenticated writes. For the wider threat
model and alternative token designs, see the
[OWASP CSRF guidance](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html).

## OpenAPI and SwaggerUI

When CSRF is enabled, the SwaggerUI page at `/docs` issues the CSRF cookie and
embeds an interceptor that sends `X-CSRF-Token` for "Try it out" requests.
A docs guard that rejects access does not issue that cookie. Protecting the docs
and choosing which routes are exempt remain separate decisions; see
[OpenAPI](13-openapi.md).

## Reference

| Surface | Contract |
|---------|----------|
| `gebweb.useCsrf(app, opts)` | Configure token handling for wrapped framework routes; return the app. |
| `{{ csrf }}` | Current request token in view context. |
| `_csrf` | Submitted hidden field for URL-encoded or multipart forms. |
| `X-CSRF-Token` | Submitted header for JSON/Ajax or other writes. |
| `@CsrfExempt` | Exempt one handler, a controller or a resource class. |

Related chapters: [Views and templates](04a-views.md),
[Testing](14-testing.md), [File storage](21-storage.md),
[Forms](25-forms.md), [Security](27-security.md),
[Authentication](40-authentication.md), and [OpenAPI](13-openapi.md).
