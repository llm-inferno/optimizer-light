# Optimal-Concurrency Batch-Size Selection — Design

**Date:** 2026-06-15
**Status:** Approved (design); pending implementation plan

## Goal

Replace the Optimizer's prior-benchmark + linear-in-tokens heuristic for choosing
a server's max batch size (concurrency) with the queue analyzer's
optimal-concurrency search added in
[`llm-inferno/queue-analysis` PR #19](https://github.com/llm-inferno/queue-analysis/pull/19).

Given model + workload + SLO targets, the analyzer now returns the **minimum
concurrency `M*`** that reaches near-peak request throughput while still meeting
the SLO. Operating below `M*` sacrifices throughput; operating above it
over-provisions concurrency and reduces robustness to traffic surges. We want the
Optimizer to use `M*` as the max batch size for each (server, accelerator)
candidate.

## Background — current behavior

In `pkg/core/allocation.go` (`CreateAllocation`, lines ~84–93) the max batch size
`N` is chosen as:

```go
K := load.AvgOutTokens
if server.maxBatchSize > 0 {
    N = server.maxBatchSize                       // explicit override
} else {
    N = max(perf.MaxBatchSize*perf.AtTokens/K, 1) // linear-in-tokens scaling
}
```

`perf.MaxBatchSize` / `perf.AtTokens` (e.g. `18` measured at `512` tokens) come
from offline benchmarking; `N` is scaled inversely with the actual average output
length `K`. `N` then becomes `analyzer.Configuration.MaxBatchSize`, feeding
`Size()` (max sustainable rate per replica) and `Analyze()` (expected metrics).

## The new queue-analysis surface (PR #19)

Consumed in-process as a Go library (we do **not** use the `/optimize` REST
endpoint or its `OptimizeData` JSON shape):

- `func (qa *LLMQueueAnalyzer) OptimalConcurrency(target *TargetPerf) (*ConcurrencyResult, error)`
  — uses the analyzer's own `ServiceParms` / `RequestSize` and its
  **`MaxBatchSize` as the search ceiling `m_max`**, returning the smallest
  concurrency reaching near-peak throughput under the SLO.
- `ConcurrencyResult` fields used here: `Concurrency` (= `M*`), `Feasible`
  (false ⇒ SLO unreachable within `[1, ceiling]`, with `Concurrency` pinned to
  `MMin`=1). Other fields (`Metrics`, `MITL`, `MTPF`, `AnchorThroughput`,
  `Calls`, `Probes`, `Throughput`) are available but not required by this change.
- The search's closed-form brackets use `TargetITL` and `TargetTTFT`; the oracle
  (`Size()`) uses all three targets. The `TargetPerf` we already build is
  therefore sufficient.

**Critical semantic:** `MaxBatchSize` is the search *ceiling*, not a target. If
the ceiling is set near the true optimum, the search has no room to descend and
`Concurrency` simply pins to the ceiling. The ceiling must be **generous**.

## Decisions (from brainstorming)

1. **Role:** optimal-concurrency is the **primary** method — it replaces the
   linear-scaling path entirely (not a fallback, not a toggle). The explicit
   `server.maxBatchSize` override is retained.
2. **Ceiling source:** reuse the existing per-(model,accelerator)
   `perf.MaxBatchSize` field as the search ceiling, defaulting to `256` when
   `0`/unset. This is the natural granularity for a hardware/KV-cache limit and a
   ready hook for a future KV-cache-derived value. `perf.AtTokens` is retired.

## Design

### Control flow — `CreateAllocation` (`pkg/core/allocation.go`)

Two cases for `N`:

```
if server.maxBatchSize > 0:          # explicit override — unchanged
    N = server.maxBatchSize
else:                                # NEW primary path
    ceiling = perf.MaxBatchSize > 0 ? perf.MaxBatchSize : config.DefaultConcurrencyCeiling
    searchQA = NewLLMQueueAnalyzer(config @ MaxBatchSize=ceiling, requestData)
    res, err = searchQA.OptimalConcurrency(targetPerf)
    if err != nil || !res.Feasible:  # SLO unreachable in [1, ceiling]
        return nil                   # skip — joins existing "no feasible allocation" path
    N = res.Concurrency              # = M*
```

Downstream is **unchanged**: build the analyzer at `MaxBatchSize=N`, call
`Size()` → `rateStar`, compute replicas, `Analyze(perReplicaRate)` →
itl/ttft/rho.

- **Redundant solve, accepted for simplicity:** in the non-override path the
  search already sizes at `M*`, and the downstream `Size()` re-sizes at `N=M*` —
  one extra deterministic solve. We keep the downstream code identical rather
  than thread `res.Metrics` through. Easy to optimize later if it matters.
- **Minor reordering:** `maxQueue`, `requestData`, and `targetPerf` must be
  constructed *before* the search.
- **Guard preserved:** the all-zero-perfParms guard (α=β=γ=0 ⇒ return nil) stays
  before the search; all-zero parameters would make the search degenerate too.
- **Infeasible handling:** `err != nil || !res.Feasible` ⇒ `return nil`, matching
  the existing skip semantics. We do **not** silently allocate at the pinned
  `MMin`=1.

### Config / types

- `pkg/config/defaults.go`: add `const DefaultConcurrencyCeiling = 256`.
- `pkg/config/types.go` (`ModelAcceleratorPerfData`):
  - `MaxBatchSize` comment changes to: "search ceiling (max concurrency) for the
    optimal-concurrency search; 0 ⇒ `DefaultConcurrencyCeiling` (256)".
  - Remove the `AtTokens` field. Existing JSON carrying `atTokens` still parses
    (unknown keys are ignored by `encoding/json`).

### Dependency

- Add `replace github.com/llm-inferno/queue-analysis => ../queue-analysis` and run
  `go mod tidy`. PR #19 is merged on `queue-analysis/main` but the latest tag
  (`v0.7.0`) predates it, so a local `replace` is the interim mechanism until a
  new tag (e.g. `v0.8.0`) lands.

### Other touched sites

- `demos/generators/main.go`: stop emitting `atTokens`; set `MaxBatchSize` to a
  generous ceiling (e.g. `256`) instead of the benchmarked `18`, otherwise the
  search pins to the ceiling.
- `sample-data/` (`small/model-data.json`, `large/model-data.json`,
  `small/system-data.json`): remove `atTokens`; replace benchmarked `maxBatchSize`
  values with a generous ceiling so the demos exercise a real search instead of
  pinning to a tight value.
- `rest-server/README.md` (**documentation only — no Go changes in `rest-server`**;
  handlers pass `SystemSpec` through, covered by the `types.go` struct change):
  update the example payloads (drop `atTokens`, generous `maxBatchSize`) and the
  field descriptions — `maxBatchSize` becomes "search ceiling (max concurrency)
  for the optimal-concurrency search; 0 ⇒ 256", and the `atTokens` line is
  removed.
- `pkg/core/allocation_test.go`, `pkg/solver/solver_test.go`: drop `AtTokens`,
  give a generous `MaxBatchSize` ceiling plus ITL/TTFT targets, and update
  expected batch sizes / assertions.
- `zeroLoadAllocation` (`allocation.go` ~265): keeps using override-or-
  `perf.MaxBatchSize` for its cosmetic batch-size value (no traffic ⇒ nothing to
  optimize). The field there now denotes the ceiling; acceptable for the
  zero-load case.

## Testing

- `pkg/core/allocation_test.go` (white-box): zero perfParms with non-zero load
  still returns nil; zero-load still returns a valid minimum allocation; add a
  case where a non-override candidate with valid perfParms + generous ceiling +
  SLO targets yields a feasible `M*` within `[1, ceiling]`; add an infeasible-SLO
  case asserting `nil` (not a batch-size-1 allocation).
- `pkg/solver/solver_test.go` (black-box): existing zero-perfParms-errors and
  valid-perfParms-nil expectations preserved with the updated perf-data shape.
- `go test ./...`, `go vet ./...`, `go build ./...` green; `gofmt` clean.

## Downstream impact / follow-up (out of scope)

`llm-inferno/optimizer` depends on `optimizer-light` **directly** (`v0.7.8`) and
pulls `queue-analysis` only *indirectly* through it. The batch-size decision lives
entirely in optimizer-light's `CreateAllocation`, which `optimizer` reuses — it
has no linear-scaling code of its own. Therefore:

- The behavior change **propagates automatically** once `optimizer` bumps its
  `optimizer-light` dependency to the version carrying this work. No logic change
  required in `optimizer`.
- The only follow-up is mechanical: after we tag `optimizer-light`, bump the
  `optimizer` require, and give any `optimizer`-side sample data the same
  treatment (drop `atTokens`, set generous `MaxBatchSize` ceilings).

Tracked as a separate workitem.

## Out of scope (YAGNI)

- An opt-in linear-vs-optimal toggle.
- A dedicated per-server ceiling field (`perf.MaxBatchSize` is the hook).
- Computing the ceiling from KV-cache limits.
- Consuming `ConcurrencyResult` diagnostics (`MITL`/`MTPF`/`oracleCalls`) in the
  allocation output.
