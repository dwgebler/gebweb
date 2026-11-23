# RSA tokens, route gates and ownership policies

The main authentication chapter contains the source of these scripts and explains
each step. From this directory, using Geblang 1.46 or later and the local Gebweb
dependency:

```sh
umask 077
geblang generate_keys.gb
geblang issue_token.gb > token.txt
geblang api.gb
```

In another terminal in this directory:

```sh
curl -i http://localhost:8081/me
curl -i -H "Authorization: Bearer $(cat token.txt)" http://localhost:8081/me
curl -i -H "Authorization: Bearer $(cat token.txt)" http://localhost:8081/admin
```

Expect 401, 200 and 403. The issuer uses the private PEM to select RS256 and sign
a 15-minute token for `user-42`. Verification uses only the public PEM, an
explicit algorithm allow-list, issuer and audience. The API publishes its public
key set at `/.well-known/jwks.json`. The API maps the verified
subject to an account with a server-side `editor` role.

This issuer is a command-line demonstration, not a public login endpoint. A
login endpoint must verify credentials before issuing a token. Private PEMs and
tokens are ignored by Git; do not commit or publish them. Regenerating the keys
invalidates existing tokens unless verifiers retain the old public key.

Start the policy example with `geblang policy_api.gb`. It listens on port 8082
and accepts the same token:

```sh
curl -i -H "Authorization: Bearer $(cat token.txt)" http://localhost:8082/docs/d2
curl -i -X PATCH -H "Authorization: Bearer $(cat token.txt)" \
  -H 'Content-Type: application/json' -d '{"title":"Edited"}' \
  http://localhost:8082/docs/d1
curl -i -X PATCH -H "Authorization: Bearer $(cat token.txt)" \
  -H 'Content-Type: application/json' -d '{"title":"Edited"}' \
  http://localhost:8082/docs/d2
curl -i -X POST -H "Authorization: Bearer $(cat token.txt)" \
  -H 'Content-Type: application/json' -d '{"title":"New document"}' \
  http://localhost:8082/docs
```

The first response includes `canEdit: false`. Updating owned `d1` succeeds;
updating someone else's `d2` returns 403. Creating a document succeeds because
the type-level `create(user)` policy accepts editors. A missing document returns
404 after authentication; missing credentials return 401 before the handler.

The documents are recreated for each request to keep the example about
policy decisions. Changes are returned, not persisted. Replace the lookup and
write with a repository for an application. Both servers bind to all interfaces;
use HTTPS when deploying an API with bearer tokens.
