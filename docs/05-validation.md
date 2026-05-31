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
