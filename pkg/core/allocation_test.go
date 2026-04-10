package core

import (
	"testing"

	"github.com/llm-inferno/optimizer-light/pkg/config"
)

// newTestSystem builds the minimal System for allocation tests:
// one accelerator (H100), one model (m1) with given perfParms,
// one service class (Premium) with ITL/TTFT targets for m1,
// one server (s1) with the given load.
func newTestSystem(perfParms config.PerfParms, arrivalRate float32, minReplicas int) {
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
			{Name: "m1", Acc: "H100", AccCount: 1, MaxBatchSize: 16, AtTokens: 512,
				PerfParms: perfParms},
		},
	})
	sys.SetServiceClassesFromSpec(&config.ServiceClassData{
		Spec: []config.ServiceClassSpec{
			{Name: "Premium", Priority: 1, ModelTargets: []config.ModelTarget{
				{Model: "m1", SLO_ITL: 100, SLO_TTFT: 2000},
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

// Zero perfParms + non-zero load: CreateAllocation must return nil (guard fires).
func TestCreateAllocation_ZeroPerfParms_NonZeroLoad_ReturnsNil(t *testing.T) {
	newTestSystem(config.PerfParms{Alpha: 0, Beta: 0, Gamma: 0}, 60, 1)
	alloc := CreateAllocation("s1", "H100")
	if alloc != nil {
		t.Errorf("expected nil allocation for zero perfParms with non-zero load, got %v", alloc)
	}
}

// Zero perfParms + zero load: CreateAllocation must return non-nil (zeroLoadAllocation path,
// perfParms not needed).
func TestCreateAllocation_ZeroPerfParms_ZeroLoad_ReturnsNonNil(t *testing.T) {
	newTestSystem(config.PerfParms{Alpha: 0, Beta: 0, Gamma: 0}, 0, 0)
	alloc := CreateAllocation("s1", "H100")
	if alloc == nil {
		t.Error("expected non-nil allocation for zero load even with zero perfParms")
	}
}

// Zero perfParms + zero load + minReplicas > 0: zeroLoadAllocation must not produce +Inf
// MaxArrvRatePerReplica (division-by-zero when maxServTime == 0).
func TestCreateAllocation_ZeroPerfParms_ZeroLoad_NonZeroMinReplicas_NoInf(t *testing.T) {
	newTestSystem(config.PerfParms{Alpha: 0, Beta: 0, Gamma: 0}, 0, 2)
	alloc := CreateAllocation("s1", "H100")
	if alloc == nil {
		t.Fatal("expected non-nil allocation for zero load with minReplicas=2")
	}
	if alloc.MaxArrvRatePerReplica() != 0 {
		t.Errorf("expected MaxArrvRatePerReplica=0 for zero perfParms, got %v", alloc.MaxArrvRatePerReplica())
	}
}
