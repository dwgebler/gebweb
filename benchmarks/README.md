# Gebweb rendering benchmarks

`render_matrix.gb` measures the real Gebweb `ViewEngine` through Geblang's
native profiler. It separates:

- Cold template load, parse, compile, and first render.
- Warm rendering of an already loaded `View`, without response caching.

The matrix varies static template size, interpolation count, context width,
loop size, filters, includes, and inheritance. Each output line is JSON so
results can be archived and compared mechanically.

Run both backends from the repository root:

```sh
gebweb/benchmarks/run-render-benchmarks.sh
```

Save five runs per backend for comparison:

```sh
REPEATS=5 OUTPUT_DIR=gebweb/benchmarks/results/baseline \
  gebweb/benchmarks/run-render-benchmarks.sh
```

The program asserts every cold and warm output before reporting a result.
An output regression therefore fails the benchmark instead of producing a
misleading performance improvement.

Benchmark a running website over a real socket:

```sh
gebweb/benchmarks/benchmark-website.sh http://127.0.0.1:18085
```

Set `REQUESTS` or `CONCURRENCIES` to change the default 1,000 requests at
concurrency 1, 10, 50, and 100. The script measures the first request before
warming each page and then uses HTTP keep-alive for the load matrix. Restart
the server between cold-start comparisons.

For a stable comparison:

- Use the same machine and power mode.
- Record `go version`, `geblang --version`, CPU count, and commit.
- Run each backend at least five times.
- Compare medians rather than one result.
- Keep generated output outside the repository.

The release gate additionally benchmarks the built website over a real socket;
this matrix isolates renderer work and is not an HTTP-capacity benchmark.

See `BASELINE.md` for the initial result and the performance issues it exposes.
