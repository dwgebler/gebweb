# Template rendering baseline

Date: 2026-06-30

Commit: `01ac8b33`

Environment:

- Linux 6.6.87.2-microsoft-standard-WSL2, amd64.
- 12th Gen Intel Core i5-12450HX, 12 logical CPUs.
- Go 1.26.3.
- Geblang 1.30.1.
- Command: `gebweb/benchmarks/run-render-benchmarks.sh`.

The initial run used the bytecode VM. Times are elapsed milliseconds and
allocation figures are bytes reported by Geblang's native profiler.

| Case | Cold ms | Cold allocated | Warm ms/render | Warm bytes/render |
|---|---:|---:|---:|---:|
| Static 1 KB | 2.80 | 4,643,376 | 0.0050 | 4,112 |
| Static 10 KB | 139.72 | 420,742,768 | 0.0095 | 13,904 |
| Static 50 KB | 3,330.82 | 10,490,920,856 | 0.0377 | 62,737 |
| 10 outputs | 1.30 | 406,512 | 0.0220 | 4,184 |
| 100 outputs | 11.06 | 9,360,344 | 0.1757 | 14,504 |
| 1,000 outputs | 500.12 | 843,842,752 | 1.7452 | 122,603 |
| Context width 5 | - | - | 0.0078 | 3,496 |
| Context width 50 | - | - | 0.0329 | 13,392 |
| Context width 500 | - | - | 0.3787 | 160,431 |
| Loop 100, context 5 | - | - | 0.9877 | 446,986 |
| Loop 100, context 500 | - | - | 26.2998 | 15,851,393 |
| Loop 1,000, context 50 | - | - | 34.7588 | 11,965,105 |
| Loop 1,000, context 500* | 261.54 | 157,429,936 | 255.3681 | 157,362,064 |
| Inheritance, includes, filters | - | - | 0.5971 | 141,602 |

The evaluator produced identical output for every case. It was materially
slower on dynamic templates, including 10.5563 ms per render for 1,000 output
nodes and 90.4466 ms for the 1,000-item loop with a 50-key context.
`*` This case was added immediately before the scope-frame change at commit
`6738efa1`; the other rows are from the original `01ac8b33` baseline.

The added 1,000-item loop with a 500-key context took 527.6109 ms and
allocated 169,722,156 bytes per evaluator render.

## Findings

Cold parsing of static text grows approximately quadratically: increasing the
template from 10 KB to 50 KB multiplies input size by five, but time by about
24 and allocated bytes by about 25. This is a Gebweb template tokenizer or
parser issue until profiling demonstrates an engine cause.

This finding was resolved in 1.8.2 by a linear character-list scan plus a
static-template fast path. Across three VM runs, the 50 KB case fell from
3,330.82 ms and 10,490,920,856 allocated bytes to medians of 0.27 ms and
430,296 bytes. The 1,000-output case fell from 500.12 ms and 843,842,752 bytes
to 403.20 ms and 7,792,328 bytes. Evaluator medians were 0.41 ms and 237,584
bytes for 50 KB of static text, and 3,476.70 ms and 121,855,304 bytes for
1,000 outputs. The remaining evaluator cost is expression-heavy compilation,
not repeated static-source slicing.

Loop allocation grows with both iteration count and context width. A
100-iteration loop with 500 context keys allocates about 15.9 MB per render,
consistent with copying the complete context for every iteration.

Warm response-cache throughput remains the regression floor recorded in the
main performance plan. Renderer changes must improve these uncached results
without reducing cached website throughput below 95 percent of that baseline.
