# Views and templates

Gebweb ships a Twig-style template engine compiled from
`stdlib/web/views`. Templates render through `gebweb.view(app,
request, name, ctx)`, `gebweb.html(body)`, or `this.view(request,
name, ctx)` inside a controller. The engine supports inheritance
via `extends` / `block`, file inclusion, control flow,
expressions, filters, and a registerable filter / context-injector
surface.

## Wiring

```gb
let app = gebweb.app([HelloController]);
gebweb.useViews(app, "templates");
```

The directory argument defaults to `"templates"`, resolved
relative to the working directory.

## Compilation and caching

`ViewEngine` compiles each template into compact node and expression opcodes
on first load. Compilation merges adjacent static text, folds safe
literal-only expressions, and resolves built-in filter slots. Custom filters
remain dynamic, and replacing a built-in filter with `addFilter` or
`gebweb.viewsFilter` affects templates that were already compiled.
Tokenization scans template characters once, so first-load work scales with
template size rather than with the square of its size. A template containing
only static text takes a direct path without expression tokenization.

Compiled views are cached. Development mode checks the source modification
time and recompiles a changed template; bundled production applications keep
the compiled view for the process lifetime. This cache is separate from HTTP
response caching: an uncached response still renders its compiled template
against the current request context.

### Production preloading

Compile known production templates before accepting traffic:

```gb
gebweb.useViews(app, "templates");
gebweb.preloadViews(app, [
    "home.html",
    "account/profile.html",
    "layout.html",
    "partials/nav.html",
]);
```

`preloadViews` loads only the names supplied, so list shared layouts and
includes explicitly. A missing or malformed production template fails during
startup instead of on its first request. Development mode remains lazy so
source edits continue to use modification-time reloads. Preloading compiles
templates; it does not render pages, populate response caches, or run context
injectors.

## Syntax

| Construct | Form |
|-----------|------|
| Variable | `{{ user.name }}` |
| Variable with filter | `{{ user.name | upper }}` |
| Variable with chained filters | `{{ user.bio | default("") | escape }}` |
| Comment | `{# ... #}` |
| If / elif / else | `{% if x %} ... {% elif y %} ... {% else %} ... {% endif %}` |
| For loop | `{% for item in items %} ... {% endfor %}` |
| Loop helpers | `{{ loop.index }}`, `{{ loop.first }}`, `{{ loop.last }}` |
| Set | `{% set total = items | length %}` |
| Include | `{% include "partials/menu.html" %}` |
| Extends + block | `{% extends "layout.html" %}` + `{% block main %}...{% endblock %}` |
| Inline ternary | `{{ verbose ? long : short }}` |
| `is defined / null / empty` test | `{% if name is defined %}`, `{% if x is not null %}`, `{% if list is empty %}` |
| `{% raw %} ... {% endraw %}` | Disable interpolation in a span (e.g. for embedded `{{ }}` literal output). |

Identifiers resolve from the context dict; dotted paths walk
nested dicts / instance fields. Missing names render as empty
strings without raising.

### Scope behavior

The context passed to `render` is never mutated. A loop creates a local scope
for its item and `loop` metadata; those names shadow outer values only for that
iteration. A `set` inside the loop is local to the iteration, while a `set`
outside a loop remains visible to following nodes. Includes and blocks share
the scope visible at their call site.

## Inheritance

```html
{# templates/layout.html #}
<!doctype html>
<html>
<head><title>{% block title %}Default{% endblock %}</title></head>
<body>
  {% block content %}{% endblock %}
</body>
</html>
```

```html
{# templates/show.html #}
{% extends "layout.html" %}
{% block title %}{{ user.name }}{% endblock %}
{% block content %}
  <h1>{{ user.name }}</h1>
  <p>{{ user.bio | default("(no bio)") }}</p>
{% endblock %}
```

The deepest child wins on each named block; named blocks left
unoverridden retain their parent body.

## Auto-escape and `raw`

Variable interpolation passes through the `escape` filter by
default (HTML entities). Mark a value safe with the `raw` or
`safe` filter when the source is trusted HTML:

```html
{{ markdown_body | raw }}
{{ snippet | safe }}
```

Filters that return HTML (`json`, `raw`, `safe`) wrap the result
in `SafeString` so subsequent interpolation skips re-escaping.

## Built-in filters

| Filter | Signature | Purpose |
|--------|-----------|---------|
| `escape` / `e` | `(any)` | HTML-escape (default for variable output). |
| `raw` / `safe` | `(any)` | Mark value as already-safe HTML; skips escaping. |
| `upper` | `(any)` | Uppercase. |
| `lower` | `(any)` | Lowercase. |
| `capitalize` | `(any)` | First letter upper, rest lower. |
| `length` | `(any)` | Length of string / list / dict; 0 for null. |
| `default` | `(any, fallback)` | `fallback` when value is null or empty string. |
| `json` | `(any)` | JSON-encode (returns safe HTML). |
| `date` | `(any, format)` | Format a unix int or RFC3339 string per `datetime.format`. |
| `replace` | `(any, from, to)` | Substring replace. |
| `trim` | `(any)` | Strip leading / trailing whitespace. |
| `join` | `(list, sep)` | Concatenate list items. |
| `split` | `(string, sep)` | Split into a list. |
| `first` | `(list \| string)` | First element / character; null for empty. |
| `last` | `(list \| string)` | Last element / character; null for empty. |
| `abs` | `(int \| decimal)` | Absolute value. |
| `round` | `(any)` | Round half-away-from-zero. |

## Custom filters

```gb
gebweb.viewsFilter(app, "currency", func(any v): any {
    return "$" + (v as string);
});
```

```html
{{ price | currency }}
```

Filters are values keyed by name in `ViewEngine.filters`; later
registrations replace earlier ones.

## Context injectors

Some values are needed in every template (csrf token, current
user, flash messages, asset map). Register them once with
`registerViewContext(app, name, fn)`; the value gets merged into
the context dict whenever `gebweb.view(app, request, name, ctx)`
runs. The injector receives a rich `Request` (1.7.1+):

```gb
gebweb.registerViewContext(app, "currentUser", func(gebweb.Request request): any {
    return request.contains("user") ? request["user"] : null;
});
```

```html
{% if currentUser %}
  <p>Signed in as {{ currentUser.name }}</p>
{% endif %}
```

The framework itself wires injectors for `csrf`, `flashes`,
`errors`, `oldInput`, `cspNonce`, and `asset` when the matching
`use*` helpers are called. User keys passed in `ctx` win over
injectors on name collision.

## Rendering paths

| API | Returns | When to use |
|-----|---------|-------------|
| `gebweb.view(app, request, name, ctx)` | `string` | Render to a string (e.g. for an email body) and pass `request` so injectors fire. |
| `gebweb.htmlView(app, request, name, ctx, status?, headers?)` | response dict | Full HTML response, ready to return from a handler. |
| `this.view(request, name, ctx, status = 200)` | response dict | Same as `htmlView`, accessible from a controller. |
| `this.partial(request, name, ctx)` | response dict | Fragment with `Vary: HX-Request`; intended for HTMX / Turbo-Frame partials. |
| `ViewEngine.render(name, ctx)` | `string` | Engine-level render without going through the framework (no injectors). |

## Reference

| API | Purpose |
|-----|---------|
| `gebweb.useViews(app, dir = "templates")` | Register the view engine. |
| `gebweb.preloadViews(app, names)` | Compile named templates during production startup. |
| `gebweb.view(app, request, name, ctx)` | Render to a string with injectors threaded. |
| `gebweb.htmlView(app, request, name, ctx, status?, headers?)` | Render and wrap as an HTML response dict. |
| `gebweb.viewsFilter(app, name, fn)` | Register a custom filter on the active view engine. |
| `gebweb.registerViewContext(app, name, fn)` | Register a value to merge into every rendered context. |
| `ViewEngine.preload(names)` | Engine-level production preload; development is a no-op. |
| `gebweb.ViewEngine(dir)` / `gebweb.View(...)` | Class refs for typing parameters in user code. |
