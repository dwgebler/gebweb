# Building authentication

Use Geblang 1.46.0 or later with the catch-body finalization repair so request
cleanup also runs when a listener error is caught and rethrown on the VM.

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
geblang check src/
geblang test tests/
geblang test --runtime=evaluator tests/
geblang -m main
```

Never place the password, hash or signing key in committed source or a public
configuration file. The example uses one configured account to keep the entire
flow visible. A real application supplies a user repository instead.

## What happens at login

The public `POST /login` route accepts a typed username/password body. It uses
`crypt.passwordVerify` against the stored password hash. It verifies the password
even when the username is wrong and returns the same generic failure either
way. Do not return details such as "account exists" or "password was wrong".

After successful credential verification it issues a 15-minute token:

```gb
let token = gebweb.jwtIssue(
    signingKey,
    {
        "sub": account.id,
        "iss": "example-auth",
        "aud": "example-api",
    },
    900
);
```

`sub` is a stable account identifier, `iss` identifies the issuer and `aud`
identifies the intended consumer. `iat` and `exp` are added automatically.
The response includes `access_token`, `token_type: Bearer` and `expires_in: 900`,
with `Cache-Control: no-store`.

The client sends the token on subsequent requests:

```http
GET /me HTTP/1.1
Authorization: Bearer <access_token>
```

The example rate-limits login. In production, combine request and account-aware
limits using shared storage when there are several app instances. Use HTTPS;
a password or bearer token sent over unencrypted HTTP can be stolen.

## Verification is followed by account lookup

App setup registers a JWT authenticator:

```gb
gebweb.useAuth(app, CurrentUser, [
    gebweb.jwtAuthenticator(verificationKey, fromClaims, {
        "allowedAlgs": ["HS256"],
        "issuer": "example-auth",
        "audience": "example-api",
    }),
]);
```

For HS256, `verificationKey` is the same random secret used to sign. The
allow-list, signature, claim types, issuer, audience and time checks run before
`fromClaims`. Expiry and subject are required by default. The resolver loads the
account using `sub`, returns `null` for a deleted or disabled account, and returns
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
[RSA JWTs in the main authentication chapter](08-auth.md#rsa-jwts-issue-verify-and-authenticate-a-request).

With HMAC, every verifier knows the signing secret and can mint tokens. That is
suitable when issuance and verification belong to the same trust boundary.

With RSA, ECDSA or EdDSA, the issuer holds a private key while API consumers need
only a public key. For example, generate an RSA key pair outside the application:

```sh
umask 077
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 -out jwt-private.pem
openssl pkey -in jwt-private.pem -pubout -out jwt-public.pem
```

Load the PEM files through your configuration/secret provider. Sign with the
private key and explicitly select RS256:

```gb
let token = gebweb.jwtIssue(
    privatePem,
    {"sub": account.id, "iss": "example-auth", "aud": "example-api"},
    900,
    {"alg": "RS256", "kid": "key-2026-11"}
);
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
