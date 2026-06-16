# Validation

When a handler binds a JSON body to a user-class parameter, the
framework runs the registered `@Assert.*` validators against the
parsed instance before calling the handler. Failures collect into a
422 Unprocessable Entity response shaped per RFC 9457 Problem Details.

## Built-in validators

```gb
class UserCreateDTO {
    @Assert.notBlank
    string name;

    @Assert.email
    string email;

    @Assert.minLength(8)
    string password;

    @Assert.range(min: 13, max: 120)
    int age;
}
```

The full built-in list:

| Decorator                            | Checks                                |
|--------------------------------------|---------------------------------------|
| `@Assert.notBlank`                   | non-null and non-empty string         |
| `@Assert.notNull`                    | non-null                              |
| `@Assert.email`                      | RFC 5322 shape                        |
| `@Assert.url`                        | http / https URL                      |
| `@Assert.uuid`                       | UUID-shaped string                    |
| `@Assert.regex(pattern)`             | matches RE2 pattern                   |
| `@Assert.minLength(n)`               | string length ≥ n                     |
| `@Assert.maxLength(n)`               | string length ≤ n                     |
| `@Assert.length(min: n, max: m)`     | string length in [n, m]               |
| `@Assert.range(min: n, max: m)`      | numeric value in [n, m]               |
| `@Assert.positive`                   | numeric value > 0                     |
| `@Assert.choice(["a", "b", "c"])`    | value is one of the listed alternatives |

Multiple decorators on one field compose: all must pass.

## Custom validators

Register a named validator on the app:

```gb
gebweb.registerAssertion(app, "strongPassword",
    func(any v, list<any> args, dict<string, any> named): ?string {
        if ((v as string).length() < 12) {
            return "must be at least 12 characters";
        }
        return null;
    });
```

The validator returns `null` on success or an error-message string on
failure. Once registered, use `@Assert.strongPassword` on a field.

### Validators that take arguments

A decorator like `@Assert.range(13, max: 120)` passes `13` in
`args` and `120` in `named`. Reach for both when your validator
accepts mixed positional and named options:

```gb
gebweb.registerAssertion(app, "range",
    func(any v, list<any> args, dict<string, any> named): ?string {
        int min = args.length() > 0 ? args[0] as int : 0;
        int max = named.contains("max") ? named["max"] as int : 999999;
        let n = v as int;
        if (n < min || n > max) {
            return "must be between " + (min as string) + " and " + (max as string);
        }
        return null;
    });
```

Use the validator with either form: `@Assert.range(13)` (just a
minimum), `@Assert.range(min: 13, max: 120)` (both named), or
`@Assert.range(13, max: 120)` (mixed).

## Validation failure shape

A failed validation throws `UnprocessableEntityError`. The framework
catches it and emits a Problem Details response:

```json
{
  "type": "about:blank",
  "title": "Unprocessable Entity",
  "status": 422,
  "detail": "validation failed",
  "errors": [
    {"field": "email", "message": "must be a valid email"},
    {"field": "password", "message": "must be 12+ chars"}
  ]
}
```

Each entry in `errors` names the failing field and a human-readable
message. Multiple failures across multiple fields are surfaced
together.

## Nested validation with `@Valid`

Request bodies are rarely flat. Mark a field `@Valid` to cascade
validation into the value it holds: a nested DTO, each element of a
`list<DTO>`, or each value of a `dict<string, DTO>`. Nested failures
are reported with a dotted / indexed / keyed path so the client can
point at the exact field.

```gb
class Address {
    @Assert.notBlank string postcode;
}

class LineItem {
    @Assert.notBlank string sku;
}

class CreateOrder {
    @Assert.notBlank string customer;

    @Valid Address shipTo;
    @Valid list<LineItem> items;
}
```

A POST whose `shipTo.postcode` is blank and whose second line item has
no `sku` returns:

```json
{
  "status": 422,
  "errors": [
    {"field": "shipTo.postcode", "message": "must not be blank"},
    {"field": "items[1].sku",    "message": "must not be blank"}
  ]
}
```

The framework deserializes the posted JSON into real nested instances
before validating, so `@Valid` works the same on a request body as on a
hand-built object. A null nested value is skipped rather than treated as
an error, and `@Valid` is only meaningful on class-typed fields.

## Validating instances outside the request path

`gebweb.validateInstance(app, instance)` runs the same validators
manually. Useful for re-validating during DTO transformation or in a
background job:

```gb
let failures = gebweb.validateInstance(app, dto);
if (failures.length() > 0) {
    /* handle without throwing */
}
```

## Reference

- `gebweb.registerAssertion(app, name, validator): GebwebApp` -
  Register a custom `@Assert.<name>` rule.
- `gebweb.validateInstance(app, instance): list<ValidationError>` -
  Run registered validators against an instance.
- Built-in decorators: `@Assert.notBlank`, `@Assert.notNull`,
  `@Assert.email`, `@Assert.url`, `@Assert.uuid`,
  `@Assert.regex(p)`, `@Assert.minLength(n)`,
  `@Assert.maxLength(n)`, `@Assert.length(min, max)`,
  `@Assert.range(min, max)`, `@Assert.positive`,
  `@Assert.choice([...])`.
- Validator callable: `func(any v, list<any> args,
  dict<string, any> named): ?string` - null on success,
  error-message string on failure.
- `@Valid` on a field cascades validation into a nested DTO, a
  `list<DTO>`, or a `dict<string, DTO>`; nested failures carry a
  dotted / indexed / keyed field path.
