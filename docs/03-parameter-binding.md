# Parameter binding

<!-- doctest:strict -->

A handler declares what it needs as typed parameters, and Gebweb fills them
from the request before calling it: path segments, query values, headers, the
body, the signed-in user, uploaded files, or the request itself. Values are
converted to the declared types first, so the handler never parses strings,
and a value that cannot be converted is answered with a 400 before the handler
runs.

## A complete example

<!-- doctest:run -->
```gb
import gebweb;
import io;

class NewComment {
    @Assert.notBlank
    string author;

    @Assert.maxLength(500)
    string text;
}

@Controller("/posts")
class CommentController extends gebweb.Controller {
    @Get("/{postId}/comments")
    func list(int postId, int page = 1, ?string author, list<string> tag, @Header("Accept-Language") ?string lang): dict<string, any> {
        return {"post": postId, "page": page, "author": author, "tag": tag, "lang": lang};
    }

    @Post("/{postId}/comments")
    func create(int postId, @Body NewComment input): gebweb.Response {
        return this.created({"post": postId, "author": input.author}, "/posts/${postId}/comments/1");
    }
}

let client = gebweb.TestClient(gebweb.app());
io.println(client.get("/posts/7/comments").text());
let query = "/posts/7/comments?page=2&author=ada&tag=go&tag=web";
io.println(client.request("GET", query, null, {"Accept-Language": "en"}).text());

io.println(client.post("/posts/7/comments", {"author": "ada", "text": "Nice"}).status);
let form = client.request("POST", "/posts/7/comments", "author=grace&text=Hi", {
    "Content-Type": "application/x-www-form-urlencoded",
});
io.println("${form.status} ${form.text()}");

let malformed = client.request("POST", "/posts/7/comments", "{not json", {
    "Content-Type": "application/json",
});
io.println("${malformed.status} ${malformed.json()["detail"]}");
let wrongType = client.post("/posts/7/comments", {"author": 5, "text": "x"});
io.println("${wrongType.status} ${wrongType.json()["detail"]}");
let invalid = client.post("/posts/7/comments", {"author": " ", "text": "x"});
io.println("${invalid.status} ${invalid.json()["errors"]}");
```

Output:

```text
{"author":null,"lang":null,"page":1,"post":7,"tag":[]}
{"author":"ada","lang":"en","page":2,"post":7,"tag":["go","web"]}
201
201 {"author":"grace","post":7}
400 invalid JSON body: expected string key in object
400 invalid JSON body: cannot assign int to string field NewComment.author
422 [{"code": "notBlank", "field": "author", "message": "must not be blank"}]
```

- `postId` matches the `{postId}` path segment and is converted to `int`.
- `page`, `author` and `tag` come from the query string. `page` has a
  default, `author` is nullable, and `tag` collects every `tag=` value, so all
  three may be left out.
- `lang` comes from the `Accept-Language` header.
- `input` is the body, bound to a `NewComment` from JSON or a form post and
  then validated. A body that is not valid JSON, or a field of the wrong type,
  is a 400; a body that breaks a validation rule is a 422 listing each
  failure.

## How a parameter's source is chosen

Gebweb looks at each parameter in turn:

1. A parameter decorated `@PathParam`, `@QueryParam`, `@Header` or `@Body`
   takes its value from that source.
2. A parameter whose name matches a `{name}` path segment takes that segment.
3. A parameter whose type is the authenticated user class receives the
   signed-in user; see [Authentication](08-auth.md).
4. A `dict<string, gebweb.UploadedFile>` parameter receives the uploaded
   files; see [File uploads](12-file-uploads.md).
5. A parameter typed as one of your classes, or a `list` of one, is the
   request body. A handler can have at most one body parameter.
6. A `gebweb.Request` parameter receives the request object, and a
   `dict<string, any>`, `dict` or `any` parameter receives the raw request
   dictionary. Either can appear alongside other parameters.
7. Every other parameter is read from the query string under its own name.

The rules are applied when the app starts, not on each request, so a handler
with two body parameters fails at startup.

## Path parameters

A `{name}` segment binds to the parameter of the same name, converted to its
type. Path parameters are always required: a route only matches when the
segment is present. See [Routing](02-routing.md#path-parameters).

## Query parameters

| Declaration | `?page=2` | `page` absent |
|-------------|-----------|---------------|
| `int page` | `2` | 400 `missing required parameter 'page'` |
| `?int page` | `2` | `null` |
| `int page = 1` | `2` | `1` |
| `list<int> page` | `[2]`; `?page=2&page=3` gives `[2, 3]` | `[]` |
| `?list<int> page` | `[2]` | `null` |

A `list` query parameter collects every occurrence of its key, converting each
value to the element type; one bad element is a 400 for the whole request.

## Headers

`@Header("Name")` reads a request header, matched case-insensitively and
converted like a query value. Make the parameter nullable or give it a default
when clients may leave the header out:

<!-- doctest:fragment context=controller -->
```gb
@Get("/widgets")
func list(@Header("X-Request-Budget") int budgetMs = 500, @Header("If-None-Match") ?string etag): dict<string, any> {
    return {"budget": budgetMs, "etag": etag};
}
```

## Request bodies

A parameter typed as one of your classes receives the body. Gebweb chooses the
decoder from the `Content-Type`:

- `application/json` (and a body sent without a content type) is parsed as
  JSON into an instance of the class, including nested class-typed fields.
- `application/x-www-form-urlencoded`, as an HTML form posts it, matches form
  fields to class fields by name and converts each to the field's type.

Fields the client sends that the class does not declare are ignored, so a
client cannot set a field you did not put on the input class. After binding,
the instance is validated against its `@Assert` rules; see
[Validation](05-validation.md). `@Groups` on the class can further limit which
fields a client may write; see
[Serialization groups](07-serialization-groups.md).

A `list<Class>` parameter binds a JSON array body, element by element:

<!-- doctest:fragment context=controller -->
```gb
@Post("/widgets/bulk")
func bulkCreate(list<WidgetInput> body): dict<string, any> {
    return {"received": body.length()};
}
```

The body class can come from another module, through an import alias
(`func create(dto.NewWidget body)`), and inherited fields of a subclass bind
and validate like its own.

Use separate input and output classes. An input class declares only what a
client may send, and its rules; the output class can carry server-assigned
fields such as `id` and timestamps.

## Explicit binding decorators

The name-and-type rules cover most handlers. A decorator states the source
outright, and can bind a public name that differs from the parameter's:

| Decorator | Source |
|-----------|--------|
| `@PathParam("name")` | the `{name}` path segment |
| `@QueryParam("name")` | the `name` query value (or values, for a list) |
| `@Header("Name")` | the `Name` request header |
| `@Body` | the request body, even when the type would not be chosen as one |

The argument defaults to the parameter's own name:

<!-- doctest:fragment context=controller -->
```gb
@Get("/widgets/{id}")
func show(@PathParam("id") int widgetId, @QueryParam("v") ?bool verbose): dict<string, any> {
    return {"id": widgetId, "verbose": verbose};
}
```

`@Description("...")` on a parameter becomes its description in the OpenAPI
document and has no effect at runtime.

## The request itself

Declare a `gebweb.Request` parameter to read anything the other rules do not
cover: the client address, cookies, raw headers, the raw body text. It can sit
beside other parameters:

<!-- doctest:fragment context=controller -->
```gb
@Get("/widgets/{id}/audit")
func audit(int id, gebweb.Request req): dict<string, any> {
    return {"id": id, "ip": req.clientIp(), "agent": req.header("User-Agent")};
}
```

`Request` offers `method()`, `path()`, `scheme()`, `isSecure()`, `host()`,
`clientIp()`, `header(name)`, `cookie(name)`, `query(name)`,
`queryInt(name)`, `queryBool(name)`, `queryAll(name)`, `isJson()`, `text()`
and `json()`. `queryInt` and `queryBool` follow the conversion rules below. A
`dict<string, any>` parameter instead receives the raw request dictionary
(`method`, `path`, `headers`, `body` and so on), which is mostly useful when
porting code written against it.

## Conversion rules

Path, query, header and form values arrive as text and are converted to the
declared type:

| Type | Accepted text | Example |
|------|---------------|---------|
| `string` | anything | `ada` |
| `int` | an optional `-` and digits | `-42` |
| `float` | a decimal or scientific number | `2.5e3` |
| `decimal` | a decimal number, kept exact | `19.99` |
| `bool` | `true`, `false`, `1`, `0`, `yes`, `no`, `on` or `off`, in any case | `Yes` |
| `?T` | the same as `T`; an empty value binds `null` | `?flag=` |
| `list<T>` (query) | each occurrence converted as `T` | `?id=1&id=2` |

A value that does not convert is a 400 Problem Details response whose detail
names the source and the parameter:

```json
{"type": "about:blank", "title": "Bad Request", "status": 400,
 "detail": "path parameter 'id' expects int, got 'abc'"}
```

The source is `path parameter`, `query parameter`, `header` or `form field`.

## OpenAPI

Every bound parameter appears in the generated OpenAPI document with its
location and type. Path parameters are always required; query and header
parameters are required unless they are nullable, have a default, or are
lists. The body appears as the operation's request body, with the input
class's schema. See [OpenAPI and SwaggerUI](13-openapi.md).

## Reference

- Path: `{name}` in the route with a parameter of the same name, or
  `@PathParam("name")`.
- Query: any parameter no other rule claims, or `@QueryParam("name")`;
  `list<T>` collects repeated keys.
- Header: `@Header("Name")`.
- Body: one parameter typed as a class or `list<Class>`, or marked `@Body`;
  JSON or form-encoded.
- User: a parameter typed as the authenticated user class.
- Files: a `dict<string, gebweb.UploadedFile>` parameter.
- Request: a `gebweb.Request` parameter, or `dict<string, any>` / `dict` /
  `any` for the raw request dictionary.
- Errors: 400 for a missing required value, a value that does not convert or
  a malformed body; 422 for a body that fails validation.
