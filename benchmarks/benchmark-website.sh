#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BASE_URL="${1:-http://127.0.0.1:18085}"
REQUESTS="${REQUESTS:-1000}"
CONCURRENCIES="${CONCURRENCIES:-1 10 50 100}"

pages=(
    "/"
    "/features"
    "/examples"
    "/web-development"
    "/docs/classes-interfaces.html"
)

if ! command -v curl >/dev/null 2>&1; then
    echo "curl is required" >&2
    exit 1
fi
if ! command -v ab >/dev/null 2>&1; then
    echo "ApacheBench (ab) is required" >&2
    exit 1
fi

echo "timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "base_url=$BASE_URL"
echo "requests=$REQUESTS"
echo "concurrencies=$CONCURRENCIES"
echo "kernel=$(uname -srmo)"
echo "go=$(go version)"
echo "geblang=$("$ROOT/geblang" --version)"
echo "cpu_count=$(getconf _NPROCESSORS_ONLN)"

for path in "${pages[@]}"; do
    url="${BASE_URL}${path}"
    result="$(curl --fail --silent --show-error --output /dev/null \
        --write-out '%{http_code} %{size_download} %{time_starttransfer} %{time_total}' \
        "$url")"
    echo "first path=$path status_size_ttfb_total=$result"

    for concurrency in $CONCURRENCIES; do
        requests="$REQUESTS"
        if (( requests < concurrency )); then
            requests="$concurrency"
        fi
        echo "ab path=$path concurrency=$concurrency requests=$requests"
        ab -k -q -n "$requests" -c "$concurrency" "$url"
    done
done
