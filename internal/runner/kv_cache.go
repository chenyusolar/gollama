package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type KVCacheManager struct {
	cacheDir string
	cache    map[string]*KVCacheEntry
	mu       sync.RWMutex
	maxAge   time.Duration
	maxSize  int64
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

type KVCacheEntry struct {
	Prompt    string
	FilePath  string
	ModelHash string
	LastUsed  time.Time
	Size      int64
}

func NewKVCacheManager(cacheDir string, maxAge time.Duration, maxSize int64) *KVCacheManager {
	os.MkdirAll(cacheDir, 0755)

	k := &KVCacheManager{
		cacheDir: cacheDir,
		cache:    make(map[string]*KVCacheEntry),
		maxAge:   maxAge,
		maxSize:  maxSize,
		stopCh:   make(chan struct{}),
	}

	k.wg.Add(1)
	go k.periodicCleanup()

	return k
}

func (k *KVCacheManager) Stop() {
	close(k.stopCh)
	k.wg.Wait()
}

func (k *KVCacheManager) periodicCleanup() {
	defer k.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-k.stopCh:
			return
		case <-ticker.C:
			k.cleanup()
		}
	}
}

func (k *KVCacheManager) GetCacheKey(prompt string, modelPath string) string {
	hash := simpleHash(prompt + "-" + filepath.Base(modelPath))
	return hash
}

func (k *KVCacheManager) Get(prompt string, modelPath string) (string, bool) {
	key := k.GetCacheKey(prompt, modelPath)

	k.mu.RLock()
	entry, exists := k.cache[key]
	k.mu.RUnlock()

	if !exists {
		return "", false
	}

	if time.Since(entry.LastUsed) > k.maxAge {
		k.removeEntry(key)
		return "", false
	}

	if _, err := os.Stat(entry.FilePath); err != nil {
		k.removeEntry(key)
		return "", false
	}

	k.mu.Lock()
	entry.LastUsed = time.Now()
	k.mu.Unlock()

	return entry.FilePath, true
}

func (k *KVCacheManager) Set(prompt string, modelPath string, cacheFile string) {
	key := k.GetCacheKey(prompt, modelPath)

	stat, err := os.Stat(cacheFile)
	if err != nil {
		return
	}

	entry := &KVCacheEntry{
		Prompt:    prompt,
		FilePath:  cacheFile,
		ModelHash: simpleHash(modelPath),
		LastUsed:  time.Now(),
		Size:      stat.Size(),
	}

	k.mu.Lock()
	k.cache[key] = entry
	k.mu.Unlock()
}

func (k *KVCacheManager) removeEntry(key string) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if entry, ok := k.cache[key]; ok {
		os.Remove(entry.FilePath)
		delete(k.cache, key)
	}
}

func (k *KVCacheManager) cleanup() {
	k.mu.Lock()
	defer k.mu.Unlock()

	var totalSize int64
	for _, entry := range k.cache {
		totalSize += entry.Size
	}

	if totalSize > k.maxSize {
		oldest := time.Now()
		var oldestKey string

		for key, entry := range k.cache {
			if entry.LastUsed.Before(oldest) {
				oldest = entry.LastUsed
				oldestKey = key
			}
		}

		if oldestKey != "" {
			if entry, ok := k.cache[oldestKey]; ok {
				os.Remove(entry.FilePath)
				totalSize -= entry.Size
				delete(k.cache, oldestKey)
			}
		}
	}

	for key, entry := range k.cache {
		if time.Since(entry.LastUsed) > k.maxAge {
			os.Remove(entry.FilePath)
			totalSize -= entry.Size
			delete(k.cache, key)
		}
	}
}

func simpleHash(s string) string {
	h := uint64(5381)
	for i := 0; i < len(s); i++ {
		h = h*33 + uint64(s[i])
	}
	return fmt.Sprintf("%x", h)
}

func (k *KVCacheManager) GetCachePath(prompt string, modelPath string) string {
	key := k.GetCacheKey(prompt, modelPath)
	return filepath.Join(k.cacheDir, fmt.Sprintf("cache_%s.bin", key))
}
