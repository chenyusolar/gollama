package pool

import (
	"sync"

	"github.com/anomalyco/myllama.cpp/internal/runner"
)

type ModelPool struct {
	pools       map[string]*Pool
	mu          sync.RWMutex
	defaultPool *Pool
}

func NewModelPool() *ModelPool {
	return &ModelPool{
		pools: make(map[string]*Pool),
	}
}

func (m *ModelPool) AddModel(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.pools[name]; !ok {
		m.pools[name] = New()
		if m.defaultPool == nil {
			m.defaultPool = m.pools[name]
		}
	}
}

func (m *ModelPool) Add(modelName string, inst *runner.LlamaInstance) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if pool, ok := m.pools[modelName]; ok {
		pool.Add(inst)
	}
}

func (m *ModelPool) GetFree(modelName string) *runner.LlamaInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if pool, ok := m.pools[modelName]; ok {
		return pool.GetFree()
	}

	if m.defaultPool != nil {
		return m.defaultPool.GetFree()
	}

	return nil
}

func (m *ModelPool) Release(modelName string, inst *runner.LlamaInstance) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if pool, ok := m.pools[modelName]; ok {
		pool.Release(inst)
	}
}

func (m *ModelPool) ReleaseToAny(inst *runner.LlamaInstance) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, pool := range m.pools {
		pool.Release(inst)
	}
}

func (m *ModelPool) GetStats() map[string]PoolStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]PoolStats)
	for name, pool := range m.pools {
		stats[name] = pool.GetStats()
	}
	return stats
}

func (m *ModelPool) GetAllPools() map[string]*Pool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*Pool)
	for k, v := range m.pools {
		result[k] = v
	}
	return result
}
