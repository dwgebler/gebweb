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

Multiple decorators on one field compose: all must pass.

**Null-skip rule:** with the exception of the presence constraints
(`notBlank`, `notNull`, `isNull`, `blank`), all other validators skip a
null field silently. A null value only fails when a presence constraint
requires it to be set. Use `@Assert.notNull` alongside another constraint
to make a field both required and further validated.

### Presence

| Decorator           | Checks                                      |
|---------------------|---------------------------------------------|
| `@Assert.notBlank`  | non-null and non-empty string (after trim)  |
| `@Assert.notNull`   | value is not null                           |
| `@Assert.isNull`    | value is null                               |
| `@Assert.blank`     | value is null, or a string empty after trim |

### Boolean

| Decorator          | Checks              |
|--------------------|---------------------|
| `@Assert.isTrue`   | value is true       |
| `@Assert.isFalse`  | value is false      |

### Type

| Decorator              | Checks                                                                         |
|------------------------|--------------------------------------------------------------------------------|
| `@Assert.type("name")` | value is of the named type: `"int"`, `"float"`, `"string"`, `"bool"`, `"list"`, or `"dict"` |

An unknown name is a configuration error and always fails with a clear
message. Null is skipped.

### Numeric sign

| Decorator                  | Checks                          |
|----------------------------|---------------------------------|
| `@Assert.positive`         | numeric value > 0               |
| `@Assert.negative`         | numeric value < 0               |
| `@Assert.positiveOrZero`   | numeric value >= 0              |
| `@Assert.negativeOrZero`   | numeric value <= 0              |

A non-numeric value fails these constraints. Null is skipped.

### Numeric comparison

| Decorator                        | Checks                  |
|----------------------------------|-------------------------|
| `@Assert.greaterThan(n)`         | numeric value > n       |
| `@Assert.greaterThanOrEqual(n)`  | numeric value >= n      |
| `@Assert.lessThan(n)`            | numeric value < n       |
| `@Assert.lessThanOrEqual(n)`     | numeric value <= n      |

A non-numeric value fails these constraints. Null is skipped.

### General equality

| Decorator               | Checks        |
|-------------------------|---------------|
| `@Assert.equalTo(v)`    | value == v    |
| `@Assert.notEqualTo(v)` | value != v    |

Works with any comparable value. Null is skipped.

### String format

| Decorator                    | Checks                            |
|------------------------------|-----------------------------------|
| `@Assert.email`              | RFC 5322 shape                    |
| `@Assert.url`                | http / https URL                  |
| `@Assert.uuid`               | UUID-shaped string                |
| `@Assert.regex(pattern)`     | matches RE2 pattern               |

### String length

| Decorator                        | Checks                  |
|----------------------------------|-------------------------|
| `@Assert.minLength(n)`           | string length >= n      |
| `@Assert.maxLength(n)`           | string length <= n      |
| `@Assert.length(min: n, max: m)` | string length in [n, m] |

Null is skipped.

### Numeric range

| Decorator                        | Checks                  |
|----------------------------------|-------------------------|
| `@Assert.range(min: n, max: m)`  | numeric value in [n, m] |

Null is skipped.

### Collection size

| Decorator                  | Checks                                     |
|----------------------------|--------------------------------------------|
| `@Assert.count(min, max)`  | list or dict has between min and max items |

A non-collection value fails. Null is skipped.

### Date and time (ISO only)

| Decorator           | Checks                                   |
|---------------------|------------------------------------------|
| `@Assert.date`      | string is a valid ISO date (YYYY-MM-DD)  |
| `@Assert.datetime`  | string is a valid ISO 8601 datetime      |
| `@Assert.time`      | string is a valid time (HH:MM:SS)        |

For custom date formats, use `@Assert.regex` instead. Null is skipped.

### Network and format

| Decorator          | Checks                              |
|--------------------|-------------------------------------|
| `@Assert.ip`       | valid IPv4 or IPv6 address string   |
| `@Assert.json`     | valid JSON string                   |

Null is skipped.

### Membership

| Decorator                          | Checks                                  |
|------------------------------------|-------------------------------------------|
| `@Assert.choice(["a", "b"])`    | value is one of the listed alternatives |
| `@Assert.in(["a", "b"])`        | equivalent to choice (legacy name)      |

### Example

```gb
class EventDTO {
    @Assert.notBlank
    string title;

    @Assert.notNull
    @Assert.date
    string startsOn;

    @Assert.positiveOrZero
    int capacity;

    @Assert.notNull
    @Assert.type("list")
    @Assert.count(1, 10)
    any tags;

    @Assert.ip
    string sourceIp;
}
```

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
- Built-in decorators - presence: `@Assert.notBlank`, `@Assert.notNull`,
  `@Assert.isNull`, `@Assert.blank`; boolean: `@Assert.isTrue`,
  `@Assert.isFalse`; type: `@Assert.type(name)`; numeric sign:
  `@Assert.positive`, `@Assert.negative`, `@Assert.positiveOrZero`,
  `@Assert.negativeOrZero`; numeric comparison: `@Assert.greaterThan(n)`,
  `@Assert.greaterThanOrEqual(n)`, `@Assert.lessThan(n)`,
  `@Assert.lessThanOrEqual(n)`; equality: `@Assert.equalTo(v)`,
  `@Assert.notEqualTo(v)`; string format: `@Assert.email`, `@Assert.url`,
  `@Assert.uuid`, `@Assert.regex(p)`; string length: `@Assert.minLength(n)`,
  `@Assert.maxLength(n)`, `@Assert.length(min, max)`; numeric range:
  `@Assert.range(min, max)`; collection: `@Assert.count(min, max)`;
  date/time: `@Assert.date`, `@Assert.datetime`, `@Assert.time`; network
  and format: `@Assert.ip`, `@Assert.json`; membership: `@Assert.choice([...])`.
- Validator callable: `func(any v, list<any> args,
  dict<string, any> named): ?string` - null on success,
  error-message string on failure.
- `@Valid` on a field cascades validation into a nested DTO, a
  `list<DTO>`, or a `dict<string, DTO>`; nested failures carry a
  dotted / indexed / keyed field path.
