package solver_test

import (
	"testing"

	"github.com/llm-inferno/optimizer-light/pkg/config"
	"github.com/llm-inferno/optimizer-light/pkg/core"
	"github.com/llm-inferno/optimizer-light/pkg/solver"
)

// newSolverTestSystem builds a system identical to Task 1's newTestSystem,
// but exposed here since solver_test is an external test package.
func newSolverTestSystem(perfParms config.PerfParms, arrivalRate float32) {
	sys := core.NewSystem()
	core.TheSystem = sys

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
				MinNumReplicas: 1,
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
	sys.Calculate()
}

// When a server has no valid allocations (zero perfParms + non-zero load),
// Solve() must return a non-nil error naming the unresolved server.
func TestSolve_ZeroPerfParms_ReturnsError(t *testing.T) {
	newSolverTestSystem(config.PerfParms{Alpha: 0, Beta: 0, Gamma: 0}, 60)
	s := solver.NewSolver(&config.OptimizerSpec{Unlimited: true})
	err := s.Solve()
	if err == nil {
		t.Error("expected error when server has no feasible allocation, got nil")
	}
}

// When a server has valid perfParms and load, Solve() must return nil (success).
func TestSolve_ValidPerfParms_ReturnsNil(t *testing.T) {
	newSolverTestSystem(config.PerfParms{Alpha: 1.5, Beta: 0.002, Gamma: 0.0001}, 60)
	s := solver.NewSolver(&config.OptimizerSpec{Unlimited: true})
	err := s.Solve()
	if err != nil {
		t.Errorf("expected nil error for valid perfParms, got: %v", err)
	}
}

// When a server has no valid allocations and the greedy solver is used,
// Solve() must also return a non-nil error naming the unresolved server.
func TestSolveGreedy_ZeroPerfParms_ReturnsError(t *testing.T) {
	newSolverTestSystem(config.PerfParms{Alpha: 0, Beta: 0, Gamma: 0}, 60)
	s := solver.NewSolver(&config.OptimizerSpec{Unlimited: false})
	err := s.Solve()
	if err == nil {
		t.Error("expected error from greedy solver with zero perfParms, got nil")
	}
}
