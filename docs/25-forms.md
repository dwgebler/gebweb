# Forms

<!-- doctest:strict -->

Typed body parameters use the same validation pipeline for browser forms and
JSON requests. When an `@Assert` rule fails, HTML clients with a session store
get a 303 redirect with submitted fields and errors saved for the next view.
JSON clients get a 422 Problem Details response without that session write.

## A complete form round trip

Put this template in `templates/contact.html`. Values are HTML-escaped; keep
that escaping when rendering submitted input:

<!-- doctest:file templates/contact.html -->
```html
{% for message in flashes.success %}<p class="success">{{ message }}</p>{% endfor %}
{% for message in flashes.error %}<p class="error">{{ message }}</p>{% endfor %}
<form method="post" action="/contact">
    <label>Name <input name="name" value="{{ old.name | default('') }}"></label>
    {% if errors.name %}<p class="error">{{ errors.name | first }}</p>{% endif %}
    <label>Email <input name="email" type="email" value="{{ old.email | default('') }}"></label>
    {% if errors.email %}<p class="error">{{ errors.email | first }}</p>{% endif %}
    <input type="hidden" name="_csrf" value="{{ csrf }}">
    <button>Submit</button>
</form>
```

The application acknowledges a valid submission. Its reserved-address branch
will demonstrate an explicit business-rule error later. Put it in
`src/contact_app.gb`:

<!-- doctest:file contact_app.gb -->
```gb
import gebweb;
import gebweb.app as appmod;
import gebweb.forms as forms;
import web.session as session;

class ContactInput {
    @Assert.notBlank
    string name;
    @Assert.email
    string email;
}

@Controller("/contact")
class ContactController extends gebweb.Controller {
    gebweb.GebwebApp app;

    func ContactController(gebweb.GebwebApp app) {
        parent();
        this.app = app;
    }

    @Get("")
    func page(gebweb.Request request): gebweb.Response {
        return this.view(request, "contact.html", {});
    }

    @Post("")
    func submit(gebweb.Request request, ContactInput body): dict<string, any> {
        if (body.email == "reserved@example.test") {
            let sessions = appmod.session(this.app);
            let response = this.redirect("/contact", 303).toDict();
            response = forms.preserveInput(sessions, response, request.toDict());
            response = forms.withErrors(sessions, response, request.toDict(), {
                "email": ["This address is reserved."],
            });
            return this.flash(request, response, "error", "Please choose another address.");
        }
        return this.redirectWithFlash(request, "/contact", "success", "Thanks, " + body.name + "!");
    }
}

export func build(string sessionDirectory): gebweb.GebwebApp {
    let app = gebweb.app();
    gebweb.useViews(app, "templates");
    gebweb.useSession(app, session.fileSessionStore(sessionDirectory, 3600));
    gebweb.useCsrf(app, {"secret": "example-only-contact-csrf-secret"});
    return app;
}
```

Use an application-owned persistent session location, a dedicated configured
CSRF secret and a consistent cookie policy in deployment. The temporary store
and literal secret are for these tests. See [Flash messages](24-flash.md) for a
store wrapper applying cookie policy on every save, and [CSRF](23-csrf.md) for
production token configuration. Put application setup in `build(conn)` and serve
through the [standard entry point](01-getting-started.md).

`TestClient` has no cookie jar. This helper carries both CSRF and session
cookies through redirects without putting their random values in test output.
Put it in `src/form_cookies.gb`:

<!-- doctest:file form_cookies.gb -->
```gb
export func apply(dict<string, string> cookies, dict<string, any> headers): void {
    if (!headers.contains("Set-Cookie")) {
        return;
    }
    let raw = headers["Set-Cookie"];
    let entries = raw instanceof list ? raw as list<any> : [raw];
    for (entry in entries) {
        string pair = (entry as string).split(";")[0];
        int equal = pair.indexOf("=");
        if (equal > 0) {
            cookies[pair.substring(0, equal)] = pair.substring(equal + 1);
        }
    }
}

export func header(dict<string, string> cookies): string {
    list<string> pairs = [];
    for (name, value in cookies) {
        pairs.push(name + "=" + value);
    }
    return pairs.join("; ");
}
```

Load the form to obtain its CSRF cookie, submit invalid data, then follow the
redirect with the updated cookies:

<!-- doctest:run -->
```gb
import contact_app;
import encoding;
import form_cookies;
import gebweb;
import io;

let client = gebweb.TestClient(contact_app.build(io.tempDir("contact-form-")));
dict<string, string> cookies = {};
form_cookies.apply(cookies, client.get("/contact").headers);
let failed = client.request(
    "POST",
    "/contact",
    "name=Mara+Smith&email=invalid-email&_csrf=" + encoding.urlEncode(cookies["geb_csrf"]),
    {
        "Accept": "text/html",
        "Content-Type": "application/x-www-form-urlencoded",
        "Referer": "/contact",
        "Cookie": form_cookies.header(cookies),
    }
);
form_cookies.apply(cookies, failed.headers);
io.println(failed.status);
io.println(failed.headers["Location"]);
let first = client.request("GET", "/contact", null, {"Cookie": form_cookies.header(cookies)});
io.println(first.text().contains('value="Mara Smith"'));
io.println(first.text().contains('value="invalid-email"'));
io.println(first.text().contains("must be a valid email"));
form_cookies.apply(cookies, first.headers);
let second = client.request("GET", "/contact", null, {"Cookie": form_cookies.header(cookies)});
io.println(second.text().contains("Mara Smith"));
io.println(second.text().contains("must be a valid email"));
```

```text
303
/contact
true
true
true
false
false
```

The handler did not run for that invalid DTO: validation failed before invocation.
The redirect saved input and errors together, and the request-aware view read
and consumed them. A browser's own input validation can improve feedback, but
the server still validates requests from clients that bypass it.

## Validation and response selection

Automatic rehydration handles `UnprocessableEntityError`, including failed body
assertions. Conversion or malformed-body errors remain their normal error
responses, such as 400; CSRF rejection remains 403. The form mechanism does not
turn every failed request into a redirect.

| Request / configuration | Validation response |
|-------------------------|---------------------|
| HTML preferred, session configured | 303 with input/error state saved in session. |
| HTML preferred, no session configured | Normal 422 Problem Details. |
| JSON preferred, no Accept header or no HTML media type | Normal 422 Problem Details; no automatic form-state save. |

The current Accept check searches for the literal `text/html` and
`application/json` strings. HTML wins when it appears before JSON, or when HTML
is present and JSON is absent. It does not evaluate quality weights (`q=`) as a
full HTTP negotiation algorithm. Set the intended type explicitly:

| Accept value | Selection |
|--------------|-----------|
| `text/html` | HTML redirect, with a configured session. |
| `application/json` | JSON 422. |
| `text/html, application/json` | HTML first. |
| `application/json, text/html` | JSON first. |
| Missing or `*/*` | JSON 422. |

The redirect uses `Referer` when present, otherwise the submitted request path,
with `/` as a final fallback. It uses that value directly; it does not validate
the destination's origin. Use an explicit application-chosen target in custom
flows that require stricter redirect control. The fallback path needs a GET
route that can render the form.

## JSON callers

Even on the same protected endpoint, a JSON caller requests a 422 response by
setting `Accept: application/json`. It still needs the CSRF header and cookie
because this app enabled CSRF:

<!-- doctest:run -->
```gb
import contact_app;
import form_cookies;
import gebweb;
import io;

let client = gebweb.TestClient(contact_app.build(io.tempDir("contact-json-")));
dict<string, string> cookies = {};
form_cookies.apply(cookies, client.get("/contact").headers);
let response = client.request("POST", "/contact", {"name": "", "email": "invalid-email"}, {
    "Accept": "application/json",
    "Cookie": form_cookies.header(cookies),
    "X-CSRF-Token": cookies["geb_csrf"],
});
io.println(response.status);
io.println(response.json().contains("errors"));
io.println(response.headers.contains("Set-Cookie"));
```

```text
422
true
false
```

That last result means this validation response did not save form state. It is
not a general promise that all middleware or authentication code avoids cookies.
The request's Content-Type determines body decoding; Accept chooses this
validation response. JSON object bodies can also be preserved when an HTML
response is selected.

## Old input, errors and templates

| View context | Type / meaning |
|--------------|----------------|
| `old` | `dict<string, any>` of submitted fields. |
| `errors` | `dict<string, list<string>>` grouped by field name. |
| `flashes` | Separate categorized notices; see [Flash messages](24-flash.md). |
| `csrf` | Current token when CSRF is configured. |

Supported old-input sources are a request form dictionary, decoded URL-encoded
fields, JSON objects and multipart text fields. Preserved input is the submitted
representation, not a copy of the successfully converted DTO: for example,
`"12"` from a text field can remain a string in `old`.

Automatic preservation copies the extracted fields without a password/secret
redaction list. Design sensitive forms with an appropriate filtering store or
custom validation response flow, and render only the fields intended for
redisplay. Do not log or retain whole input snapshots unnecessarily.

Request-aware `this.view(request, ...)`, `gebweb.htmlView` or the request form
of `gebweb.view` runs the injectors. The session response middleware then clears
both maps. Keep the newest session cookie for the next request. A JSON response
or a render without a request does not perform that automatic flow.

A render can consume state even if its template omits the corresponding inputs.
Explicit `old`/`errors` context values override their injectors. This is one-shot
state for a normal redirect flow, not a guarantee that concurrent requests or
replayed old cookies cannot see the same state.

## Business-rule errors and flash composition

A business rule evaluated inside a handler runs only after body binding and
validation pass. The reserved-address branch in `ContactController` explicitly
chooses a redirect, preserves input, adds a field error and appends a banner.
Those helpers compose through the latest session snapshot in the response.

<!-- doctest:run -->
```gb
import contact_app;
import form_cookies;
import gebweb;
import io;

let client = gebweb.TestClient(contact_app.build(io.tempDir("contact-business-")));
dict<string, string> cookies = {};
form_cookies.apply(cookies, client.get("/contact").headers);
let redirect = client.request(
    "POST",
    "/contact",
    {"name": "Mara", "email": "reserved@example.test"},
    {
        "Accept": "text/html",
        "Cookie": form_cookies.header(cookies),
        "X-CSRF-Token": cookies["geb_csrf"],
    }
);
form_cookies.apply(cookies, redirect.headers);
let page = client.request("GET", "/contact", null, {"Cookie": form_cookies.header(cookies)});
io.println(redirect.status);
io.println(page.text().contains('value="Mara"'));
io.println(page.text().contains("This address is reserved."));
io.println(page.text().contains("Please choose another address."));
form_cookies.apply(cookies, page.headers);
let again = client.request("GET", "/contact", null, {"Cookie": form_cookies.header(cookies)});
io.println(again.text().contains("This address is reserved."));
io.println(again.text().contains("Please choose another address."));
```

```text
303
true
true
true
false
false
```

This manual branch does not automatically select a JSON response just because
a caller sends an API Accept header. Implement the intended business-error
response for each client explicitly; automatic negotiation above concerns
validation exceptions.

## Multipart forms

Use `enctype="multipart/form-data"` for file inputs and include the same hidden
CSRF field. DTO text fields bind and validate beside the upload map. On an HTML
validation failure, their text values and errors survive the redirect, while
file parts are excluded from old-input state.

This exercises the same endpoint's DTO with a multipart body, including a file
that should never be stored in its form-state snapshot:

<!-- doctest:run -->
```gb
import bytes;
import contact_app;
import form_cookies;
import gebweb;
import io;

let client = gebweb.TestClient(contact_app.build(io.tempDir("contact-multipart-")));
dict<string, string> cookies = {};
form_cookies.apply(cookies, client.get("/contact").headers);
let redirect = client.multipart(
    "/contact",
    {
        "name": "Upload User",
        "email": "invalid-email",
        "_csrf": cookies["geb_csrf"],
        "document": {"filename": "sample.bin", "content": bytes.fromHex("00ff01")},
    },
    {"Accept": "text/html", "Referer": "/contact", "Cookie": form_cookies.header(cookies)}
);
form_cookies.apply(cookies, redirect.headers);
let page = client.request("GET", "/contact", null, {"Cookie": form_cookies.header(cookies)});
io.println(redirect.status);
io.println(page.text().contains('value="Upload User"'));
io.println(page.text().contains("must be a valid email"));
```

```text
303
true
true
```

The handler here does not consume the file map; see [File storage](21-storage.md)
for a typed upload handler. A browser file selection is not restored from the
session: ask the user to select the file again after a failed submission. Text
rehydration is not temporary file persistence or an upload workflow manager.

## Manual helpers and scope

Import `gebweb.forms as forms` and pass raw request/response dictionaries. With
rich objects, use `request.toDict()` and `response.toDict()` at this boundary.

| Helper | Contract |
|--------|----------|
| `forms.preserveInput(store, response, request)` | Save extracted fields as old input; return the updated response. |
| `forms.withErrors(store, response, request, fieldErrors)` | Save a `dict<string, list<string>>` error map; return the updated response. |
| `forms.currentOldInput(store, request)` | Read saved input without consuming it. |
| `forms.currentErrors(store, request)` | Read saved field errors without consuming them. |
| `forms.consume(store, response, request)` | Remove both maps while preserving other outgoing session data. |

There is no form options dictionary or wizard state machine. The configured
session store supplies persistence, expiry and cookie behavior. Set up sessions
before accepting requests; the route adapter reads the active store when a
validation failure occurs. `useSessionAuth` also installs these view-state hooks;
composed session authentication still needs explicit `useSession` when using
form/flash views.

For a multi-step form, use application-owned persistent state with an explicit
completion/expiry policy. One-shot old input is not a durable draft, a transaction
or the source of truth for an accepted submission.

## Reference

| Surface | Purpose |
|---------|---------|
| Typed body DTO + `@Assert.*` | Server-side binding and validation. |
| `gebweb.useSession(app, store)` | Enable session-backed old input/errors and their view injectors. |
| `old`, `errors` | One-shot context for a request-aware view render. |
| `gebweb.forms` helpers | Explicit preservation, error assignment and consumption. |
| `Controller.redirectWithFlash` | Success or business-error notice through a redirect. |

Related chapters: [Parameter binding](03-parameter-binding.md),
[Views and templates](04a-views.md), [Validation](05-validation.md),
[Testing](14-testing.md), [File storage](21-storage.md), [CSRF](23-csrf.md),
[Flash messages](24-flash.md), and [Authentication](40-authentication.md).
