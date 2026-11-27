#!/usr/bin/env bash
set -euo pipefail

example_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
geblang_bin=$(command -v "${GEBLANG_BIN:-geblang}")
gebweb_bin=$(command -v "${GEBWEB_BIN:-gebweb}")
geblang_bin="$(CDPATH= cd -- "$(dirname -- "$geblang_bin")" && pwd)/$(basename -- "$geblang_bin")"
gebweb_bin="$(CDPATH= cd -- "$(dirname -- "$gebweb_bin")" && pwd)/$(basename -- "$gebweb_bin")"
scratch=$(mktemp -d /tmp/bookmarks-e2e.XXXXXX)
project="$scratch/project"
port=${BOOKMARKS_TEST_PORT:-18092}
base="http://127.0.0.1:$port"
server_pid=

stop_server() {
    if [[ -n "$server_pid" ]]; then
        kill -TERM "$server_pid" 2>/dev/null || true
        wait "$server_pid"
        server_pid=
    fi
}

cleanup() {
    if [[ -n "$server_pid" ]]; then
        kill -TERM "$server_pid" 2>/dev/null || true
        wait "$server_pid" || true
    fi
    rm -rf -- "$scratch"
}
trap cleanup EXIT

mkdir "$project"
cp -R "$example_dir/src" "$example_dir/templates" "$project/"
ln -s "$example_dir/../.." "$scratch/framework"
cat >"$project/geblang.yaml" <<'YAML'
name: bookmarks
version: 0.1.0
source: src
dependencies:
  gebweb:
    path: ../framework
YAML

export DATABASE_URL="$scratch/bookmarks.db"
export SESSION_DIR="$scratch/sessions"
export CSRF_SECRET=bookmarks-verification-only-secret
export GEBWEB_HOST=127.0.0.1 GEBWEB_PORT="$port" GEBWEB_NO_TLS=1
unset GEBWEB_DOMAIN GEBWEB_SELF_SIGNED

start_server() {
    local directory=$1
    shift
    if curl --silent --max-time 1 "$base/" >/dev/null; then
        echo "Port $port is in use; choose BOOKMARKS_TEST_PORT." >&2
        exit 1
    fi
    (
        cd "$directory"
        exec "$@"
    ) >"$scratch/server.log" 2>&1 &
    server_pid=$!
    if ! curl --silent --show-error --fail --retry 20 --retry-connrefused --retry-delay 1 \
        --max-time 2 --cookie-jar "$scratch/cookies" "$base/" >"$scratch/page" 2>"$scratch/readiness.log"; then
        cat "$scratch/server.log" >&2
        cat "$scratch/readiness.log" >&2
        exit 1
    fi
    if ! kill -0 "$server_pid" 2>/dev/null; then
        cat "$scratch/server.log" >&2
        exit 1
    fi
}

request() {
    local expected=$1
    shift
    local status
    status=$(curl --silent --show-error --max-time 5 --cookie "$scratch/cookies" \
        --cookie-jar "$scratch/cookies" --dump-header "$scratch/headers" \
        --output "$scratch/body" --write-out '%{http_code}' "$@")
    if [[ "$status" != "$expected" ]]; then
        echo "Expected HTTP $expected, got $status" >&2
        cat "$scratch/body" >&2
        exit 1
    fi
}

cd "$project"
cat >"$project/src/cli_exit_probe.gb" <<'GB'
module cli_exit_probe;
import gebweb;

@Controller("/probe")
class ExitProbeController {
    func ExitProbeController() {
        throw RuntimeError("help must exit before constructing controllers");
    }

    @Get("")
    func probe(): string { return "unreachable"; }
}

export func main(list<string> args): int {
    gebweb.cli(gebweb.app(), {"name": "exit-probe"});
    return 0;
}
GB
for runtime in vm evaluator; do
    lane=()
    if [[ "$runtime" == evaluator ]]; then lane+=(--disable-vm); fi
    env -u DATABASE_URL -u SESSION_DIR -u CSRF_SECRET \
        "$geblang_bin" "${lane[@]}" -m main --help >"$scratch/help"
    grep -q 'bookmarks server options' "$scratch/help"
    "$geblang_bin" "${lane[@]}" -m cli_exit_probe --help >"$scratch/help"
    grep -q 'exit-probe server options' "$scratch/help"
done
rm "$project/src/cli_exit_probe.gb"
"$geblang_bin" check --strict src/
"$geblang_bin" test src/
"$geblang_bin" test --runtime=evaluator src/

start_server "$project" "$geblang_bin" -m main
token=$(sed -n 's/.*name="_csrf" value="\([^"]*\)".*/\1/p' "$scratch/page")
test -n "$token"
request 403 --data-urlencode url=https://geblang.com --data-urlencode title=Rejected "$base/bookmarks"
request 303 --data-urlencode "_csrf=$token" --data-urlencode url=https://geblang.com \
    --data-urlencode title=Saved "$base/bookmarks"
request 200 "$base/"
grep -q 'Bookmark saved\.' "$scratch/body"
grep -q '>Saved</a>' "$scratch/body"
request 200 "$base/"
if grep -q 'Bookmark saved\.' "$scratch/body"; then exit 1; fi
request 303 --header 'Accept: text/html' --header 'Referer: /' \
    --data-urlencode "_csrf=$token" --data-urlencode url=invalid --data-urlencode title=Retained "$base/bookmarks"
request 200 "$base/"
grep -q 'value="invalid"' "$scratch/body"
grep -q 'value="Retained"' "$scratch/body"
grep -q 'must be a valid http(s) URL' "$scratch/body"
stop_server
find "$project/.geblang-cache" -type f -name '*.gbc' -printf '%P %T@\n' | sort >"$scratch/cache-before"
test -s "$scratch/cache-before"

start_server "$project" "$geblang_bin" -m main
grep -q '>Saved</a>' "$scratch/page"
stop_server
find "$project/.geblang-cache" -type f -name '*.gbc' -printf '%P %T@\n' | sort >"$scratch/cache-after"
cmp "$scratch/cache-before" "$scratch/cache-after"

start_server "$project" "$geblang_bin" --disable-vm -m main
grep -q '>Saved</a>' "$scratch/page"
stop_server

"$gebweb_bin" build --geblang "$geblang_bin" --no-swagger --out "$scratch/bookmarks"
mkdir "$scratch/isolated"
start_server "$scratch/isolated" "$scratch/bookmarks"
grep -q '>Saved</a>' "$scratch/page"
request 200 "$base/api/bookmarks/"
grep -q '"title":"Saved"' "$scratch/body"
stop_server

echo 'Bookmarks: source, warm cache, evaluator and standalone HTTP checks passed.'
