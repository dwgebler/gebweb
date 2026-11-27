# Validation

<!-- doctest:strict -->

Validation rules are declared on input classes with `@Assert.*` decorators.
When a handler binds a request body to such a class, Gebweb checks every rule
before calling the handler. A JSON client gets a 422 response listing each
failing field; an HTML form gets its values and errors back after a redirect
when [form rehydration](25-forms.md) is on. The handler only ever sees valid
input.

## Declaring rules

<!-- doctest:run -->
```gb
import gebweb;
import io;

class Address {
    @Assert.notBlank
    string city;

    @Assert.regex("^[0-9]{5}$")
    string postcode;
}

class LineItem {
    @Assert.notBlank
    string sku;

    @Assert.range(min: 1, max: 99)
    int quantity;
}

class NewOrder {
    @Assert.email
    string email;

    @Valid
    Address shipTo;

    @Valid
    @Assert.count(min: 1)
    list<LineItem> items;

    @Assert.maxLength(200)
    ?string note;
}

@Controller("/orders")
class OrderController extends gebweb.Controller {
    @Post("")
    func create(@Body NewOrder order): gebweb.Response {
        return this.created({"items": order.items.length()}, "/orders/1");
    }
}

let client = gebweb.TestClient(gebweb.app());
let ok = client.post("/orders", {
    "email": "ada@example.com",
    "shipTo": {"city": "London", "postcode": "12345"},
    "items": [{"sku": "B-1", "quantity": 2}],
});
io.println("${ok.status} ${ok.text()}");

let bad = client.post("/orders", {
    "email": "not-an-email",
    "shipTo": {"city": "", "postcode": "W1"},
    "items": [{"sku": "B-1", "quantity": 2}, {"sku": "", "quantity": 500}],
});
io.println("${bad.status} ${bad.json()["title"]}");
for (e in bad.json()["errors"] as list<any>) {
    io.println("  ${e["field"]} [${e["code"]}] ${e["message"]}");
}
```

Output:

```text
201 {"items":1}
422 Validation failed
  email [email] must be a valid email address
  items[1].quantity [range] must be between 1 and 99
  items[1].sku [notBlank] must not be blank
  shipTo.city [notBlank] must not be blank
  shipTo.postcode [regex] must match pattern ^[0-9]{5}$
```

- Several rules on one field must all pass.
- `@Valid` on a field checks the object it holds against that class's rules:
  a nested object, each element of a list, or each value of a dict. Failures
  are reported with the path to the field.
- A rule other than a presence rule (`notBlank`, `notNull`, `isNull`,
  `blank`) skips a null value, so `note` may be left out. Add
  `@Assert.notNull` when a field is required and must also pass another rule.
- Every failure is reported at once, not only the first, ordered by field
  path.

## The failure response

A failed validation is a 422 Problem Details response:

```json
{
  "type": "https://gebweb.dev/errors/validation",
  "title": "Validation failed",
  "status": 422,
  "detail": "validation failed",
  "errors": [
    {"field": "email", "code": "email", "message": "must be a valid email address"}
  ]
}
```

Each error has the `field` path, the `code` of the rule that failed (the
`@Assert` name), and a `message`. Clients can match on `code` and show their
own text; see [Internationalisation](30-i18n.md#validation-messages) for
translated messages. A body that is not valid JSON, or has a field of the
wrong type, is a 400 before validation runs; see
[Parameter binding](03-parameter-binding.md#request-bodies).

## Built-in rules

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
| `@Assert.length(min: n, max: m)` | string length in [n, m]; either bound may be omitted |

Null is skipped.

### Numeric range

| Decorator                        | Checks                  |
|----------------------------------|-------------------------|
| `@Assert.range(min: n, max: m)`  | int, float or decimal value in [n, m]; either bound may be omitted |

A non-numeric value fails. Null is skipped.

`range`, `length` and `count` take their bounds by name (`min:`, `max:`),
by position (`@Assert.range(1, 5)`) or mixed (`@Assert.range(13, max: 120)`).
With one bound the check is one-sided: `@Assert.range(min: 13)` rejects 12
and accepts any larger number. One of these rules with no bound at all stops
the app at its first request with an `UnknownAssertionError` naming the
class and field.

### Collection size

| Decorator                  | Checks                                     |
|----------------------------|--------------------------------------------|
| `@Assert.count(min: n, max: m)`  | list or dict has between n and m items; either bound may be omitted |

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

## Custom rules

Register a rule with `gebweb.registerAssertion(app, name, fn)` and use it as
`@Assert.<name>`. The function receives the field's value, the decorator's
positional arguments and its named arguments. It returns `null` when the value
is valid, or a message when it is not:

<!-- doctest:run -->
```gb
import gebweb;
import io;

class Shipment {
    @Assert.multipleOf(5)
    int crates;

    @Assert.multipleOf(10, allowZero: true)
    int pallets;
}

func multipleOf(any value, list<any> args, dict<string, any> named): ?string {
    if (value == null) {
        return null;
    }
    int step = args[0] as int;
    bool allowZero = named.contains("allowZero") && (named["allowZero"] as bool);
    int n = value as int;
    if (n == 0 && !allowZero) {
        return "must not be zero";
    }
    if (n % step != 0) {
        return "must be a multiple of ${step}";
    }
    return null;
}

@Controller("/shipments")
class ShipmentController {
    @Post("")
    func create(Shipment shipment): dict<string, any> {
        return {"crates": shipment.crates, "pallets": shipment.pallets};
    }
}

let app = gebweb.app();
gebweb.registerAssertion(app, "multipleOf", multipleOf);
let client = gebweb.TestClient(app);
io.println(client.post("/shipments", {"crates": 15, "pallets": 0}).text());
for (e in client.post("/shipments", {"crates": 0, "pallets": 7}).json()["errors"] as list<any>) {
    io.println("${e["field"]} [${e["code"]}] ${e["message"]}");
}
```

Output:

```text
{"crates":15,"pallets":0}
crates [multipleOf] must not be zero
pallets [multipleOf] must be a multiple of 10
```

- Return `null` for a null value unless the rule is about presence, as the
  built-in rules do.
- Instead of a string, a rule can return `{"message": ..., "params": {...}}`.
  The parameters are used to fill in translated messages.
- Register rules before the app handles its first request. At that point
  Gebweb checks every `@Assert` name used by the bound classes, and a name
  that is neither built in nor registered, such as `@Assert.NotBlank` for
  `@Assert.notBlank`, stops the app with an `UnknownAssertionError` naming
  the class and field.
- Registering a built-in name, such as `range`, replaces that rule for the
  app.

## Validating outside a request

`validateInstance` in the `gebweb.app` module runs the same rules on any
object and returns its failures, for example in a job or a command-line
import:

<!-- doctest:run -->
```gb
import gebweb;
import gebweb.app as appmod;
import io;

class Contact {
    @Assert.notBlank
    string name;

    @Assert.email
    string email;
}

let app = gebweb.app([]);
let contact = Contact();
contact.name = "Ada";
contact.email = "ada-at-example";
for (failure in appmod.validateInstance(app, contact)) {
    io.println("${failure.field}: ${failure.message}");
}
```

Output:

```text
email: must be a valid email address
```

Each failure is a `ValidationError` with `field`, `code`, `message` and
`params`. Custom rules registered on the app apply here too.

## Reference

- `gebweb.registerAssertion(app, name, fn)` registers `@Assert.<name>`;
  `fn(any value, list<any> args, dict<string, any> named)` returns `null`, a
  message, or `{"message": ..., "params": {...}}`.
- `appmod.validateInstance(app, instance): list<ValidationError>` from
  `import gebweb.app as appmod`.
- `@Valid` on a field validates the object, list elements or dict values it
  holds.
- `UnknownAssertionError` is thrown at the first request for an unknown rule
  name or a `range`/`length`/`count` rule without a bound.
- Failures: 422 with `type`, `title`, `status`, `detail` and `errors`
  (`field`, `code`, `message`).
