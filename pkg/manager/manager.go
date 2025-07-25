package manager

import (
	"github.com/llm-inferno/optimizer-light/pkg/core"
	"github.com/llm-inferno/optimizer-light/pkg/solver"
)

type Manager struct {
	system    *core.System
	optimizer *solver.Optimizer
}

func NewManager(system *core.System, optimizer *solver.Optimizer) *Manager {
	core.TheSystem = system
	return &Manager{
		system:    system,
		optimizer: optimizer,
	}
}

func (m *Manager) Optimize() error {
	if err := m.optimizer.Optimize(); err != nil {
		return err
	}
	m.system.AllocateByType()
	return nil
}
