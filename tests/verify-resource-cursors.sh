#!/usr/bin/env bash
set -euo pipefail

tests_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
framework=$(CDPATH= cd -- "$tests_dir/.." && pwd)
geblang_bin=$(command -v "${GEBLANG_BIN:-geblang}")
geblang_bin="$(CDPATH= cd -- "$(dirname -- "$geblang_bin")" && pwd)/$(basename -- "$geblang_bin")"
scratch=$(mktemp -d /tmp/gebweb-resource-cursors.XXXXXX)
trap 'rm -rf -- "$scratch"' EXIT
mkdir "$scratch/src" "$scratch/isolated"
ln -s "$framework" "$scratch/framework"
for name in resource_cursor_probe resource_cursor_donor; do
    cp "$tests_dir/$name.gb" "$scratch/src/"
done
cat >"$scratch/geblang.yaml" <<'YAML'
name: resource_cursor_acceptance
version: 0.0.0
source: src
dependencies:
  gebweb:
    path: framework
YAML

cp "$framework/examples/cursor_resources.gb" "$scratch/src/"

cd "$scratch"
"$geblang_bin" check --strict src/
run_probe() {
    local output
    output=$("$@")
    if [[ "$output" != resource-cursors-ok ]]; then
        printf 'Unexpected output: %s\n' "$output" >&2
        exit 1
    fi
}
run_probe "$geblang_bin" -m resource_cursor_probe
find .geblang-cache -type f -name '*.gbc' -printf '%P %T@\n' | sort >cache-before
test -s cache-before
run_probe "$geblang_bin" -m resource_cursor_probe
find .geblang-cache -type f -name '*.gbc' -printf '%P %T@\n' | sort >cache-after
cmp cache-before cache-after
run_probe "$geblang_bin" --disable-vm -m resource_cursor_probe
"$geblang_bin" build --entry resource_cursor_probe --out "$scratch/resource-cursors" .

"$geblang_bin" -m cursor_resources >example-vm.json
"$geblang_bin" --disable-vm -m cursor_resources >example-evaluator.json
cmp example-vm.json example-evaluator.json
cat >example-expected.json <<'JSON'
{"items":[{"id":1,"name":"item-1"},{"id":2,"name":"item-2"}],"nextCursor":"Mg=="}
{"items":[{"id":3,"name":"item-3"}],"nextCursor":null}
JSON
cmp example-vm.json example-expected.json

mv "$scratch/src" "$scratch/unused-source"
mv "$scratch/framework" "$scratch/unused-framework"
mv "$scratch/geblang.yaml" "$scratch/unused-manifest"
cd "$scratch/isolated"
run_probe "$scratch/resource-cursors"
echo 'Resource cursors: VM, unchanged warm cache, evaluator and standalone bundle passed.'
