package batcher

import (
	"sync"
	"time"

	"github.com/anomalyco/myllama.cpp/internal/config"
)

type Request struct {
	Prompt      string
	MaxTokens   int
	Temperature float64
	RespCh      chan *Response
	ID          string
}

type Response struct {
	Content string
	Error   error
	ID      string
}

type Batcher struct {
	queue     []Request
	mu        sync.Mutex
	cfg       *config.BatcherConfig
	lastFlush time.Time
}

func New(cfg *config.BatcherConfig) *Batcher {
	return &Batcher{
		queue:     make([]Request, 0, cfg.QueueSize),
		cfg:       cfg,
		lastFlush: time.Now(),
	}
}

func (b *Batcher) Add(req Request) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.queue) >= b.cfg.QueueSize {
		return
	}

	b.queue = append(b.queue, req)
}

func (b *Batcher) Flush() []Request {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.queue) == 0 {
		return nil
	}

	elapsed := time.Since(b.lastFlush)
	maxWait := time.Duration(b.cfg.MaxWaitMs) * time.Millisecond

	if len(b.queue) < b.cfg.MaxBatchSize && elapsed < maxWait {
		return nil
	}

	batch := b.queue
	b.queue = make([]Request, 0, b.cfg.QueueSize)
	b.lastFlush = time.Now()

	return batch
}

func (b *Batcher) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.queue)
}

func (b *Batcher) ForceFlush() []Request {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.queue) == 0 {
		return nil
	}

	batch := b.queue
	b.queue = make([]Request, 0, b.cfg.QueueSize)
	b.lastFlush = time.Now()

	return batch
}
