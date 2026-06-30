#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
GEBLANG="${GEBLANG:-$ROOT/geblang}"
REPEATS="${REPEATS:-1}"
OUTPUT_DIR="${OUTPUT_DIR:-}"

cd "$ROOT/gebweb"

"$GEBLANG" check benchmarks/render_matrix.gb

echo "timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "commit=$(git -C "$ROOT" rev-parse --short HEAD)"
echo "kernel=$(uname -srmo)"
echo "go=$(go version)"
echo "geblang=$("$GEBLANG" --version)"
echo "cpu_count=$(getconf _NPROCESSORS_ONLN)"
echo "repeats=$REPEATS"

if [[ -n "$OUTPUT_DIR" ]]; then
    mkdir -p "$OUTPUT_DIR"
fi

for backend in vm evaluator; do
    for ((run = 1; run <= REPEATS; run++)); do
        echo "backend=$backend run=$run"
        command=("$GEBLANG" run benchmarks/render_matrix.gb)
        if [[ "$backend" == "evaluator" ]]; then
            command=("$GEBLANG" run --disable-vm benchmarks/render_matrix.gb)
        fi
        if [[ -n "$OUTPUT_DIR" ]]; then
            "${command[@]}" | tee "$OUTPUT_DIR/$backend-$run.jsonl"
        else
            "${command[@]}"
        fi
    done
done
