# LLM integration

<!-- doctest:strict -->

`gebweb.llmClient(opts)` builds a stdlib `llm.Client` and `gebweb.useLlm`
registers it for constructor injection. Services depend on that interface,
so application tests can supply a deterministic implementation without API
keys or provider traffic. Provider selection does not make every operation or
model interchangeable; the supported combinations are listed below.

## Wiring

The factory selects an adapter and stores its configuration. Constructing it
does not contact the provider or verify credentials/model access. This
complete example makes no network calls:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import llm;

let app = gebweb.app([], {"docs": false});
io.println(gebweb.llm(app) == null);
let client = gebweb.llmClient({
    "provider": "anthropic",
    "apiKey": "example-only-key",
    "timeoutMs": 15000,
});
gebweb.useLlm(app, client);
io.println(gebweb.llm(app) instanceof llm.Client);
```

```text
true
true
```

Register the client before constructing consumers. `gebweb.app()` discovers
loaded controllers lazily, so registration before the first request works.
An explicit `gebweb.app([ControllerClass])` list or `addControllers` constructs
controllers immediately: configure the app first, then add them. Replacing a
client later does not update a client already held by a constructed service.

## Using the client in handlers

Put this module in `src/summary_app.gb`. The service uses constructor injection;
the controller reads the application-owned model parameter. Input validation
bounds text length before a provider call:

<!-- doctest:file summary_app.gb -->
```gb
import gebweb;
import gebweb.errors as errors;
import llm;

class SummaryInput {
    @Assert.notBlank
    @Assert.maxLength(4000)
    string text;
}

class Summariser {
    llm.Client client;

    func Summariser(llm.Client client) {
        this.client = client;
    }

    func run(string text, string model): string {
        try {
            let response = this.client.chat([{"role": "user", "content": text}], {
                "model": model,
                "system": "Summarise the supplied text in one sentence.",
                "maxTokens": 256,
            });
            return response["content"] as string;
        } catch (IOError error) {
            throw errors.HttpException(503, "Summary service unavailable", "Service Unavailable");
        } catch (RuntimeError error) {
            throw errors.HttpException(503, "Summary service unavailable", "Service Unavailable");
        }
    }
}

@Controller("/summary")
class SummaryController {
    Summariser summariser;
    gebweb.GebwebApp app;

    func SummaryController(Summariser summariser, gebweb.GebwebApp app) {
        this.summariser = summariser;
        this.app = app;
    }

    @Post("")
    func summarise(SummaryInput body): dict<string, any> {
        string model = gebweb.getParameter(this.app, "llm.model") as string;
        return {"summary": this.summariser.run(body.text, model)};
    }
}

export func build(llm.Client client, string model): gebweb.GebwebApp {
    let app = gebweb.app();
    gebweb.parameter(app, "llm.model", model);
    gebweb.useLlm(app, client);
    return app;
}
```

`useLlm` registers the `llm.Client` interface, including its canonical
`llm.contract.Client` alias target. `Summariser` needs no explicit factory.
See [Dependency injection](06a-dependency-injection.md).

### Test without credentials

This fixed-response stub implements the four required methods. It also has
failure modes for exercising the HTTP error boundary. Put it in
`src/summary_stub.gb`; it is test support, not a production provider:

<!-- doctest:file summary_stub.gb -->
```gb
import llm;

export class SummaryStub implements llm.Client {
    string failure;

    func SummaryStub(string failure = "") {
        this.failure = failure;
    }

    func chat(list<dict<string, any>> messages, dict<string, any> opts): dict<string, any> {
        if (this.failure == "transport") {
            throw TimeoutError("private transport details");
        }
        if (this.failure == "provider") {
            throw RuntimeError("private upstream details");
        }
        return {
            "content": "A fixed test summary.",
            "model": "test-model",
            "stopReason": "stop",
            "usage": {"inputTokens": 0, "outputTokens": 0, "totalTokens": 0},
        };
    }

    func embed(string text, dict<string, any> opts): dict<string, any> {
        throw RuntimeError("test stub does not embed");
    }

    func analyzeImage(bytes image, string prompt, dict<string, any> opts): dict<string, any> {
        throw RuntimeError("test stub does not analyze images");
    }

    func generateImage(string prompt, dict<string, any> opts): dict<string, any> {
        throw RuntimeError("test stub does not generate images");
    }
}
```

Drive the actual application route, including DI and DTO validation:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import summary_app;
import summary_stub;

let app = summary_app.build(summary_stub.SummaryStub(), "test-model");
let client = gebweb.TestClient(app);
let response = client.post("/summary", {"text": "A long article goes here."});
io.println(response.status);
io.println((response.json() as dict<string, any>)["summary"]);
io.println(client.post("/summary", {"text": ""}).status);
```

```text
200
A fixed test summary.
422
```

The stub verifies application behavior independently of provider availability.
It does not verify a provider's HTTP protocol or model output. Keep a separate,
opt-in integration check for the provider/model you deploy.

For code that already owns the app, the getter avoids container lookup:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import llm;
import summary_app;
import summary_stub;

let app = summary_app.build(summary_stub.SummaryStub(), "test-model");
let client = gebweb.llm(app) as llm.Client;
let response = client.chat([{"role": "user", "content": "Example text"}], {"model": "test-model"});
io.println(response["content"]);
```

```text
A fixed test summary.
```

## Picking a provider

`provider` is required and accepts exactly `openai`, `anthropic` or `bedrock`.
The facade forwards other options to the selected stdlib adapter:

| Option | Applies to | Default / requirement |
|--------|------------|-----------------------|
| `apiKey` | OpenAI, Anthropic | Required; load from application secrets. |
| `region` | Bedrock | Required. |
| `accessKey`, `secretKey` | Bedrock | Both required explicitly. |
| `timeoutMs` | All | `600000` (ten minutes) for non-streaming HTTP calls; choose a deadline suited to the application. `0` disables that deadline. |
| `endpoint` | OpenAI | `https://api.openai.com` |
| `endpoint` | Anthropic | `https://api.anthropic.com` |
| `endpoint` | Bedrock | `https://bedrock-runtime.<region>.amazonaws.com` |
| `controlEndpoint` | Bedrock | `https://bedrock.<region>.amazonaws.com`, for model listing. |
| `organization` | OpenAI | Omitted. |
| `apiVersion` | Anthropic | `2023-06-01` |

Endpoints are base URLs; adapters append their API paths. An arbitrary
compatible-looking service may use different paths, authentication or request
fields. The Bedrock adapter takes explicit signing credentials; it does not
load an AWS credential chain or accept a session-token option.

For production, load credentials and an account-enabled model during setup.
This module can supply the `build(conn)` function used by the
[standard entry point](01-getting-started.md); it is type-checked without
reading environment variables or contacting a provider during these examples:

<!-- doctest:file live_summary.gb -->
```gb
import db;
import gebweb;
import secrets;
import summary_app;

export func build(db.Connection conn): gebweb.GebwebApp {
    let client = gebweb.llmClient({
        "provider": "anthropic",
        "apiKey": secrets.requireEnv("ANTHROPIC_API_KEY"),
        "timeoutMs": 15000,
    });
    return summary_app.build(client, secrets.requireEnv("LLM_MODEL"));
}
```

`LLM_MODEL` is application configuration, not a built-in variable read by the
client. Keep the provider, endpoint and model under application control rather
than accepting them from HTTP input. Factory configuration is not a model
access check; verify access in a controlled integration/deployment check.

## Client operations

These are capabilities of the adapters shipped with this implementation;
individual operations still require a model that supports them:

| Operation | OpenAI adapter | Anthropic adapter | Bedrock adapter |
|-----------|----------------|-------------------|-----------------|
| `chat` | Supported | Supported | Anthropic Messages schema |
| `chatStream` | SSE | SSE | Throws `RuntimeError` |
| `embed` | Supported | Throws `RuntimeError` | Titan/Cohere embedding families |
| `embedBatch` | One input array | Throws `RuntimeError` | Calls `embed` for each text |
| `analyzeImage` | Supported | Supported | Anthropic Messages schema |
| `generateImage` | Supported | Throws `RuntimeError` | Titan/Stability image families |
| `models` | Models endpoint | Models endpoint | ListFoundationModels |

`model` is supplied in each operation's options, not in `useLlm`. It is a
free-form identifier passed to the adapter/provider; the factory does not
choose a default model. A `models()` entry does not prove that every operation
supports that model.

| Call / option | Shape or behavior |
|---------------|-------------------|
| `chat(messages, opts)` | Messages are dictionaries with `role` and `content`; returns `content`, `model`, `stopReason`, `usage`, and optional `toolCalls`. |
| Chat `system` | Use this option for a portable system instruction. |
| Chat `maxTokens` | OpenAI forwards it when supplied; Anthropic/Bedrock default to `1024`. |
| Chat `temperature`, `topP` | Forwarded when supplied; valid ranges depend on the model. |
| `embed(text, opts)` | Returns `vector`, `model`, `usage`. |
| `embedBatch(texts, opts)` | Returns `vectors`, `model`, `usage`. |
| `analyzeImage(image, prompt, opts)` | Takes image bytes; `mimeType` defaults to `image/png`. Returns a chat-shaped result. |
| `generateImage(prompt, opts)` | Returns `images` (dictionaries with `bytes` and `format`) and `model`; provider-specific options differ. |
| `chatStream(messages, opts, callback)` | Calls the callback with incremental text and returns the assembled chat-shaped result. |

The streaming callback is not automatically an HTTP stream to your browser.
Adapt it deliberately to your response protocol. Streaming is not covered by
the client's non-streaming `timeoutMs` deadline; do not assume a total stream
deadline or request-disconnect cancellation. Tool calls are returned as data:
the framework does not execute them. Validate names/arguments and apply the
caller's authorization before invoking any application capability.

## Errors

Provider HTTP failures and unsupported operations can raise `RuntimeError`.
Transport failures, including `TimeoutError`, belong to the `IOError` family.
The example service maps both to a generic 503; it does not expose upstream
bodies, credentials or private request information in Problem Details:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import summary_app;
import summary_stub;

for (failure in ["provider", "transport"]) {
    let app = summary_app.build(summary_stub.SummaryStub(failure), "test-model");
    let response = gebweb.TestClient(app).post("/summary", {"text": "Example text"});
    io.println(response.status);
    io.println((response.json() as dict<string, any>)["detail"]);
    io.println(response.text().contains("private"));
}
```

```text
503
Summary service unavailable
false
503
Summary service unavailable
false
```

Report safe diagnostic context through your application logger. The adapters
make synchronous calls and do not add automatic retry/backoff or a framework
queue. Bound input/output, use measured rate/admission limits, and move long
work to [background jobs](18-background-jobs.md) when a synchronous HTTP request
is unsuitable. A provider timeout is not a complete application request budget.
Treat model output as untrusted data; validate it before storage, rendering or
use as executable instructions.

## Reference

| Framework API | Behavior |
|---------------|----------|
| `gebweb.llmClient(opts)` | Thin wrapper around `llm.client(opts)`; returns `llm.Client`. |
| `gebweb.useLlm(app, client)` | Registers an implementation for DI and stores it on the app; returns the app. |
| `gebweb.llm(app)` | Returns that client or null before registration. |

The underlying interface is `llm.Client`, an alias of `llm.contract.Client`.
See [Dependency injection](06a-dependency-injection.md),
[Validation](05-validation.md), [Responses](04-responses.md),
[Testing](14-testing.md) and [Security](27-security.md) for application boundaries.
