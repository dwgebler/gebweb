#!/usr/bin/env bash
set -euo pipefail

tests_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
framework=$(CDPATH= cd -- "$tests_dir/.." && pwd)
geblang_bin=$(command -v "${GEBLANG_BIN:-geblang}")
geblang_bin="$(CDPATH= cd -- "$(dirname -- "$geblang_bin")" && pwd)/$(basename -- "$geblang_bin")"
scratch=$(mktemp -d /tmp/gebweb-response-schemas.XXXXXX)
trap 'rm -rf -- "$scratch"' EXIT
mkdir "$scratch/src" "$scratch/isolated"
ln -s "$framework" "$scratch/framework"
for name in openapi_response_schema_probe openapi_response_schema_donor openapi_donor_a openapi_donor_b; do
    cp "$tests_dir/$name.gb" "$scratch/src/"
done
cat >"$scratch/geblang.yaml" <<'YAML'
name: response_schema_probe
version: 0.0.0
source: src
dependencies:
  gebweb:
    path: framework
YAML

cd "$scratch"
"$geblang_bin" check --strict src/
run_probe() {
    local output
    output=$("$@")
    if [[ "$output" != response-schemas-ok ]]; then
        printf 'Unexpected output: %s\n' "$output" >&2
        exit 1
    fi
}
run_probe "$geblang_bin" -m openapi_response_schema_probe
find .geblang-cache -type f -name '*.gbc' -printf '%P %T@\n' | sort >cache-before
test -s cache-before
run_probe "$geblang_bin" -m openapi_response_schema_probe
find .geblang-cache -type f -name '*.gbc' -printf '%P %T@\n' | sort >cache-after
cmp cache-before cache-after
run_probe "$geblang_bin" --disable-vm -m openapi_response_schema_probe
"$geblang_bin" build --entry openapi_response_schema_probe --out "$scratch/response-schemas" .
cd "$scratch/isolated"
run_probe "$scratch/response-schemas"
echo 'Response schemas: VM, unchanged warm cache, evaluator and standalone bundle passed.'
