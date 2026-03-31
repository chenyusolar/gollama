package pool

import (
	"sync"

	"github.com/anomalyco/myllama.cpp/internal/runner"
)

type Pool struct {
	instances []*runner.LlamaInstance
	mu        sync.RWMutex
}

func New() *Pool {
	return &Pool{
		instances: make([]*runner.LlamaInstance, 0),
	}
}

func (p *Pool) Add(inst *runner.LlamaInstance) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.instances = append(p.instances, inst)
}

func (p *Pool) GetFree() *runner.LlamaInstance {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, inst := range p.instances {
		if !inst.Busy {
			inst.Busy = true
			return inst
		}
	}
	return nil
}

func (p *Pool) GetByID(id int) *runner.LlamaInstance {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, inst := range p.instances {
		if inst.ID == id {
			return inst
		}
	}
	return nil
}

func (p *Pool) Release(inst *runner.LlamaInstance) {
	if inst == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	inst.Busy = false
}

func (p *Pool) GetAll() []*runner.LlamaInstance {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]*runner.LlamaInstance, len(p.instances))
	copy(result, p.instances)
	return result
}

func (p *Pool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.instances)
}

func (p *Pool) GetStats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := PoolStats{
		Total: len(p.instances),
	}

	for _, inst := range p.instances {
		if inst.Busy {
			stats.Busy++
		} else {
			stats.Free++
		}
	}

	return stats
}

type PoolStats struct {
	Total int
	Busy  int
	Free  int
}
