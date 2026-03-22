# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Inferno Optimizer Light** is a Go microservice that optimizes GPU (accelerator) assignment for LLM inference servers. Given a set of models, accelerators, service-level objectives (SLOs), and request load profiles, it determines which GPU type to assign to each inference server, how many replicas to run, and what batch size to use.

The optimizer uses queueing theory (M/M/1 state-dependent) to model inference performance and a greedy algorithm to allocate constrained GPU resources.

## Commands

```bash
# Build the REST server
cd cmd/optimizer && go build main.go

# Run REST server (stateless, default)
cd cmd/optimizer && go run main.go

# Run REST server (stateful mode, persists system state between calls)
cd cmd/optimizer && go run main.go -F

# Run the main demo (uses sample-data/large/ by default)
cd demos/main && go run main.go [large|small]

# Run tests
go test ./...

# Run tests for a specific package
go test ./pkg/solver/...
go test ./pkg/analyzer/...

# Build Docker image
docker build -t inferno-optimizer-light . --load
```

## Architecture

### Data Flow

**Input → System Setup → Optimization → Solution**

1. JSON config files (or REST API body) describe: accelerators, models with per-accelerator perf data, service classes (SLOs), servers (model+load bindings), and available capacity.
2. `System` object is populated from specs via `Set*FromSpec()` methods, then `Calculate()` pre-computes model parameters.
3. `Manager.Optimize()` → `Optimizer.Optimize()` → `Solver.Solve()` runs the allocation.
4. Output is an `AllocationSolution` with per-server GPU assignments, replica counts, batch sizes, and expected latency metrics.

### Optimization Modes

- **Unlimited** (capacity planning): Assigns the cheapest accelerator that meets SLOs, ignoring capacity limits.
- **Greedy** (cluster mode): Generates candidates per server sorted by priority × delta (cost gap to next-best option), allocates greedily respecting GPU inventory.

### Key Packages

| Package | Role |
|---------|------|
| `pkg/config` | All type definitions (`types.go`), config loading, defaults |
| `pkg/core` | Domain models: `System`, `Server`, `Allocation`, `Model`, `Accelerator`, `ServiceClass` |
| `pkg/solver` | `Optimizer` → `Solver` (dispatches unlimited vs. greedy) → `Greedy` algorithm |
| `pkg/analyzer` | `QueueAnalyzer`: queueing model for sizing replicas and computing expected ITL/TTFT |
| `pkg/manager` | `Manager` coordinates System + Optimizer |
| `rest-server` | Gin HTTP handlers; `stateless.go` (only `/optimizeOne`) and `statefull.go` (full CRUD + `/optimize`) |
| `cmd/optimizer` | Entry point; `-F` flag enables stateful mode |
| `demos/` | Standalone programs for testing and demonstrating specific workflows |
| `sample-data/` | Small and large JSON test datasets (6-7 files each) |

### Core Allocation Logic (`pkg/core/allocation.go`)

For each (Server, Accelerator) candidate:
1. `QueueAnalyzer.Size(targetPerf)` finds the max request rate per replica satisfying SLO targets (ITL, TTFT).
2. Replicas = `ceil(totalRate / maxRatePerReplica)`.
3. `QueueAnalyzer.Analyze(rate)` computes expected metrics for the solution.
4. Infeasible if no batch size meets SLOs or GPU count × replicas exceeds available capacity.

### REST API Modes

**Stateless** (default): Single endpoint `/optimizeOne` accepts a full `SystemSpec` + optimizer params, returns solution. No state persists.

**Stateful** (`-F` flag): Full CRUD endpoints (`/setAccelerators`, `/setModels`, `/setServiceClasses`, `/setServers`, `/setCapacities`) build up state, then `/optimize` runs the solver. State persists across calls.

### Sample Data Format

Six JSON files per dataset in `sample-data/{small,large}/`:
- `accelerators.json` — GPU specs (type, multiplicity, cost, power)
- `models.json` — model perf data per accelerator (batch sizes, decode/prefill timing)
- `servers.json` — server definitions (model, service class, request rate, token profile)
- `serviceclasses.json` — SLO definitions (ITL, TTFT, TPS targets per model)
- `capacity.json` — available GPU counts by type
- `optimizer.json` — optimizer settings (mode, batch sizes to try)
