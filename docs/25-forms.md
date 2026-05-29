# Forms

When a typed handler parameter carries `@Assert.*` annotations and a
request fails validation, the framework decides how to respond by
content negotiation:

- **HTML clients** (Accept header preferring `text/html`) get a
  **303 redirect** back to the submitting page. The submitted body
  and the per-field error map are stashed in the session. The next
  render reads them via the auto-injected `old` and `errors`
  context keys and consumes them.
- **JSON / API clients** keep the existing **422 Problem-Details**
  response with the validation error list. No session writes.

This delivers two ergonomic UXes from one validation pipeline, with
no per-handler boilerplate.

## Setup

Form rehydration uses the session store, so register one:

```gb
import gebweb;
import web.session as session;
import io;

let app = gebweb.app([SignupController()]);
gebweb.useViews(app, "templates");
gebweb.useSession(app, session.fileSessionStore(io.tempDir("app-sess-*"), 3600));
```

That's it - rehydration is on for every route whose typed body
carries `@Assert.*` annotations.

## Re-rendering with old input + errors

After a failed POST that redirected, the next render injects two
keys into the view context:

- `old` - `dict<string, any>` of the previously submitted form
  fields, keyed by field name.
- `errors` - `dict<string, list<string>>` of per-field error
  messages.

Idiomatic template:

```html
<form method="post" action="/signup">
    <label>Name
        <input name="name" value="{{ old.name|default('') }}">
    </label>
    {% if errors.name %}
        <p class="err">{{ errors.name|first }}</p>
    {% endif %}

    <label>Email
        <input name="email" type="email" value="{{ old.email|default('') }}">
    </label>
    {% if errors.email %}
        <p class="err">{{ errors.email|first }}</p>
    {% endif %}

    <input type="hidden" name="_csrf" value="{{ csrf }}">
    <button>Sign up</button>
</form>
```

Both `old` and `errors` are one-shot: after the next render
consumes them they vanish from the session.

## Combining with flash

A common pattern is to render an error banner alongside the
preserved form:

```html
{% if flashes.error %}
    <div class="banner banner-err">{{ flashes.error|first }}</div>
{% endif %}
```

Set the banner on the failing POST handler if you want a top-level
message in addition to the per-field errors:

```gb
@Post("/signup")
func submit(SignupDto body): dict<string, any> {
    /* validation already passed because @Assert raised earlier;
       business-rule failures happen here. */
    if (userExists(body.email)) {
        let resp = gebweb.redirect("/signup");
        return gebweb.flash(app, request, resp, "error", "Email taken.");
    }
    /* ... persist + redirect on success ... */
}
```

## How content negotiation works

The framework inspects the `Accept` header. HTML is preferred when
`text/html` appears in the header AND comes before
`application/json` (or `application/json` is absent). The default
when no `Accept` header is set is JSON. A handler can force the
JSON path even from a browser by setting
`Accept: application/json` on the fetch request.

## Manual API (escape hatch)

For handlers that want explicit control instead of automatic
rehydration:

```gb
import gebweb.forms as forms;

@Post("/signup")
func submit(dict<string, any> request): dict<string, any> {
    let store = gebweb.session(app);
    /* ... custom validation, build error map ... */
    let response = gebweb.redirect("/signup");
    let response = forms.preserveInput(store, response, request);
    return forms.withErrors(store, response, request, {"email": ["Already taken."]});
}
```

## Out of scope

- **Multi-step / wizard forms** that persist input across many
  requests use the session directly. The one-shot rehydration is
  specifically for the post-redirect-get error path; a wizard wants
  the opposite ("keep this around until the user finishes").
- **File-upload rehydration.** Re-presenting an `<input type="file">`
  with the previously selected file isn't possible from the server
  side (browsers don't accept programmatic file values for security
  reasons). Only text fields survive the round trip.
