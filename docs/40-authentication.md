# Building authentication

<!-- doctest:strict -->

Use Geblang 1.47.0 or later with Gebweb 1.11.1.

Authentication has several steps: verify a credential, identify the account,
check that it is still active, then enforce access rules. A JWT handles only
part of that process. This guide walks through the runnable
`examples/authentication` application, then shows asymmetric keys and extension
hooks. See [Authentication and roles](08-auth.md) for the API reference.

## Run the complete example

The example contains:

- `src/security_app.gb`: account type, login and protected handlers, app factory.
- `src/main.gb`: configuration and serving.
- `tests/auth_test.gb`: login failure/success, authenticated identity and role denial.
- `README.md`: credential generation, commands, HTTP requests and build instructions.

From its directory, follow the README to configure `AUTH_USERNAME`,
`AUTH_PASSWORD_HASH` and a random `JWT_SECRET` of at least 32 bytes. Then:

```sh
geblang check --strict src/
geblang check --strict tests/
geblang test tests/
geblang test --runtime=evaluator tests/
geblang -m main
```

Never place the password, hash or signing key in committed source or a public
configuration file. The example uses one configured account to keep the entire
flow visible. A real application supplies a user repository instead.

The factory's complete implementation is below. It registers its configured
controller instance before discovery and returns a fresh user object from each
successful subject lookup. The password hash and signing key stay on the server.

### `src/security_app.gb`

<!-- doctest:file security_app.gb -->
```gb
module security_app;
import crypt;
import gebweb;
import gebweb.errors as errors;
import http;

export class CurrentUser {
    string id;
    string name;
    list<string> roles;
    func CurrentUser(string id, string name) {
        this.id = id;
        this.name = name;
        this.roles = ["user"];
    }
}

class LoginInput {
    string username;
    string password;
}

@Controller("")
class AuthController {
    string username;
    string passwordHash;
    string signingKey;

    func AuthController(string username, string passwordHash, string signingKey) {
        this.username = username;
        this.passwordHash = passwordHash;
        this.signingKey = signingKey;
    }

    @Post("/login")
    @RateLimit(2, 5)
    func login(LoginInput input): http.Response {
        bool passwordValid = crypt.passwordVerify(input.password, this.passwordHash);
        if (input.username != this.username || !passwordValid) {
            throw errors.UnauthorizedError("invalid credentials");
        }
        let token = gebweb.jwtIssue(
            this.signingKey,
            {"sub": "account-1", "iss": "example-auth", "aud": "example-api"},
            900
        );
        return http.response({
            "status": 200,
            "headers": {"Cache-Control": "no-store"},
            "body": {"access_token": token, "token_type": "Bearer", "expires_in": 900},
        });
    }

    @Get("/me")
    @Auth
    func me(CurrentUser user): CurrentUser {
        return user;
    }

    @Get("/admin")
    @RequiresRole("admin")
    func admin(): string {
        return "admin";
    }
}

export func createApp(string username, string passwordHash, string signingKey): gebweb.GebwebApp {
    if (username.trim().isEmpty() || passwordHash.trim().isEmpty() || (signingKey as bytes).length() < 32) {
        throw ValueError("configure a username, password hash and signing key of at least 32 bytes");
    }
    let app = gebweb.app();
    gebweb.registerInstance(app, AuthController, AuthController(username, passwordHash, signingKey));
    gebweb.useAuth(app, CurrentUser, [
        gebweb.jwtAuthenticator(
            signingKey,
            func(dict<string, any> claims): ?CurrentUser {
                if (claims["sub"] != "account-1") {
                    return null;
                }
                return CurrentUser("account-1", username);
            },
            {"allowedAlgs": ["HS256"], "issuer": "example-auth", "audience": "example-api"}
        ),
    ]);
    gebweb.useSecurity(app, {});
    gebweb.after(app, func(gebweb.Request request, http.Response response): http.Response {
        return response.withHeader("Cache-Control", "no-store");
    });
    return app;
}
```

This runnable check uses temporary test credentials, logs to stderr and never
prints a token. It exercises the public login, identity and role gates:

<!-- doctest:run -->
```gb
import crypt;
import gebweb;
import io;
import log;
import security_app;

let app = security_app.createApp(
    "ada",
    crypt.passwordHash("example-only-password"),
    "example-only-signing-key-at-least-32-bytes"
);
gebweb.useLogger(app, log.stderr());
let client = gebweb.TestClient(app);
io.println(client.get("/me").status);
io.println(client.post("/login", {"username": "ada", "password": "wrong"}).status);
let login = client.post("/login", {"username": "ada", "password": "example-only-password"});
io.println(login.status);
io.println(login.headers["Cache-Control"]);
io.println(login.json()["token_type"]);
io.println(login.json()["expires_in"]);
let headers = {"Authorization": "Bearer " + (login.json()["access_token"] as string)};
let me = client.request("GET", "/me", null, headers);
io.println(me.status);
io.println(me.json()["name"]);
io.println(client.request("GET", "/admin", null, headers).status);
io.println(client.request("GET", "/me", null, {"Authorization": "Bearer broken"}).status);
```

```text
401
401
200
no-store
Bearer
900
200
ada
403
401
```

The entry point checks CLI arguments before reading secrets. Discovery runs
before the CLI serves requests; use standard `--host` / `--port` flags or
`GEBWEB_HOST` / `GEBWEB_PORT` to change the loopback address.

### `src/main.gb`

<!-- doctest:file main.gb -->
```gb
module main;
import gebweb;
import gebweb.cli as cli;
import security_app;
import secrets;

export func main(list<string> args): int {
    cli.checkArgs(args, {"name": "authentication"});
    let app = security_app.createApp(
        secrets.requireEnv("AUTH_USERNAME"),
        secrets.requireEnv("AUTH_PASSWORD_HASH"),
        secrets.requireEnv("JWT_SECRET")
    );
    gebweb.cli(app, {"name": "authentication", "host": "127.0.0.1"});
    return 0;
}
```

`--help` and `--version` can exit before credentials are loaded. This example
has no database or other application-owned handle to close. The CLI supplies
graceful request draining; register shutdown hooks for resources you add.

## What happens at login

The public `POST /login` route accepts a typed username/password body. It uses
`crypt.passwordVerify` against the stored password hash. It verifies the password
even when the username is wrong and returns the same generic failure either
way. Do not return details such as "account exists" or "password was wrong".

After successful credential verification, the handler issues a 15-minute token
using trusted `sub`, `iss` and `aud` values. `sub` is a stable account identifier,
`iss` identifies the issuer and `aud` identifies the intended consumer. The
response includes `access_token`, `token_type: Bearer` and `expires_in: 900`,
with `Cache-Control: no-store`. The app also sets `no-store` on other responses.

`jwtIssue` adds `iat` and, for a positive TTL, `exp`. Supplied claims can override
those generated values. Build the claims from trusted server data, rather than
passing a client-supplied dictionary to the issuer. A zero or negative TTL omits
the generated expiry; the example verifier rejects tokens without an expiry.

The client sends the token on subsequent requests:

```http
GET /me HTTP/1.1
Authorization: Bearer <access_token>
```

The example uses `@RateLimit(2, 5)`: a bucket with two tokens per second and
a burst of five, keyed by IPv4 address or IPv6 /64 network. IPv6 clients sharing
a /64 consume the same bucket. This is an in-process limit. In production, combine request
and account-aware limits using shared storage when there are several app instances. Use HTTPS;
a password or bearer token sent over unencrypted HTTP can be stolen.

## Verification is followed by account lookup

The factory registers `CurrentUser` with `useAuth` and a `jwtAuthenticator`.
Its verification policy is explicit:

| Setting | Example value | Effect |
|---------|---------------|--------|
| `allowedAlgs` | `["HS256"]` | Only accept this signature algorithm. Required by `jwtAuthenticator`. |
| `issuer` | `example-auth` | Require this exact `iss`. |
| `audience` | `example-api` | Require this `aud`, or membership in an audience list. |
| `requiredClaims` | Default `["sub", "exp"]` | Require identity and expiry; keep these defaults for this flow. |
| `leewaySeconds` | Default `0` | Tolerance for expiry and not-before checks. |

For HS256, `verificationKey` is the same random secret used to sign. The
allow-list, signature, claim types, issuer, audience, expiry and optional `nbf`
checks run before the resolver. `iat` must be an integer if present; checking
its age requires `maxAgeSeconds`, which this example does not configure. Expiry and subject are required by
default. The resolver loads the account using `sub`, returns `null` for a deleted or disabled account, and returns
an instance of `CurrentUser` otherwise.

The example recognizes its configured account and obtains its roles from server
code. In an application, load those roles from trusted account data. Accepting
client-supplied roles at login would let the client choose its privileges.
Loading current roles also avoids waiting for token expiry after a role change.

`@Auth` requires a user; `@RequiresRole("admin")` also requires an admin role.
The user parameter is injected by its resolved class identity, including when
the account class is imported through an alias. The handler can also read
`request.user()`.

A missing or invalid token produces 401. A valid token for a user without the
required role produces 403. This is why `/admin` returns 403 for the example's
ordinary account even after a successful login.

## Shared secrets versus asymmetric keys

For complete key-generation, RSA issuance, verification and API scripts, see
[RSA JWTs in the main authentication chapter](08-auth.md#signed-tokens-jwt).

With HMAC, every verifier knows the signing secret and can mint tokens. That is
suitable when issuance and verification belong to the same trust boundary.

With RSA, ECDSA or EdDSA, the issuer holds a private key while API consumers need
only a public key. For example, generate an RSA key pair outside the application:

```sh
umask 077
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 -out jwt-private.pem
openssl pkey -in jwt-private.pem -pubout -out jwt-public.pem
```

Load production PEM files through your configuration/secret provider. This
runnable example generates throwaway keys, signs with RS256 and verifies using
only the public key:

<!-- doctest:run -->
```gb
import crypt;
import gebweb;
import io;

string privatePem = crypt.generateRsaKey(3072);
string publicPem = crypt.publicKey(privatePem);
let claims = {"sub": "account-1", "iss": "example-auth", "aud": "example-api"};
string token = gebweb.jwtIssue(privatePem, claims, 900, {"alg": "RS256", "kid": "example-key"});
let policy = {
    "allowedAlgs": ["RS256"],
    "issuer": "example-auth",
    "audience": "example-api",
    "requiredClaims": ["sub", "exp"],
};
let verified = gebweb.jwtVerify(publicPem, token, policy) as dict<string, any>;
io.println(verified["sub"]);
string forged = gebweb.jwtIssue(crypt.generateRsaKey(3072), claims, 900, {"alg": "RS256"});
io.println(gebweb.jwtVerify(publicPem, forged, policy) == null);
```

```text
account-1
true
```

Configure consumers with the public key and `allowedAlgs: ["RS256"]`. Do not
include HMAC algorithms in an asymmetric verification policy. ECDSA uses the
appropriate EC key and ES algorithm; EdDSA uses an Ed25519 key.

For rotation, distribute a JWKS containing both current and retiring public
keys, each with a unique `kid`. New tokens use the current private key and its
`kid`; consumers select the matching public key. Keep the old public key until
its tokens have expired, including permitted clock tolerance. `useJwks` can
publish this set, while `jwtAuthenticator` accepts an already loaded JWK/JWKS
dictionary as its verification key. Replace configuration during a controlled
restart or deployment; registration is not a concurrent key-management API.

The JWT helper does not fetch keys or trust token-provided URLs. For an external
identity provider, use trusted issuer configuration and deliberate key retrieval;
see [OIDC](33-oidc.md). An OIDC ID token and an API access token have different
purposes and must not be accepted interchangeably just because both are JWTs.

## Sessions, token storage and logout

For browser-rendered applications, server-side sessions are often simpler than
JWTs. Set HttpOnly, Secure and appropriate SameSite cookie attributes, rotate
session identifiers at login, invalidate server-side sessions at logout, and
validate CSRF tokens on state-changing requests. See [CSRF](23-csrf.md) and
[Building bookmarks](38-building-bookmarks.md).

A signed JWT is readable, not encrypted. Keep passwords, secrets and unnecessary
personal data out of it. Avoid putting tokens in URLs, logs or JavaScript-accessible
persistent browser storage. If authentication uses a cookie, apply CSRF
protection; choosing JWT as the cookie's content does not remove that need.

Deleting a bearer token from a client does not invalidate stolen copies. The
example intentionally has no refresh or revocation endpoint; tokens remain valid
until expiry, unless account lookup rejects the account. Immediate per-token
revocation needs server-side state, usually keyed by a token identifier (`jti`).
A refresh flow needs its own stored, expiring credentials, atomic rotation and
reuse detection. Do not implement refresh by repeatedly accepting an expired
access token. These mechanisms are not supplied by `jwtIssue`.

## Hook into authentication and request handling

Use a custom `Authenticator` or `CallbackAuthenticator` for another credential
format, and compose it with built-ins through `useAuth`. Explicitly distinguish
absent credentials from rejected ones. See the [custom authenticator contract](08-auth.md#custom-authenticators).

Use `gebweb.authenticate(app, request)` when middleware or a public handler needs
an optional identity. Repeated calls and protected route gates share the same
request decision. Do not store a current user on a shared controller or service.

Use `auth.authenticated`, `auth.rejected` and `auth.absent` events for audit
observers. Record selected fields such as outcome, authenticator name and account
id, never the Authorization header or raw request body. The event payload includes
the request, so indiscriminate logging would expose credentials.

Middleware controls request admission and response transformation. Lifecycle
events provide shared extension points for plugins and application listeners.
For event names, ordering and failure behavior, see [Events](19-events.md#framework-lifecycle-events).

## Verify and extend the application

The checked-out example has five tests covering credentials, no-store responses,
identity/role gates, malformed signatures, issuer/audience/time policy failures,
unknown accounts and incomplete configuration. Run both runtimes as shown above.
Its `verify.sh` additionally tests real HTTP on the VM and evaluator, a second
VM launch, imported custom authenticators and standalone binaries with source
files removed. It requires Bash and Python 3, uses temporary credentials and
storage, and cleans up its server processes.

Keep account activation, role lookup and credential storage in a repository as
the application grows. Controllers and registered services are shared; retain
request identity on the request rather than in a shared service field. For the
full construction and lifetime rules, see [dependency injection](06a-dependency-injection.md)
and [concurrency](35-concurrency.md).

## Reference

| Surface | Responsibility |
|---------|----------------|
| `crypt.passwordVerify(password, hash)` | Verify the submitted password against a stored hash. |
| `gebweb.jwtIssue(key, claims, ttlSeconds, opts = {})` | Sign trusted claims; does not perform account lookup. |
| `gebweb.jwtAuthenticator(key, resolver, opts)` | Verify the bearer token before resolving an account. |
| `gebweb.useAuth(app, UserType, authenticators)` | Configure identity resolution and injection for the app. |
| `@Auth` / `@RequiresRole("admin")` | Require authentication / a trusted role. |
| `gebweb.authenticate(app, request)` | Resolve an optional identity through the request's cached decision. |

See [authentication and roles](08-auth.md), [security](27-security.md),
[CSRF](23-csrf.md) and [OIDC](33-oidc.md) for the corresponding API contracts.
