package scheduler

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/anomalyco/myllama.cpp/internal/batcher"
	"github.com/anomalyco/myllama.cpp/internal/config"
	"github.com/anomalyco/myllama.cpp/internal/gpu"
	"github.com/anomalyco/myllama.cpp/internal/pool"
	"github.com/anomalyco/myllama.cpp/internal/runner"
)

type Scheduler struct {
	batcher    *batcher.Batcher
	modelPools *pool.ModelPool
	cfg        *config.Config
	runners    map[string]*runner.LlamaRunner
	kvCacheMgr *runner.KVCacheManager
	gpuMgr     *gpu.GPUManager
	mu         sync.RWMutex
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

func New(cfg *config.Config) *Scheduler {
	sched := &Scheduler{
		batcher:    batcher.New(&cfg.Batcher),
		modelPools: pool.NewModelPool(),
		cfg:        cfg,
		runners:    make(map[string]*runner.LlamaRunner),
		stopCh:     make(chan struct{}),
	}

	if cfg.KVCache.Enabled {
		sched.kvCacheMgr = runner.NewKVCacheManager(
			cfg.KVCache.CacheDir,
			time.Duration(cfg.KVCache.MaxAgeHours)*time.Hour,
			cfg.KVCache.MaxSizeMB*1024*1024,
		)
	}

	if cfg.GPU.AutoDetect {
		sched.gpuMgr = gpu.NewGPUManager(cfg.GPU.MemoryGB)
	}

	return sched
}

func (s *Scheduler) Init() error {
	if s.gpuMgr != nil {
		if err := s.gpuMgr.Start(); err != nil {
			log.Printf("Warning: GPU detection failed: %v", err)
		} else {
			log.Printf("GPU Manager started with %d GPU(s)", s.gpuMgr.Count())
		}
	}

	totalInstances := 0

	for _, model := range s.cfg.Models {
		s.modelPools.AddModel(model.Name)

		llamaCfg := config.LlamaConfig{
			Binary:          s.cfg.Llama.Binary,
			Model:           model.Path,
			ContextSize:     model.ContextSize,
			PromptBatch:     model.BatchSize,
			GenerationBatch: s.cfg.Llama.GenerationBatch,
			GpuLayers:       model.GpuLayers,
		}

		r := runner.NewLlamaRunner(&llamaCfg)
		s.runners[model.Name] = r

		gpuIDs := s.assignGPUIDs(model.Instances)
		instances, err := r.StartAll(model.Instances, s.cfg.Pool.BasePort+totalInstances, gpuIDs)
		if err != nil {
			return fmt.Errorf("failed to start %s instances: %w", model.Name, err)
		}

		for _, inst := range instances {
			inst.Model = model.Name
			inst.CacheMgr = s.kvCacheMgr
			s.modelPools.Add(model.Name, inst)
		}

		log.Printf("Started %d instances for model: %s", model.Instances, model.Name)
		totalInstances += model.Instances
	}

	log.Printf("Started %d total llama.cpp instances", totalInstances)
	return nil
}

func (s *Scheduler) assignGPUIDs(instanceCount int) []int {
	if s.gpuMgr == nil || s.cfg.GPU.GPUIDs == nil {
		return nil
	}

	gpuIDs := make([]int, 0, instanceCount)
	for i := range instanceCount {
		gpuIdx := i % len(s.cfg.GPU.GPUIDs)
		gpuIDs = append(gpuIDs, s.cfg.GPU.GPUIDs[gpuIdx])
	}

	return gpuIDs
}

func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.runScheduler()

	s.wg.Add(1)
	go s.runHealthChecker()

	log.Println("Scheduler started")
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-s.stopCh:
		// Already closed
	default:
		close(s.stopCh)
	}

	s.wg.Wait()

	for _, r := range s.runners {
		r.StopAll()
	}

	log.Println("Scheduler stopped")
}

func (s *Scheduler) runScheduler() {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Duration(s.cfg.Batcher.MaxWaitMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.processBatch()
		}
	}
}

func (s *Scheduler) processBatch() {
	batch := s.batcher.Flush()
	if len(batch) == 0 {
		return
	}

	modelName := "default"
	if len(batch) > 0 && batch[0].ID != "" {
		modelName = batch[0].ID
	}

	inst := s.modelPools.GetFree(modelName)
	if inst == nil {
		for _, req := range batch {
			req.RespCh <- &batcher.Response{
				Error: fmt.Errorf("no available instances"),
				ID:    req.ID,
			}
		}
		return
	}

	go s.handleBatch(inst, batch)
}

func (s *Scheduler) handleBatch(inst *runner.LlamaInstance, batch []batcher.Request) {
	defer s.modelPools.Release(inst.Model, inst)

	var wg sync.WaitGroup
	for _, req := range batch {
		wg.Add(1)
		go func(r batcher.Request) {
			defer wg.Done()

			content, err := inst.Complete(r.Prompt, r.MaxTokens, r.Temperature)
			r.RespCh <- &batcher.Response{
				Content: content,
				Error:   err,
				ID:      r.ID,
			}
		}(req)
	}

	wg.Wait()
}

func (s *Scheduler) runHealthChecker() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.cfg.Pool.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.checkHealth()
		}
	}
}

func (s *Scheduler) checkHealth() {
	for modelName, pool := range s.modelPools.GetAllPools() {
		instances := pool.GetAll()
		for _, inst := range instances {
			if !inst.IsHealthy() {
				log.Printf("Instance %d (%s) is unhealthy, restarting...", inst.ID, modelName)
				go s.restartInstance(modelName, inst)
			}
		}
	}
}

func (s *Scheduler) restartInstance(modelName string, inst *runner.LlamaInstance) {
	s.modelPools.Release(modelName, inst)
	inst.Stop()

	time.Sleep(1 * time.Second)

	r := s.runners[modelName]
	if r == nil {
		log.Printf("Runner for model %s not found", modelName)
		return
	}

	newInst, err := r.StartInstance(inst.ID, inst.Port, inst.GPUID)
	if err != nil {
		log.Printf("Failed to restart instance %d: %v", inst.ID, err)
		// Attempt to restart the instance with a delay
		go func() {
			time.Sleep(5 * time.Second)
			newInst, retryErr := r.StartInstance(inst.ID, inst.Port, inst.GPUID)
			if retryErr != nil {
				log.Printf("Retry failed to restart instance %d: %v", inst.ID, retryErr)
				return
			}
			newInst.Model = modelName
			newInst.CacheMgr = s.kvCacheMgr
			s.modelPools.Add(modelName, newInst)
			log.Printf("Instance %d (%s) restarted successfully (retry)", inst.ID, modelName)
		}()
		return
	}

	newInst.Model = modelName
	newInst.CacheMgr = s.kvCacheMgr
	s.modelPools.Add(modelName, newInst)
	log.Printf("Instance %d (%s) restarted successfully", inst.ID, modelName)
}

func (s *Scheduler) Submit(prompt string, maxTokens int, temperature float64, id string, modelName string) chan *batcher.Response {
	respCh := make(chan *batcher.Response, 1)

	s.batcher.Add(batcher.Request{
		Prompt:      prompt,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		RespCh:      respCh,
		ID:          modelName,
	})

	return respCh
}

func (s *Scheduler) GetFreeInstance() *runner.LlamaInstance {
	return s.modelPools.GetFree("default")
}

func (s *Scheduler) ReleaseInstance(inst *runner.LlamaInstance) {
	s.modelPools.Release(inst.Model, inst)
}

func (s *Scheduler) SubmitToModel(prompt string, maxTokens int, temperature float64, modelName string) chan *batcher.Response {
	respCh := make(chan *batcher.Response, 1)

	s.batcher.Add(batcher.Request{
		Prompt:      prompt,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		RespCh:      respCh,
		ID:          modelName,
	})

	return respCh
}

func (s *Scheduler) GetStats() SchedulerStats {
	allStats := s.modelPools.GetStats()

	total, busy, free := 0, 0, 0
	for _, stats := range allStats {
		total += stats.Total
		busy += stats.Busy
		free += stats.Free
	}

	stats := SchedulerStats{
		TotalInstances: total,
		BusyInstances:  busy,
		FreeInstances:  free,
		QueueSize:      s.batcher.Size(),
		ModelStats:     allStats,
	}

	if s.gpuMgr != nil {
		stats.GPUStats = s.gpuMgr.GetStats()
	}

	return stats
}

type SchedulerStats struct {
	TotalInstances int
	BusyInstances  int
	FreeInstances  int
	QueueSize      int
	ModelStats     map[string]pool.PoolStats
	GPUStats       gpu.GPUStats
}
