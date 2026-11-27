# Fixture chapter

<!-- doctest:strict -->

<!-- doctest:run -->
```gb
import io;
import gebweb;

@Controller("/greetings")
class GreetingController extends gebweb.Controller {
    @Get("/{name}")
    func greet(string name): dict<string, any> {
        return {"hello": name};
    }
}

let client = gebweb.TestClient(gebweb.app());
io.println(client.get("/greetings/ada").text());
```

Output:

```text
{"hello":"ada"}
```

<!-- doctest:run -->
```gb
import gebweb;

@Controller("/health")
class HealthController {
    @Get("")
    func check(): dict<string, any> {
        return {"status": "ok"};
    }
}

export func main(list<string> args): int {
    gebweb.cli(gebweb.app(), {"name": "fixture"});
    return 0;
}
```

```text
METHOD  PATH                          HANDLER
GET     /health                       HealthController.check
```

<!-- doctest:fragment context=controller -->
```gb
@Get("/ping")
func ping(): dict<string, any> {
    return {"pong": true};
}
```

<!-- doctest:excerpt src=source.gb#build -->
```gb
export func build(): gebweb.GebwebApp {
    return gebweb.app();
}
```

<!-- doctest:skip shows a shell transcript inside gb fences on purpose -->
```gb
this is not geblang
```

<!-- doctest:file greeting.gb -->
```gb
module greeting;

export class Greeting {
    string text;

    func Greeting(string text) {
        this.text = text;
    }
}
```

<!-- doctest:run -->
```gb
import greeting;
import io;

io.println(greeting.Greeting("hi").text);
```

```text
hi
```

<!-- doctest:run file=main.gb -->
```gb
module main;
import greeting;
import io;

export func main(list<string> args): int {
    io.println(greeting.Greeting("from main").text);
    return 0;
}
```

```text
from main
```

<!-- doctest:file templates/hello.html -->
```html
<p>Hello, {{ name }}!</p>
```

<!-- doctest:run -->
```gb
import gebweb;
import io;

let app = gebweb.app([]);
gebweb.useViews(app, "templates");
io.println(gebweb.view(app, "hello.html", {"name": "Ada"}));
```

```text
<p>Hello, Ada!</p>
```
