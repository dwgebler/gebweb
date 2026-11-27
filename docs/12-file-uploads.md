# File uploads

<!-- doctest:strict -->

A browser uploads files with a form whose `enctype` is `multipart/form-data`.
A handler receives the files by declaring a `dict<string, gebweb.UploadedFile>`
parameter, keyed by the form field name, and the form's text fields by
declaring a DTO:

<!-- doctest:run -->
```gb
import bytes;
import crypt;
import gebweb;
import io;

class DocumentForm {
    @Assert.notBlank
    string title;
    bool shared = false;
}

@Controller("/documents")
class DocumentController extends gebweb.Controller {
    gebweb.GebwebApp app;

    func DocumentController(gebweb.GebwebApp app) {
        parent();
        this.app = app;
    }

    @Post("")
    @MaxBody(5242880)
    func upload(DocumentForm form, dict<string, gebweb.UploadedFile> files): gebweb.Response {
        if (!files.hasKey("file")) {
            this.badRequest("choose a file to upload");
        }
        let file = files["file"];
        let extensions = {"application/pdf": "pdf", "text/plain": "txt"};
        if (!extensions.hasKey(file.contentType)) {
            this.badRequest("only PDF and text files are accepted");
        }
        string id = crypt.sha256(file.bytes()).substring(0, 12);
        string name = "documents/${id}.${extensions[file.contentType]}";
        file.saveToStorage(this.app, name);
        return this.created(
            {"stored": name, "bytes": file.size(), "shared": form.shared},
            "/${name}"
        );
    }
}

let app = gebweb.app();
gebweb.useStorage(app, gebweb.memoryStorage());
let client = gebweb.TestClient(app);
let note = {"filename": "notes.txt", "contentType": "text/plain", "content": "Agenda: budget"};

let r = client.multipart("/documents", {"title": "Board notes", "shared": "on", "file": note}, {});
io.println("${r.status} ${r.text()}");
let stored = r.json()["stored"] as string;
io.println(bytes.toString(gebweb.get(app, stored) ?? bytes.fromString("")));

let image = {"filename": "cat.png", "contentType": "image/png", "content": "png"};
io.println(client.multipart("/documents", {"title": "Cat", "file": image}, {}).json()["detail"]);
io.println(client.multipart("/documents", {"title": " ", "file": note}, {}).status);
io.println(client.multipart("/documents", {"title": "Empty"}, {}).json()["detail"]);
```

Output:

```text
201 {"bytes":14,"shared":true,"stored":"documents/9d110ff73528.txt"}
Agenda: budget
only PDF and text files are accepted
422
choose a file to upload
```

The text fields bind exactly like an urlencoded form: names match fields,
values are converted to the declared types, and `@Assert` rules are checked,
so a blank title answers 422. A ticked checkbox sends `on`, which binds as
`true`; an unticked one sends nothing, so give the field a default. The
matching HTML form:

<!-- doctest:file templates/documents/new.html -->
```html
<form method="post" action="/documents" enctype="multipart/form-data">
  <input type="hidden" name="_csrf" value="{{ csrf }}">
  <input name="title" required>
  <label><input type="checkbox" name="shared"> Share with the team</label>
  <input type="file" name="file" accept=".pdf,.txt">
  <button>Upload</button>
</form>
```

With CSRF protection on (see [CSRF](23-csrf.md)), the hidden `_csrf` field
is checked for upload forms as it is for any other form.

## Handling files safely

- **Never build a path from client input.** `filename` and the form fields
  are whatever the client sent, and can contain `../`. Name stored files
  yourself: above, from a hash of the content, which also stores a file
  uploaded twice only once.
- **Check the type you accept.** `contentType` is also client-supplied, so
  treat it as a hint; when it matters, check the content itself (for example
  the first bytes of `file.bytes()`).
- **Cap the size.** Request bodies are limited to 10 MB by default.
  `@MaxBody(bytes)` lowers the limit for one route and
  `gebweb.useMaxBodyBytes(app, bytes)` changes it for the app; a larger
  request answers 413 before the handler runs.
- **Store files outside the web root**, through a storage backend or a
  directory that static file serving does not expose.

## `UploadedFile`

| Member | Description |
|--------|-------------|
| `filename` | The name the client sent. |
| `contentType` | The type the client sent; `application/octet-stream` if none. |
| `bytes()` / `data` | The content. |
| `size()` | The content length in bytes. |
| `saveToStorage(app, name)` | Write to the app's storage backend; see [Storage](21-storage.md). |
| `saveTo(path)` | Write to a local file. |

A form can send several files under different field names; each is an entry
in the dict. If a field name is sent twice, the last file wins.

The whole request body is read into memory before the handler runs, so keep
the size limit in line with the memory you can spare per request.

## Testing uploads

`client.multipart(path, parts, headers)` sends a `multipart/form-data`
request. Each part is a string for a text field or a dict with `filename`,
`contentType` and `content` for a file; `content` may be a string or `bytes`,
so binary files such as images can be tested:

<!-- doctest:fragment context=script -->
```gb
import bytes;

let client = gebweb.TestClient(gebweb.app());
let png = bytes.fromHex("89504e470d0a1a0a");
let r = client.multipart(
    "/avatars",
    {"avatar": {"filename": "me.png", "contentType": "image/png", "content": png}},
    {"X-API-Key": "test-key"}
);
```

## Reference

- Files: a `dict<string, gebweb.UploadedFile>` handler parameter.
- Text fields: a DTO parameter, bound and validated like an urlencoded form.
- `@MaxBody(bytes)` / `gebweb.useMaxBodyBytes(app, bytes)`: size limits.
- `TestClient.multipart(path, parts, headers)`: send an upload in tests.
- `web.parseMultipart(request)`: the low-level parser, returning
  `{"fields": ..., "files": ...}`.
