package core

import (
	"testing"

	"github.com/llm-inferno/optimizer-light/pkg/config"
)

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
			{Name: "m1", Acc: "H100", AccCount: 1, MaxBatchSize: maxBatchSize,
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

// newTestSystem builds the minimal System for allocation tests:
// one accelerator (H100), one model (m1) with given perfParms,
// one service class (Premium) with ITL/TTFT targets for m1,
// one server (s1) with the given load.
func newTestSystem(perfParms config.PerfParms, maxBatchSize int, arrivalRate float32, minReplicas int) {
	newTestSystemWithTargets(perfParms, maxBatchSize, arrivalRate, minReplicas, 100, 2000)
}

// Zero perfParms + non-zero load: CreateAllocation must return nil (guard fires).
func TestCreateAllocation_ZeroPerfParms_NonZeroLoad_ReturnsNil(t *testing.T) {
	newTestSystem(config.PerfParms{Alpha: 0, Beta: 0, Gamma: 0}, 256, 60, 1)
	alloc := CreateAllocation("s1", "H100")
	if alloc != nil {
		t.Errorf("expected nil allocation for zero perfParms with non-zero load, got %v", alloc)
	}
}

// Zero perfParms + zero load: CreateAllocation must return non-nil (zeroLoadAllocation path,
// perfParms not needed).
func TestCreateAllocation_ZeroPerfParms_ZeroLoad_ReturnsNonNil(t *testing.T) {
	newTestSystem(config.PerfParms{Alpha: 0, Beta: 0, Gamma: 0}, 256, 0, 0)
	alloc := CreateAllocation("s1", "H100")
	if alloc == nil {
		t.Error("expected non-nil allocation for zero load even with zero perfParms")
	}
}

// Zero perfParms + zero load + minReplicas > 0: zeroLoadAllocation must not produce +Inf
// MaxArrvRatePerReplica (division-by-zero when maxServTime == 0).
func TestCreateAllocation_ZeroPerfParms_ZeroLoad_NonZeroMinReplicas_NoInf(t *testing.T) {
	newTestSystem(config.PerfParms{Alpha: 0, Beta: 0, Gamma: 0}, 256, 0, 2)
	alloc := CreateAllocation("s1", "H100")
	if alloc == nil {
		t.Fatal("expected non-nil allocation for zero load with minReplicas=2")
	}
	if alloc.MaxArrvRatePerReplica() != 0 {
		t.Errorf("expected MaxArrvRatePerReplica=0 for zero perfParms, got %v", alloc.MaxArrvRatePerReplica())
	}
}

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

// Explicit server.maxBatchSize override: CreateAllocation must use it verbatim
// and skip the optimal-concurrency search (result is the override, not a
// searched value bounded by the perf ceiling).
func TestCreateAllocation_Override_UsesGivenBatchSize(t *testing.T) {
	newTestSystemWithTargets(config.PerfParms{Alpha: 1.5, Beta: 0.002, Gamma: 0.0001}, 256, 60, 1, 100, 2000)
	const override = 7
	GetServer("s1").maxBatchSize = override // in-package access; override the server's max batch size
	alloc := CreateAllocation("s1", "H100")
	if alloc == nil {
		t.Fatal("expected a feasible allocation with an explicit override")
	}
	if alloc.MaxBatchSize() != override {
		t.Errorf("expected batch size to equal override %d, got %d", override, alloc.MaxBatchSize())
	}
}
