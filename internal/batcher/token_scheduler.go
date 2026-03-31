package batcher

import (
	"sync"
	"time"
)

type TokenRequest struct {
	ID          string
	Prompt      string
	MaxTokens   int
	Temperature float64
	RespCh      chan *TokenResponse
	Phase       RequestPhase
	Priority    int
}

type TokenResponse struct {
	Content   string
	Error     error
	ID        string
	IsPartial bool
}

type RequestPhase int

const (
	PhasePrompt     RequestPhase = iota
	PhaseGeneration RequestPhase = iota
)

type TokenScheduler struct {
	promptQueue     []TokenRequest
	generationQueue []TokenRequest
	mu              sync.Mutex
	maxBatchSize    int
	maxWaitMs       int
	lastPromptFlush time.Time
	lastGenFlush    time.Time
}

func NewTokenScheduler(maxBatchSize, maxWaitMs int) *TokenScheduler {
	return &TokenScheduler{
		promptQueue:     make([]TokenRequest, 0),
		generationQueue: make([]TokenRequest, 0),
		maxBatchSize:    maxBatchSize,
		maxWaitMs:       maxWaitMs,
		lastPromptFlush: time.Now(),
		lastGenFlush:    time.Now(),
	}
}

func (t *TokenScheduler) Add(req TokenRequest) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if req.Phase == PhasePrompt {
		t.promptQueue = append(t.promptQueue, req)
	} else {
		t.generationQueue = append(t.generationQueue, req)
	}
}

func (t *TokenScheduler) GetPromptBatch() []TokenRequest {
	t.mu.Lock()
	defer t.mu.Unlock()

	elapsed := time.Since(t.lastPromptFlush)
	maxWait := time.Duration(t.maxWaitMs) * time.Millisecond

	if len(t.promptQueue) == 0 {
		return nil
	}

	if len(t.promptQueue) >= t.maxBatchSize || elapsed >= maxWait {
		batch := t.promptQueue
		t.promptQueue = make([]TokenRequest, 0)
		t.lastPromptFlush = time.Now()
		return batch
	}

	return nil
}

func (t *TokenScheduler) GetGenerationBatch() []TokenRequest {
	t.mu.Lock()
	defer t.mu.Unlock()

	elapsed := time.Since(t.lastGenFlush)
	maxWait := 10 * time.Millisecond

	if len(t.generationQueue) == 0 {
		return nil
	}

	if len(t.generationQueue) >= t.maxBatchSize/4 || elapsed >= maxWait {
		batch := t.generationQueue
		t.generationQueue = make([]TokenRequest, 0)
		t.lastGenFlush = time.Now()
		return batch
	}

	return nil
}

func (t *TokenScheduler) Size() (prompt, gen int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.promptQueue), len(t.generationQueue)
}

type PriorityTokenScheduler struct {
	queues    map[int][]TokenRequest
	mu        sync.Mutex
	maxBatch  int
	maxWaitMs int
}

func NewPriorityTokenScheduler(maxBatch, maxWaitMs int) *PriorityTokenScheduler {
	return &PriorityTokenScheduler{
		queues:    make(map[int][]TokenRequest),
		maxBatch:  maxBatch,
		maxWaitMs: maxWaitMs,
	}
}

func (p *PriorityTokenScheduler) Add(req TokenRequest) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.queues[req.Priority] = append(p.queues[req.Priority], req)
}

func (p *PriorityTokenScheduler) GetBatch() []TokenRequest {
	p.mu.Lock()
	defer p.mu.Unlock()

	for priority := 0; priority <= 10; priority++ {
		if len(p.queues[priority]) > 0 {
			batchSize := p.maxBatch
			if len(p.queues[priority]) < batchSize {
				batchSize = len(p.queues[priority])
			}

			batch := p.queues[priority][:batchSize]
			p.queues[priority] = p.queues[priority][batchSize:]

			return batch
		}
	}

	return nil
}

func (p *PriorityTokenScheduler) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	total := 0
	for _, q := range p.queues {
		total += len(q)
	}
	return total
}
