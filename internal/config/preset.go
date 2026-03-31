package config

import (
	"fmt"
	"log"
)

type GPUMemoryTier int

const (
	Memory8GB  GPUMemoryTier = 8
	Memory12GB GPUMemoryTier = 12
	Memory16GB GPUMemoryTier = 16
	Memory24GB GPUMemoryTier = 24
	Memory32GB GPUMemoryTier = 32
	Memory48GB GPUMemoryTier = 48
	Memory64GB GPUMemoryTier = 64
	Memory96GB GPUMemoryTier = 96
)

type MemoryPreset struct {
	Name              string `yaml:"name"`
	MemoryGB          int    `yaml:"memory_gb"`
	MaxInstances      int    `yaml:"max_instances"`
	ContextSize       int    `yaml:"context_size"`
	PromptBatch       int    `yaml:"prompt_batch"`
	GenerationBatch   int    `yaml:"generation_batch"`
	GpuLayers         int    `yaml:"gpu_layers"`
	MaxQueueSize      int    `yaml:"max_queue_size"`
	MaxWaitMs         int    `yaml:"max_wait_ms"`
	UseFlashAttention bool   `yaml:"use_flash_attention"`
	UseKVCacheOffload bool   `yaml:"use_kv_cache_offload"`
	TensorParallelism int    `yaml:"tensor_parallelism"`
	KVCacheMB         int    `yaml:"kv_cache_mb"`
}

var DefaultMemoryPresets = map[GPUMemoryTier]MemoryPreset{
	Memory8GB: {
		Name:              "8GB (RTX 2070/3060)",
		MemoryGB:          8,
		MaxInstances:      2,
		ContextSize:       2048,
		PromptBatch:       512,
		GenerationBatch:   32,
		GpuLayers:         100,
		MaxQueueSize:      100,
		MaxWaitMs:         100,
		UseFlashAttention: false,
		UseKVCacheOffload: false,
		TensorParallelism: 1,
		KVCacheMB:         512,
	},
	Memory12GB: {
		Name:              "12GB (RTX 3060/4070)",
		MemoryGB:          12,
		MaxInstances:      2,
		ContextSize:       3072,
		PromptBatch:       768,
		GenerationBatch:   64,
		GpuLayers:         100,
		MaxQueueSize:      150,
		MaxWaitMs:         80,
		UseFlashAttention: true,
		UseKVCacheOffload: false,
		TensorParallelism: 1,
		KVCacheMB:         1024,
	},
	Memory16GB: {
		Name:              "16GB (RTX 4070 Ti/Apple M2 Pro)",
		MemoryGB:          16,
		MaxInstances:      3,
		ContextSize:       4096,
		PromptBatch:       1024,
		GenerationBatch:   128,
		GpuLayers:         100,
		MaxQueueSize:      200,
		MaxWaitMs:         60,
		UseFlashAttention: true,
		UseKVCacheOffload: false,
		TensorParallelism: 1,
		KVCacheMB:         2048,
	},
	Memory24GB: {
		Name:              "24GB (RTX 3090/4090)",
		MemoryGB:          24,
		MaxInstances:      4,
		ContextSize:       4096,
		PromptBatch:       1536,
		GenerationBatch:   256,
		GpuLayers:         100,
		MaxQueueSize:      300,
		MaxWaitMs:         50,
		UseFlashAttention: true,
		UseKVCacheOffload: false,
		TensorParallelism: 1,
		KVCacheMB:         4096,
	},
	Memory32GB: {
		Name:              "32GB (A4000/RTX 6000)",
		MemoryGB:          32,
		MaxInstances:      4,
		ContextSize:       8192,
		PromptBatch:       2048,
		GenerationBatch:   512,
		GpuLayers:         100,
		MaxQueueSize:      400,
		MaxWaitMs:         40,
		UseFlashAttention: true,
		UseKVCacheOffload: true,
		TensorParallelism: 1,
		KVCacheMB:         8192,
	},
	Memory48GB: {
		Name:              "48GB (A5000/A6000)",
		MemoryGB:          48,
		MaxInstances:      6,
		ContextSize:       8192,
		PromptBatch:       3072,
		GenerationBatch:   512,
		GpuLayers:         100,
		MaxQueueSize:      600,
		MaxWaitMs:         30,
		UseFlashAttention: true,
		UseKVCacheOffload: true,
		TensorParallelism: 2,
		KVCacheMB:         16384,
	},
	Memory64GB: {
		Name:              "64GB (H100 PCIE)",
		MemoryGB:          64,
		MaxInstances:      8,
		ContextSize:       16384,
		PromptBatch:       4096,
		GenerationBatch:   1024,
		GpuLayers:         100,
		MaxQueueSize:      800,
		MaxWaitMs:         20,
		UseFlashAttention: true,
		UseKVCacheOffload: true,
		TensorParallelism: 2,
		KVCacheMB:         32768,
	},
	Memory96GB: {
		Name:              "96GB (H100 SXM)",
		MemoryGB:          96,
		MaxInstances:      8,
		ContextSize:       32768,
		PromptBatch:       8192,
		GenerationBatch:   2048,
		GpuLayers:         100,
		MaxQueueSize:      1000,
		MaxWaitMs:         10,
		UseFlashAttention: true,
		UseKVCacheOffload: true,
		TensorParallelism: 4,
		KVCacheMB:         65536,
	},
}

type GPUConfig struct {
	MemoryGB      int    `yaml:"memory_gb"`
	AutoDetect    bool   `yaml:"auto_detect"`
	PresetName    string `yaml:"preset_name"`
	UseBestPreset bool   `yaml:"use_best_preset"`
	GPUIDs        []int  `yaml:"gpu_ids"`
	Scheduling    string `yaml:"scheduling"`
}

func GetPresetForMemory(memoryGB int) MemoryPreset {
	switch {
	case memoryGB <= 8:
		return DefaultMemoryPresets[Memory8GB]
	case memoryGB <= 12:
		return DefaultMemoryPresets[Memory12GB]
	case memoryGB <= 16:
		return DefaultMemoryPresets[Memory16GB]
	case memoryGB <= 24:
		return DefaultMemoryPresets[Memory24GB]
	case memoryGB <= 32:
		return DefaultMemoryPresets[Memory32GB]
	case memoryGB <= 48:
		return DefaultMemoryPresets[Memory48GB]
	case memoryGB <= 64:
		return DefaultMemoryPresets[Memory64GB]
	default:
		return DefaultMemoryPresets[Memory96GB]
	}
}

func GetPresetByName(name string) (MemoryPreset, error) {
	for _, preset := range DefaultMemoryPresets {
		if preset.Name == name {
			return preset, nil
		}
	}
	return MemoryPreset{}, fmt.Errorf("preset not found: %s", name)
}

func GetAllPresets() []MemoryPreset {
	presets := make([]MemoryPreset, 0, len(DefaultMemoryPresets))
	for _, preset := range DefaultMemoryPresets {
		presets = append(presets, preset)
	}
	return presets
}

func (c *Config) ApplyMemoryPreset() error {
	var preset MemoryPreset

	if c.GPU.PresetName != "" {
		var err error
		preset, err = GetPresetByName(c.GPU.PresetName)
		if err != nil {
			return err
		}
	} else if c.GPU.MemoryGB > 0 {
		preset = GetPresetForMemory(c.GPU.MemoryGB)
	} else if c.GPU.UseBestPreset {
		preset = GetPresetForMemory(8)
	} else {
		return nil
	}

	c.MemoryPreset = preset
	return nil
}

func DetectGPUMemory() int {
	return 8
}

func GetLlamaArgsForPreset(preset MemoryPreset) []string {
	args := []string{
		"-c", fmt.Sprintf("%d", preset.ContextSize),
		"-b", fmt.Sprintf("%d", preset.PromptBatch),
		"--parallel", "1",
	}

	if preset.UseFlashAttention {
		args = append(args, "--flash-attention")
	}

	if preset.KVCacheMB > 0 {
		args = append(args, "--numa", "dump")
	}

	return args
}

func (c *Config) ApplyPresetToConfig() error {
	if err := c.ApplyMemoryPreset(); err != nil {
		return err
	}

	preset := c.MemoryPreset
	if preset.MemoryGB == 0 {
		return nil
	}

	log.Printf("Applying memory preset: %s (%dGB)", preset.Name, preset.MemoryGB)
	log.Printf("  Max Instances: %d", preset.MaxInstances)
	log.Printf("  Context Size: %d", preset.ContextSize)
	log.Printf("  Prompt Batch: %d", preset.PromptBatch)
	log.Printf("  Generation Batch: %d", preset.GenerationBatch)
	log.Printf("  GPU Layers: %d", preset.GpuLayers)

	if c.Batcher.MaxBatchSize == 0 {
		c.Batcher.MaxBatchSize = preset.PromptBatch
	}
	if c.Batcher.MaxWaitMs == 0 {
		c.Batcher.MaxWaitMs = preset.MaxWaitMs
	}
	if c.Batcher.QueueSize == 0 {
		c.Batcher.QueueSize = preset.MaxQueueSize
	}

	log.Printf("  Queue Size: %d", c.Batcher.QueueSize)
	log.Printf("  Max Wait: %dms", c.Batcher.MaxWaitMs)

	if c.Llama.ContextSize == 0 {
		c.Llama.ContextSize = preset.ContextSize
	}
	if c.Llama.PromptBatch == 0 {
		c.Llama.PromptBatch = preset.PromptBatch
	}
	if c.Llama.GenerationBatch == 0 {
		c.Llama.GenerationBatch = preset.GenerationBatch
	}
	if c.Llama.GpuLayers == 0 {
		c.Llama.GpuLayers = preset.GpuLayers
	}

	for i := range c.Models {
		if c.Models[i].Instances == 0 {
			c.Models[i].Instances = preset.MaxInstances
		}
		if c.Models[i].ContextSize == 0 {
			c.Models[i].ContextSize = preset.ContextSize
		}
		if c.Models[i].GpuLayers == 0 {
			c.Models[i].GpuLayers = preset.GpuLayers
		}
		if c.Models[i].BatchSize == 0 {
			c.Models[i].BatchSize = preset.PromptBatch
		}
	}

	if c.KVCache.Enabled && c.KVCache.MaxSizeMB == 0 {
		c.KVCache.MaxSizeMB = int64(preset.KVCacheMB)
	}

	return nil
}

func GetMemoryTierInfo() string {
	info := "Available Memory Presets:\n"
	for _, preset := range DefaultMemoryPresets {
		info += fmt.Sprintf("  - %s\n", preset.Name)
	}
	return info
}
