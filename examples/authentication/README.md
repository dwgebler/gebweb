# JWT authentication example

This app verifies a configured account's password, issues a short-lived token,
loads the user from its subject, and protects `/me` and `/admin`. Password hashes
and roles come from trusted server configuration and code. The example account
has only `user`, so `/admin` returns 403 even after login.

Use Geblang 1.47.0 or later and Gebweb 1.11.1.

From this directory, configure the account and signing key:

```sh
export AUTH_USERNAME=ada
read -r -s -p 'Password: ' AUTH_PASSWORD
export AUTH_PASSWORD
```

Create `make_credentials.gb` temporarily in this directory:

```gb
import crypt;
import secrets;
import io;
io.println(crypt.passwordHash(secrets.requireEnv("AUTH_PASSWORD")));
io.println(secrets.randomBase64(48));
```

Run `geblang make_credentials.gb`. Set `AUTH_PASSWORD_HASH` to the first output
line and `JWT_SECRET` to the second, using single quotes around each value.
Keep these values out of source control. Remove the temporary script and unset
`AUTH_PASSWORD`. The server needs only the password hash.

```sh
geblang check --strict src/
geblang check --strict tests/
geblang test tests/
geblang test --runtime=evaluator tests/
geblang -m main
```

In another terminal:

```sh
curl -i http://127.0.0.1:8080/me
curl -i -H 'Content-Type: application/json' \
  -d '{"username":"ada","password":"your-password"}' \
  http://127.0.0.1:8080/login
```

Copy `access_token` from the successful login response:

```sh
curl -i -H 'Authorization: Bearer YOUR_TOKEN' http://127.0.0.1:8080/me
curl -i -H 'Authorization: Bearer YOUR_TOKEN' http://127.0.0.1:8080/admin
```

The first request returns 401 without a token, `/me` returns 200 with a valid
one, and `/admin` returns 403. A malformed, expired or incorrectly signed token
returns 401. Login and authenticated responses carry `Cache-Control: no-store`.
Login attempts are rate limited; production deployments should use a shared
limiter and account-aware abuse controls.

The five tests cover credentials, identity/role gates, invalid tokens and
configuration.

Run `GEBLANG_BIN=/path/to/geblang ./verify.sh` to verify both runtimes, cache
reuse, imported custom authenticators, real HTTP requests and standalone builds.
The script requires Python 3 and cleans up its temporary files and servers.

For a standalone build:

```sh
geblang build --entry main --out /tmp/gebweb-auth-example .
/tmp/gebweb-auth-example
```

Use `--host`, `--port`, `GEBWEB_HOST` or `GEBWEB_PORT` to override
`127.0.0.1:8080`. CLI help works before credentials are loaded. Use TLS at the server or trusted reverse
proxy for production. The single configured account is for demonstrating the
flow; replace the credential check and subject resolver with a user repository
for an application. The resolver should reject deleted or disabled accounts and
load current roles. A JWT is signed, not encrypted. This app has no refresh
endpoint or token revocation store: deleting the client token does not invalidate
copies, which remain usable until their 15-minute expiry.
