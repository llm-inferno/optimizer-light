# Optimal-Concurrency Batch-Size Selection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Optimizer's linear-in-tokens max-batch-size heuristic with the queue analyzer's `OptimalConcurrency` search (queue-analysis PR #19), using `perf.MaxBatchSize` as the per-(model,accelerator) search ceiling.

**Architecture:** In `CreateAllocation`, when there is no explicit `server.maxBatchSize` override, build a queue analyzer at a generous ceiling and call `OptimalConcurrency(targetPerf)` to obtain the minimum concurrency `M*` reaching near-peak throughput under the SLO; use `M*` as the max batch size. Infeasible SLO ⇒ skip the candidate. The `perf.AtTokens` field and the linear formula are retired.

**Tech Stack:** Go 1.24, `github.com/llm-inferno/queue-analysis` (PR #19, via local `replace`), Go's built-in `testing`.

**Spec:** `docs/superpowers/specs/2026-06-15-optimal-concurrency-batch-size-design.md`

---

## File Structure

- **Modify** `go.mod` — add `replace` directive to the local `queue-analysis` checkout.
- **Modify** `pkg/config/defaults.go` — add `DefaultConcurrencyCeiling` constant.
- **Modify** `pkg/core/allocation.go` — replace the `N` computation in `CreateAllocation` with the override-or-search logic.
- **Modify** `pkg/config/types.go` — re-comment `MaxBatchSize`; remove `AtTokens`.
- **Modify** `pkg/core/allocation_test.go` — parameterize the test helper; add feasible/infeasible search tests; drop `AtTokens`.
- **Modify** `pkg/solver/solver_test.go` — update the duplicate helper; drop `AtTokens`.
- **Modify** `demos/generators/main.go` — emit a generous ceiling; drop `AtTokens` and the now-unused benchmark matrix.
- **Modify** `sample-data/{small,large}/model-data.json`, `sample-data/small/system-data.json` — drop `atTokens`, set generous `maxBatchSize`.
- **Modify** `rest-server/README.md` — documentation only (payloads + field descriptions).

---

## Task 1: Wire up the dependency and add the ceiling constant

**Files:**
- Modify: `go.mod`
- Modify: `pkg/config/defaults.go`

- [ ] **Step 1: Add the `replace` directive to `go.mod`**

Append to the end of `go.mod`:

```
replace github.com/llm-inferno/queue-analysis => ../queue-analysis
```

- [ ] **Step 2: Tidy modules**

Run: `go mod tidy`
Expected: completes without error; `go.mod` keeps the `require github.com/llm-inferno/queue-analysis` line (the `replace` redirects resolution to the local checkout containing PR #19).

- [ ] **Step 3: Verify the new symbol resolves**

Run: `go doc github.com/llm-inferno/queue-analysis/pkg/analyzer.LLMQueueAnalyzer.OptimalConcurrency`
Expected: prints the method signature `func (qa *LLMQueueAnalyzer) OptimalConcurrency(target *TargetPerf) (*ConcurrencyResult, error)`. If it errors with "no such symbol", the `replace` path or the local checkout is wrong — stop and fix before continuing.

- [ ] **Step 4: Add `DefaultConcurrencyCeiling` to `pkg/config/defaults.go`**

Insert after line 18 (the `DefaultMaxNumTokens` constant):

```go
// default upper bound (search ceiling) for the optimal-concurrency search;
// used when a (model,accelerator)'s perf.MaxBatchSize is 0/unset
const DefaultConcurrencyCeiling = 256
```

- [ ] **Step 5: Verify the build**

Run: `go build ./...`
Expected: builds cleanly (no code uses the new symbol or constant yet).

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum pkg/config/defaults.go
git commit -m "build: vendor queue-analysis PR #19 via replace; add DefaultConcurrencyCeiling

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: Switch `CreateAllocation` to the optimal-concurrency search

**Files:**
- Modify: `pkg/core/allocation_test.go`
- Modify: `pkg/core/allocation.go:84-134`

- [ ] **Step 1: Parameterize the test helper (no behavior change)**

In `pkg/core/allocation_test.go`, change the helper signature to accept the per-(model,accelerator) ceiling, and use it in the model spec. Replace the signature line and the `SetModelsFromSpec` block:

```go
func newTestSystem(perfParms config.PerfParms, maxBatchSize int, arrivalRate float32, minReplicas int) {
```

and

```go
	sys.SetModelsFromSpec(&config.ModelData{
		PerfData: []config.ModelAcceleratorPerfData{
			{Name: "m1", Acc: "H100", AccCount: 1, MaxBatchSize: maxBatchSize, AtTokens: 512,
				PerfParms: perfParms},
		},
	})
```

Update the three existing callers to pass a generous ceiling (`256`):

- `TestCreateAllocation_ZeroPerfParms_NonZeroLoad_ReturnsNil`: `newTestSystem(config.PerfParms{Alpha: 0, Beta: 0, Gamma: 0}, 256, 60, 1)`
- `TestCreateAllocation_ZeroPerfParms_ZeroLoad_ReturnsNonNil`: `newTestSystem(config.PerfParms{Alpha: 0, Beta: 0, Gamma: 0}, 256, 0, 0)`
- `TestCreateAllocation_ZeroPerfParms_ZeroLoad_NonZeroMinReplicas_NoInf`: `newTestSystem(config.PerfParms{Alpha: 0, Beta: 0, Gamma: 0}, 256, 0, 2)`

- [ ] **Step 2: Run the existing tests to confirm the refactor is green**

Run: `go test ./pkg/core/ -run TestCreateAllocation -v`
Expected: the three existing tests PASS (helper refactor changed no behavior).

- [ ] **Step 3: Write the failing red-green test for the searched concurrency**

Append to `pkg/core/allocation_test.go`:

```go
// No benchmark (perf.MaxBatchSize == 0) + valid perfParms + lenient SLOs:
// the analyzer's optimal-concurrency search must pick a batch size > 1.
// The OLD linear formula computed N = max(0*AtTokens/K, 1) = 1, so this
// asserts the new search path is in effect.
func TestCreateAllocation_NoBenchmark_UsesSearchedConcurrency(t *testing.T) {
	newTestSystem(config.PerfParms{Alpha: 1.5, Beta: 0.002, Gamma: 0.0001}, 0, 60, 1)
	alloc := CreateAllocation("s1", "H100")
	if alloc == nil {
		t.Fatal("expected a feasible allocation with valid perfParms and lenient SLOs")
	}
	if alloc.MaxBatchSize() <= 1 {
		t.Errorf("expected searched concurrency > 1, got %d", alloc.MaxBatchSize())
	}
	if alloc.MaxBatchSize() > config.DefaultConcurrencyCeiling {
		t.Errorf("batch size %d exceeds search ceiling %d", alloc.MaxBatchSize(), config.DefaultConcurrencyCeiling)
	}
	if alloc.NumReplicas() < 1 {
		t.Errorf("expected at least 1 replica, got %d", alloc.NumReplicas())
	}
}

// Infeasible SLO (ITL target below what any batch size can achieve):
// CreateAllocation must return nil (skip), never an allocation pinned to
// the search's MMin=1 fallback.
func TestCreateAllocation_InfeasibleSLO_ReturnsNil(t *testing.T) {
	newTestSystemWithTargets(config.PerfParms{Alpha: 50, Beta: 1, Gamma: 0.1}, 256, 60, 1,
		0.001 /*SLO_ITL*/, 0.001 /*SLO_TTFT*/)
	alloc := CreateAllocation("s1", "H100")
	if alloc != nil {
		t.Errorf("expected nil for an unachievable SLO, got %v", alloc)
	}
}
```

Add a targets-aware helper variant just below `newTestSystem` (the original delegates to it so existing behavior is unchanged):

```go
// newTestSystemWithTargets is newTestSystem with explicit ITL/TTFT SLO targets.
func newTestSystemWithTargets(perfParms config.PerfParms, maxBatchSize int, arrivalRate float32, minReplicas int, sloITL, sloTTFT float32) {
	sys := NewSystem()
	TheSystem = sys

	sys.SetAcceleratorsFromSpec(&config.AcceleratorData{
		Spec: []config.AcceleratorSpec{
			{Name: "H100", Type: "H100", Multiplicity: 1, Cost: 75},
		},
	})
	sys.SetCapacityFromSpec(&config.CapacityData{
		Count: []config.AcceleratorCount{{Type: "H100", Count: 8}},
	})
	sys.SetModelsFromSpec(&config.ModelData{
		PerfData: []config.ModelAcceleratorPerfData{
			{Name: "m1", Acc: "H100", AccCount: 1, MaxBatchSize: maxBatchSize, AtTokens: 512,
				PerfParms: perfParms},
		},
	})
	sys.SetServiceClassesFromSpec(&config.ServiceClassData{
		Spec: []config.ServiceClassSpec{
			{Name: "Premium", Priority: 1, ModelTargets: []config.ModelTarget{
				{Model: "m1", SLO_ITL: sloITL, SLO_TTFT: sloTTFT},
			}},
		},
	})
	sys.SetServersFromSpec(&config.ServerData{
		Spec: []config.ServerSpec{
			{Name: "s1", Class: "Premium", Model: "m1",
				MinNumReplicas: minReplicas,
				CurrentAlloc: config.AllocationData{
					Load: config.ServerLoadSpec{
						ArrivalRate:  arrivalRate,
						AvgInTokens:  512,
						AvgOutTokens: 512,
					},
				},
			},
		},
	})
}
```

Then replace the body of the original `newTestSystem` so it delegates (keeps its lenient `ITL: 100, TTFT: 2000` targets):

```go
func newTestSystem(perfParms config.PerfParms, maxBatchSize int, arrivalRate float32, minReplicas int) {
	newTestSystemWithTargets(perfParms, maxBatchSize, arrivalRate, minReplicas, 100, 2000)
}
```

- [ ] **Step 4: Run the new test against the OLD implementation to confirm red**

Run: `go test ./pkg/core/ -run TestCreateAllocation_NoBenchmark_UsesSearchedConcurrency -v`
Expected: FAIL — old code computes `N = max(perf.MaxBatchSize*perf.AtTokens/K, 1) = max(0,1) = 1`, so `MaxBatchSize() == 1` trips the `expected searched concurrency > 1` assertion.

- [ ] **Step 5: Replace the `N` computation in `CreateAllocation`**

In `pkg/core/allocation.go`, replace lines 84–134 (from the `// calculate max batch size (N) ...` comment through the `rateStar := metrics.Throughput` line) with:

```go
	// average request output length
	K := load.AvgOutTokens
	maxQueue := server.maxQueueSize

	serviceParms := &analyzer.ServiceParms{
		Alpha: perf.PerfParms.Alpha,
		Beta:  perf.PerfParms.Beta,
		Gamma: perf.PerfParms.Gamma,
	}
	requestData := &analyzer.RequestSize{
		AvgInputTokens:  float32(load.AvgInTokens),
		AvgOutputTokens: float32(K),
	}
	targetPerf := &analyzer.TargetPerf{
		TargetTTFT: target.TTFT,
		TargetITL:  target.ITL,
		TargetTPS:  target.TPS,
	}

	// determine max batch size (concurrency) N
	var N int
	if server.maxBatchSize > 0 {
		// explicit override: use as-is, skip the search
		N = server.maxBatchSize
	} else {
		// primary path: let the queue analyzer find the optimal concurrency
		// (smallest max batch size reaching near-peak throughput under the SLO).
		// perf.MaxBatchSize is the search ceiling; 0 => default ceiling.
		ceiling := perf.MaxBatchSize
		if ceiling <= 0 {
			ceiling = config.DefaultConcurrencyCeiling
		}
		searchAnalyzer, err := analyzer.NewLLMQueueAnalyzer(&analyzer.Configuration{
			MaxBatchSize: ceiling,
			MaxNumTokens: config.DefaultMaxNumTokens,
			MaxQueueSize: maxQueue,
			ServiceParms: serviceParms,
		}, requestData)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		res, err := searchAnalyzer.OptimalConcurrency(targetPerf)
		if err != nil || res == nil || !res.Feasible {
			// SLO unachievable within [1, ceiling]; skip this candidate.
			return nil
		}
		N = res.Concurrency
	}

	// create queue analyzer at the chosen concurrency N
	qConfig := &analyzer.Configuration{
		MaxBatchSize: N,
		MaxNumTokens: config.DefaultMaxNumTokens,
		MaxQueueSize: maxQueue,
		ServiceParms: serviceParms,
	}

	queueAnalyzer, err := analyzer.NewLLMQueueAnalyzer(qConfig, requestData)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	// determine max rates to satisfy targets
	_, metrics, _, err := queueAnalyzer.Size(targetPerf)
	if err != nil {
		return nil
	}
	rateStar := metrics.Throughput
```

Note: this deletes the old standalone `targetPerf` block (and its `// TODO: do we need this?` comment) that previously sat between `NewLLMQueueAnalyzer` and `Size()`, since `targetPerf` is now built earlier. The code below `rateStar := metrics.Throughput` (replicas, cost, `Analyze`, struct construction) is unchanged.

- [ ] **Step 6: Run the core tests to verify green**

Run: `go test ./pkg/core/ -run TestCreateAllocation -v`
Expected: all five tests PASS (three originals + `NoBenchmark_UsesSearchedConcurrency` + `InfeasibleSLO_ReturnsNil`).

- [ ] **Step 7: Commit**

```bash
git add pkg/core/allocation.go pkg/core/allocation_test.go
git commit -m "feat: choose max batch size via queue-analyzer OptimalConcurrency

Replace the linear-in-tokens heuristic with the optimal-concurrency
search. perf.MaxBatchSize is now the search ceiling (default 256);
explicit server.maxBatchSize still overrides. Infeasible SLO => skip.

Refs #13

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: Retire the `AtTokens` field

**Files:**
- Modify: `pkg/config/types.go:68-69`
- Modify: `pkg/core/allocation_test.go`
- Modify: `pkg/solver/solver_test.go`
- Modify: `demos/generators/main.go`

- [ ] **Step 1: Update `pkg/config/types.go`**

Replace the two lines (68–69):

```go
	MaxBatchSize int       `json:"maxBatchSize"` // max batch size based on average number of tokens per request
	AtTokens     int       `json:"atTokens"`     // average number of tokens per request assumed in max batch size calculation
```

with:

```go
	MaxBatchSize int       `json:"maxBatchSize"` // search ceiling (max concurrency) for the optimal-concurrency search; 0 => DefaultConcurrencyCeiling (256)
```

- [ ] **Step 2: Remove `AtTokens` from the core test helpers**

In `pkg/core/allocation_test.go`, both `SetModelsFromSpec` blocks (in `newTestSystem`'s delegate target `newTestSystemWithTargets` — there is now one such block) — change:

```go
			{Name: "m1", Acc: "H100", AccCount: 1, MaxBatchSize: maxBatchSize, AtTokens: 512,
				PerfParms: perfParms},
```

to:

```go
			{Name: "m1", Acc: "H100", AccCount: 1, MaxBatchSize: maxBatchSize,
				PerfParms: perfParms},
```

- [ ] **Step 3: Update the solver test helper**

In `pkg/solver/solver_test.go`, change the helper signature and model spec to match the core helper. Replace the signature:

```go
func newSolverTestSystem(perfParms config.PerfParms, maxBatchSize int, arrivalRate float32) {
```

Replace the model spec block:

```go
	sys.SetModelsFromSpec(&config.ModelData{
		PerfData: []config.ModelAcceleratorPerfData{
			{Name: "m1", Acc: "H100", AccCount: 1, MaxBatchSize: maxBatchSize,
				PerfParms: perfParms},
		},
	})
```

Update the three callers to pass a generous ceiling (`256`):

- `TestSolve_ZeroPerfParms_ReturnsError`: `newSolverTestSystem(config.PerfParms{Alpha: 0, Beta: 0, Gamma: 0}, 256, 60)`
- `TestSolve_ValidPerfParms_ReturnsNil`: `newSolverTestSystem(config.PerfParms{Alpha: 1.5, Beta: 0.002, Gamma: 0.0001}, 256, 60)`
- `TestSolveGreedy_ZeroPerfParms_ReturnsError`: `newSolverTestSystem(config.PerfParms{Alpha: 0, Beta: 0, Gamma: 0}, 256, 60)`

- [ ] **Step 4: Update `demos/generators/main.go`**

Change the perf-data construction (lines ~156–157) from:

```go
				MaxBatchSize: maxBatchSize[j][i] * batchSizeFactor,
				AtTokens:     atTokens,
```

to:

```go
				MaxBatchSize: config.DefaultConcurrencyCeiling,
```

Then remove the now-unused declarations so the file compiles (Go errors on unused locals):
- the entire `maxBatchSize := [][]int{ ... }` literal (lines ~74–96),
- `atTokens := 512` (line ~122),
- `batchSizeFactor := 2` (line ~124),
- the `maxBatchSize = MaskMatrix(maxBatchSize, accMask, modMask)` line inside the `useMask` block (line ~139).

- [ ] **Step 5: Verify build, vet, and full test suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all green. The solver `TestSolve_ValidPerfParms_ReturnsNil` exercises the new search path end-to-end (valid perfParms, ceiling 256, lenient SLOs ⇒ feasible).

- [ ] **Step 6: Commit**

```bash
git add pkg/config/types.go pkg/core/allocation_test.go pkg/solver/solver_test.go demos/generators/main.go
git commit -m "refactor: retire perf.AtTokens; perf.MaxBatchSize is the search ceiling

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 4: Update sample data and REST documentation

**Files:**
- Modify: `sample-data/small/model-data.json`, `sample-data/large/model-data.json`, `sample-data/small/system-data.json`
- Modify: `rest-server/README.md`

- [ ] **Step 1: Rewrite the sample-data perf entries**

Drop every `atTokens` key and set every perf-entry `maxBatchSize` to a generous ceiling (`256`) so the demos exercise a real search rather than pinning to a tight benchmarked value. Run:

```bash
tmp=$(mktemp)
for f in sample-data/small/model-data.json sample-data/large/model-data.json; do
  jq '(.models[]) |= (del(.atTokens) | .maxBatchSize = 256)' "$f" > "$tmp" && mv "$tmp" "$f"
done
jq '(.system.models[]) |= (del(.atTokens) | .maxBatchSize = 256)' sample-data/small/system-data.json > "$tmp" && mv "$tmp" sample-data/small/system-data.json
```

- [ ] **Step 2: Confirm no `atTokens` remains in sample-data**

Run: `grep -rn "atTokens" sample-data/ || echo "clean"`
Expected: `clean`.

- [ ] **Step 3: Run the demo end-to-end**

Run: `cd demos/main && go run main.go large; cd ../..`
Expected: completes without error and prints a solution (per-server accelerator assignments, replicas, batch sizes). Batch sizes now reflect searched `M*` values, not the old benchmarked numbers.

- [ ] **Step 4: Update `rest-server/README.md` — example payloads**

In the model-data example block (lines ~74–113), remove all three `"atTokens": 512,` lines, and change the three `"maxBatchSize"` example values to `256` (i.e. `"maxBatchSize": 256,` for the `A100`, `G2`, and `llama_70b`/`G2` entries).

- [ ] **Step 5: Update `rest-server/README.md` — field descriptions**

Replace lines 119–120:

```
   - `maxBatchSize`: maximum batch size to use, beyond which performance deteriorates
   - `atTokens`: average number of tokens used when determining the `maxBatchSize`
```

with:

```
   - `maxBatchSize`: search ceiling (max concurrency) for the optimal-concurrency search that sizes each server; `0` uses the default ceiling (256). An explicit per-server `maxBatchSize` override (in server data) bypasses the search.
```

(Leave the `accCount` and `perfParms` bullets unchanged.)

- [ ] **Step 6: Commit**

```bash
git add sample-data/small/model-data.json sample-data/large/model-data.json sample-data/small/system-data.json rest-server/README.md
git commit -m "docs+data: drop atTokens, use generous maxBatchSize ceiling in samples and README

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 5: Final verification and pull request

**Files:** none (verification + PR)

- [ ] **Step 1: Full verification sweep**

Run: `gofmt -l . && go build ./... && go vet ./... && go test ./...`
Expected: `gofmt -l .` prints nothing (all formatted); build, vet, and tests all green.

- [ ] **Step 2: Confirm no stray references remain**

Run: `grep -rn "AtTokens\|atTokens" --include="*.go" . ; grep -rn "MaxBatchSize\*" --include="*.go" .`
Expected: no matches (the field and the linear formula are fully removed).

- [ ] **Step 3: Push and open the PR**

```bash
git push
gh pr create --fill --base main \
  --title "Use queue-analyzer OptimalConcurrency to choose max batch size" \
  --body "$(cat <<'EOF'
Closes #13.

Replaces the linear-in-tokens max-batch-size heuristic with the queue analyzer's optimal-concurrency search (queue-analysis PR #19).

- `perf.MaxBatchSize` is now the per-(model,accelerator) search ceiling (default 256); `perf.AtTokens` is retired.
- Explicit `server.maxBatchSize` still overrides and skips the search.
- Infeasible SLO ⇒ candidate is skipped.
- Interim dependency via `replace => ../queue-analysis` until queue-analysis is tagged past v0.7.0.

Design: `docs/superpowers/specs/2026-06-15-optimal-concurrency-batch-size-design.md`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Note: the `replace` directive points at a local sibling checkout and must not be merged as-is for consumers. Before merge, tag `queue-analysis` (e.g. `v0.8.0`), then swap the `replace` for a real `require` bump (`go get github.com/llm-inferno/queue-analysis@v0.8.0 && go mod tidy`). Call this out in the PR description / review.

---

## Self-Review

**Spec coverage:**
- Primary-path control flow → Task 2 (Step 5). ✓
- `DefaultConcurrencyCeiling` → Task 1 (Step 4). ✓
- Reuse `perf.MaxBatchSize` as ceiling + retire `AtTokens` → Task 2 (logic) + Task 3 (field). ✓
- Infeasible ⇒ nil → Task 2 (Step 3 test + Step 5 logic). ✓
- All-zero perfParms guard preserved → unchanged (above the replaced block); covered by existing tests carried in Task 2. ✓
- Redundant-solve-accepted-for-simplicity → Task 2 Step 5 keeps the downstream `Size()`. ✓
- Dependency `replace` → Task 1 + Task 5 merge note. ✓
- `demos/generators`, `sample-data`, `rest-server/README.md` → Task 3 (generators) + Task 4. ✓
- `zeroLoadAllocation` left as-is → not modified by any task (intentional). ✓
- Tests in `pkg/core` + `pkg/solver` → Task 2 + Task 3. ✓

**Placeholder scan:** No TBD/TODO/"handle edge cases"; every code step shows full code. ✓

**Type consistency:** `OptimalConcurrency(target *TargetPerf) (*ConcurrencyResult, error)`, `res.Concurrency`, `res.Feasible`, `config.DefaultConcurrencyCeiling`, helper signatures `newTestSystem(perfParms, maxBatchSize, arrivalRate, minReplicas)` / `newSolverTestSystem(perfParms, maxBatchSize, arrivalRate)` are used consistently across tasks. ✓
