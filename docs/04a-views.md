# Views and templates

<!-- doctest:strict -->

Gebweb renders HTML from Twig-style templates: `{{ }}` prints a value,
`{% %}` runs a tag such as `if`, `for`, `extends` or `include`, and `{# #}` is
a comment. Values are HTML-escaped unless you say otherwise. A controller
renders a page with `this.view(request, name, context)`, which also adds the
per-request values every page needs, such as the CSRF token and flash
messages.

## A page with a layout

A typical app has one layout and a template per page. These two templates sit
in the `templates/` directory of the project:

<!-- doctest:file templates/layout.html -->
```html
<!doctype html>
<html>
<head><title>{% block title %}Library{% endblock %}</title></head>
<body>
{% block content %}{% endblock %}
</body>
</html>
```

<!-- doctest:file templates/books/index.html -->
```html
{% extends "layout.html" %}
{% block title %}Books ({{ books | length }}){% endblock %}
{% block content %}
<h1>Books</h1>
{% if books is empty %}
<p>No books yet.</p>
{% endif %}
{% for book in books %}
<p>{{ loop.index }}. {{ book.title }}{% if book.year < 1900 %} (classic){% endif %}</p>
{% endfor %}
{% endblock %}
```

The controller passes the data, and the request so the framework can add
its per-request values:

<!-- doctest:run -->
```gb
import gebweb;
import io;

class Book {
    string title;
    int year;

    func Book(string title, int year) {
        this.title = title;
        this.year = year;
    }
}

@Controller("/books")
class BookPages extends gebweb.Controller {
    @Get("")
    func index(gebweb.Request request): gebweb.Response {
        let books = [Book("Emma", 1815), Book("Dune", 1965)];
        return this.view(request, "books/index.html", {"books": books});
    }
}

let app = gebweb.app();
gebweb.useViews(app, "templates");
let page = gebweb.TestClient(app).get("/books");
io.println(page.headers["Content-Type"]);
io.println(page.text().trim());
```

Output:

```text
text/html; charset=utf-8
<!doctype html>
<html>
<head><title>Books (2)</title></head>
<body>

<h1>Books</h1>


<p>1. Emma (classic)</p>

<p>2. Dune</p>


</body>
</html>
```

`gebweb.useViews(app, "templates")` registers the template engine; the
directory is relative to the working directory, so start the app from the
project root. A child template's blocks replace the layout's blocks of the
same name, and blocks it leaves out keep the layout's content.

## Template syntax

| Construct | Form |
|-----------|------|
| Print a value | `{{ user.name }}` (dots read dict keys and object fields) |
| Apply filters | `{{ user.bio \| default("") \| trim }}` |
| Comment | `{# not rendered #}` |
| Condition | `{% if x %} ... {% elseif y %} ... {% else %} ... {% endif %}` |
| Loop | `{% for item in items %} ... {% endfor %}`; test `items is empty` for the empty case |
| Loop state | `{{ loop.index }}` (from 1), `{{ loop.first }}`, `{{ loop.last }}` |
| Variable | `{% set total = items \| length %}` |
| Include | `{% include "partials/nav.html" %}` |
| Inheritance | `{% extends "layout.html" %}` with `{% block name %} ... {% endblock %}` |
| Inline condition | `{{ verbose ? long : short }}` |
| List literal | `{% for size in ['s', 'm', 'l'] %}` |
| Tests | `{% if name is defined %}`, `{% if x is not null %}`, `{% if items is empty %}` |
| Literal text | `{% raw %} {{ not interpreted }} {% endraw %}` |

A name missing from the context prints as an empty string. The context you
pass is never changed. Loop variables and a `set` inside a loop last for one
iteration; a `set` outside a loop is visible to everything after it.
Includes and blocks see the variables in scope where they appear.

## Escaping

Every printed value is HTML-escaped, so stored text can never become markup.
When a value is HTML you produced and trust, print it with the `raw` (or
`safe`) filter. The `json` filter writes a value as JSON for a script, with
`<`, `>`, `&` and `'` escaped so the data cannot end the `<script>` element:

<!-- doctest:file templates/comment.html -->
```html
<p>{{ comment.text }}</p>
<div>{{ comment.html | raw }}</div>
<script>const comment = {{ comment | json }};</script>
```

<!-- doctest:run -->
```gb
import gebweb;
import io;

let app = gebweb.app([]);
gebweb.useViews(app, "templates");
let comment = {"text": "<b>hi</b>", "html": "<em>trusted</em>"};
io.println(gebweb.view(app, "comment.html", {"comment": comment}).trim());
```

Output:

```text
<p>&lt;b&gt;hi&lt;/b&gt;</p>
<div><em>trusted</em></div>
<script>const comment = {"html":"\u003cem\u003etrusted\u003c/em\u003e","text":"\u003cb\u003ehi\u003c/b\u003e"};</script>
```

Use `raw` only for HTML your application generated, such as rendered
Markdown that was sanitised on the way in, never for text a user typed.

## Filters

| Filter | Arguments | Result |
|--------|-----------|--------|
| `escape`, `e` | | HTML-escaped text (the default for printed values) |
| `raw`, `safe` | | the value, marked as trusted HTML |
| `upper`, `lower` | | upper- or lower-case text |
| `capitalize` | | first letter upper case, the rest lower |
| `title` | | each word capitalised |
| `trim` | | text without surrounding whitespace |
| `length` | | length of a string, list or dict; 0 for null |
| `default` | `fallback` | `fallback` when the value is null or an empty string |
| `json` | | the value as script-safe JSON, marked as trusted |
| `date` | `format` | a Unix time or RFC 3339 string formatted with `datetime.format` |
| `replace` | `from`, `to` | text with every `from` replaced |
| `join` | `separator` | list items joined into text |
| `split` | `separator` | text split into a list |
| `first`, `last` | | first or last element or character; null when empty |
| `abs` | | absolute value |
| `round` | | number rounded half away from zero |

Register your own with `gebweb.viewsFilter(app, name, fn)`. The function
receives the value and then up to three arguments from the template. Its
result is escaped like any other value; when it returns HTML you trust, chain
`| raw` after it in the template.

<!-- doctest:file templates/price.html -->
```html
{{ price | money("EUR") }} {{ price | money("GBP", 0) }}
```

<!-- doctest:run -->
```gb
import gebweb;
import io;

let app = gebweb.app([]);
gebweb.useViews(app, "templates");
gebweb.viewsFilter(app, "money", func(any value, any currency, any places = 2): any {
    return "${currency} ${(value as decimal).format(places as int)}";
});
io.println(gebweb.view(app, "price.html", {"price": 1234.5}).trim());
```

Output:

```text
EUR 1234.50 GBP 1235
```

Registering a filter under a built-in name replaces the built-in filter.

## Values for every template

Some values belong in every page: the signed-in user, the site name, the
current locale. Register a context injector once and every render that has the
request gets the value. The injector receives the request:

<!-- doctest:file templates/greeting.html -->
```html
{{ siteName }}: {% if visitor %}signed in as {{ visitor }}{% else %}guest{% endif %}
```

<!-- doctest:run -->
```gb
import gebweb;
import io;

@Controller("/")
class HomePage extends gebweb.Controller {
    @Get("")
    func home(gebweb.Request request): gebweb.Response {
        return this.view(request, "greeting.html", {});
    }
}

let app = gebweb.app();
gebweb.useViews(app, "templates");

func siteName(gebweb.Request request): any {
    return "Library";
}

func visitor(gebweb.Request request): any {
    return request.header("X-Visitor");
}

gebweb.registerViewContext(app, "siteName", siteName);
gebweb.registerViewContext(app, "visitor", visitor);
let client = gebweb.TestClient(app);
io.println(client.get("/").text().trim());
io.println(client.request("GET", "/", null, {"X-Visitor": "ada"}).text().trim());
```

Output:

```text
Library: guest
Library: signed in as ada
```

A key passed in the context wins over an injector of the same name. The
framework registers injectors itself when you turn features on: `csrf` with
`useCsrf`, `flashes` with sessions, `errors` and `oldInput` for
[form rehydration](25-forms.md), `cspNonce` with
[security headers](27-security.md), and `asset` with the
[asset pipeline](26-assets.md). Injectors run only when the render has the
request, which is why handlers pass it to `this.view`.

## Context helpers

A named callable in view context can be invoked as `helper(value)`. Calls
resolve the helper from the current render's context, including through loops,
includes and layouts.
This supports the request-bound [`t(key, args)` translation helper](30-i18n.md).
Returned strings use normal HTML escaping unless the helper explicitly returns
a safe value.

`safe(...)` remains reserved. For other function-call names, a context callable
takes precedence over filter shorthand; a non-callable context value leaves
filter calls unchanged. Pipe filters such as `value | upper` still use the
registered filter.

## Rendering functions

| Function | Returns | Use |
|----------|---------|-----|
| `this.view(request, name, ctx = {}, status = 200)` | response | A page from a controller. |
| `this.partial(request, name, ctx = {})` | response | A fragment for HTMX or Turbo, with `Vary: HX-Request`. |
| `gebweb.htmlView(app, request, name, ctx = {}, status = 200, headers = null)` | response dict | A page outside a controller. |
| `gebweb.view(app, name, ctx = {}, request = null)` | `string` | Rendered text, such as an email body; pass the request to run injectors. |

## Compilation, caching and reloading

The engine compiles each template on first use and keeps the compiled form.
When running from source, it checks the file's modification time on each
render and recompiles a changed template, so edits show up without a restart.
A built binary keeps compiled templates for the life of the process. Pass
`{"reload": false}` or `{"reload": true}` as the third argument of `useViews`
to choose explicitly; any other value is a `ValueError`. This cache is separate
from HTTP response caching: every request still renders with its own context.

To find a broken template at startup instead of on its first request, preload
the templates production uses. List layouts and includes too, because preload
compiles only the names given:

<!-- doctest:fragment context=app -->
```gb
gebweb.useViews(app, "templates", {"reload": false});
gebweb.preloadViews(app, ["layout.html", "books/index.html", "partials/nav.html"]);
```

Preloading compiles templates; it does not render them or run injectors. With
reloading on, preload is skipped and templates still compile on first use.
Inheritance and include chains are limited to 128 templates and may not form
a cycle; a template that breaks a rule fails its own render without affecting
others.

## Reference

| API | Purpose |
|-----|---------|
| `gebweb.useViews(app, dir = "templates", options = {})` | Register the template engine; `options.reload` is a bool. |
| `gebweb.preloadViews(app, names)` | Compile the named templates now. |
| `gebweb.viewsFilter(app, name, fn)` | Register a filter: `fn(value, args...)`, up to three arguments. |
| `gebweb.registerViewContext(app, name, fn)` | Register a value for every render with a request: `fn(gebweb.Request)`. |
| `gebweb.view(app, name, ctx = {}, request = null)` | Render to a string. |
| `gebweb.htmlView(app, request, name, ctx = {}, status = 200, headers = null)` | Render to an HTML response dict. |
| `this.view` / `this.partial` | Render from a controller. |
