# LLM integration

Gebweb wraps the stdlib `llm` module so a single `import gebweb;`
gives applications a provider-agnostic LLM client plus DI
integration, without a manual factory hop through the stdlib.

For the underlying interface (chat / embed / image analysis /
image generation) see the language docs for the
[`llm` module](https://github.com/dwgebler/geblang). The Gebweb
facade adds the wiring patterns below.

## Wiring

Build a client with `gebweb.llmClient(opts)` and register it on
the app with `gebweb.useLlm(app, client)`:

```gb
import gebweb;
import sys;

let app = gebweb.app([SummaryController]);
gebweb.useLlm(app, gebweb.llmClient({
    "provider": "anthropic",
    "apiKey":   sys.getenv("ANTHROPIC_API_KEY") as string
}));
gebweb.serve(app, ":8080");
```

`gebweb.llmClient(opts)` is a thin re-export of `llm.client(opts)`;
either form works. `gebweb.useLlm` stores the client on the app
and also registers it in the DI container under the `llm.Client`
key.

## Using the client in handlers

Two paths. For services autowired through the DI container, type
the constructor parameter as `llm.Client` and the configured
client is injected automatically:

```gb
import llm;

class Summariser {
    llm.Client client;
    func Summariser(llm.Client client) { this.client = client; }
    func run(string text): string {
        let resp = this.client.chat([
            {"role": "user", "content": "Summarise: " + text}
        ], {"model": "claude-opus-4-8", "maxTokens": 256});
        return resp["content"] as string;
    }
}

class SummaryController {
    Summariser summariser;
    func SummaryController(Summariser summariser) {
        this.summariser = summariser;
    }

    @Post("/summary")
    func summarise(SummaryRequest body): dict<string, any> {
        return {"summary": this.summariser.run(body.text)};
    }
}
```

`useLlm` registers the client under the interface name
"llm.Client" in the DI container, so any constructor parameter
typed with that interface resolves to the configured instance
without an explicit factory. See
[Dependency injection](06a-dependency-injection.md) for the
container's autowiring rules and the registration surface.

For handler code that already has the app reference and just
wants the configured client, use the `gebweb.llm(app)` getter:

```gb
let client = gebweb.llm(app) as llm.Client;
let resp = client.chat(messages, opts);
```

`gebweb.llm(app)` returns the registered client (or null if
`useLlm` has not been called) without going through DI.

## Picking a provider

`provider` in the opts dict picks the backend:

```gb
gebweb.useLlm(app, gebweb.llmClient({"provider": "openai",    "apiKey": "..."}));
gebweb.useLlm(app, gebweb.llmClient({"provider": "anthropic", "apiKey": "..."}));
gebweb.useLlm(app, gebweb.llmClient({
    "provider":  "bedrock",
    "region":    "us-east-1",
    "accessKey": gebweb.parameter(app, "aws.accessKey") as string,
    "secretKey": gebweb.parameter(app, "aws.secretKey") as string
}));
```

Same surface across all three: `chat`, `embed`, `analyzeImage`,
`generateImage`. Operations that a provider does not support
throw a `RuntimeError` naming the missing method.

## Errors

Upstream API failures (rate limits, invalid keys, validation
errors) surface as `RuntimeError` from inside the handler. Route
them through Gebweb's exception handling like any other call:

```gb
import gebweb.errors as errors;

@Post("/summary")
func summarise(SummaryRequest body): dict<string, any> {
    let client = gebweb.resolve(this.app, llm.Client) as llm.Client;
    try {
        let resp = client.chat(buildMessages(body), {"model": "..."});
        return {"summary": resp["content"]};
    } catch (RuntimeError e) {
        throw errors.HttpException(503, "LLM provider error: " + e.message,
            "Service Unavailable");
    }
}
```

The framework's Problem Details renderer formats the thrown
exception at the matching status. See
[Responses](04-responses.md) for the full HttpException surface
and the controller-method short forms.
