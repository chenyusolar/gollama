package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig     `yaml:"server"`
	Llama        LlamaConfig      `yaml:"llama"`
	Models       []ModelConfig    `yaml:"models"`
	Pool         PoolConfig       `yaml:"pool"`
	Batcher      BatcherConfig    `yaml:"batcher"`
	KVCache      KVCacheConfig    `yaml:"kv_cache"`
	GPU          GPUConfig        `yaml:"gpu"`
	MemoryPreset MemoryPreset     `yaml:"-"`
	Multimodal   MultimodalConfig `yaml:"multimodal"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type LlamaConfig struct {
	Binary          string `yaml:"binary"`
	Model           string `yaml:"model"`
	ContextSize     int    `yaml:"context_size"`
	PromptBatch     int    `yaml:"prompt_batch"`
	GenerationBatch int    `yaml:"generation_batch"`
	GpuLayers       int    `yaml:"gpu_layers"`
}

type ModelConfig struct {
	Name        string  `yaml:"name"`
	Path        string  `yaml:"path"`
	Instances   int     `yaml:"instances"`
	ContextSize int     `yaml:"context_size"`
	Priority    int     `yaml:"priority"`
	GpuLayers   int     `yaml:"gpu_layers"`
	BatchSize   int     `yaml:"batch_size"`
	Temperature float64 `yaml:"temperature"`
	MaxTokens   int     `yaml:"max_tokens"`
}

type PoolConfig struct {
	BasePort            int           `yaml:"base_port"`
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`
}

type BatcherConfig struct {
	MaxWaitMs    int `yaml:"max_wait_ms"`
	MaxBatchSize int `yaml:"max_batch_size"`
	QueueSize    int `yaml:"queue_size"`
}

type KVCacheConfig struct {
	Enabled     bool   `yaml:"enabled"`
	CacheDir    string `yaml:"cache_dir"`
	MaxSizeMB   int64  `yaml:"max_size_mb"`
	MaxAgeHours int    `yaml:"max_age_hours"`
}

type MultimodalConfig struct {
	ComfyUI ComfyUIConfig `yaml:"comfyui"`
	FFmpeg  FFmpegConfig  `yaml:"ffmpeg"`
}

type ComfyUIConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	WorkflowDir string `yaml:"workflow_dir"`
	OutputDir   string `yaml:"output_dir"`
	QueueSize   int    `yaml:"queue_size"`
	Timeout     int    `yaml:"timeout"`
}

type FFmpegConfig struct {
	Enabled      bool   `yaml:"enabled"`
	BinaryPath   string `yaml:"binary_path"`
	OutputDir    string `yaml:"output_dir"`
	OutputFormat string `yaml:"output_format"`
	VideoCodec   string `yaml:"video_codec"`
	AudioCodec   string `yaml:"audio_codec"`
	VideoBitrate string `yaml:"video_bitrate"`
	FrameRate    int    `yaml:"frame_rate"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if len(cfg.Models) == 0 && cfg.Llama.Model != "" {
		cfg.Models = []ModelConfig{
			{
				Name:        "default",
				Path:        cfg.Llama.Model,
				Instances:   2,
				ContextSize: cfg.Llama.ContextSize,
				GpuLayers:   cfg.Llama.GpuLayers,
				BatchSize:   cfg.Llama.PromptBatch,
				Temperature: 0.7,
				MaxTokens:   512,
			},
		}
	}

	if cfg.Batcher.MaxBatchSize == 0 {
		cfg.Batcher.MaxBatchSize = 512
	}
	if cfg.Batcher.MaxWaitMs == 0 {
		cfg.Batcher.MaxWaitMs = 100
	}
	if cfg.Batcher.QueueSize == 0 {
		cfg.Batcher.QueueSize = 100
	}

	return &cfg, nil
}
