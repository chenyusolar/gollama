package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/anomalyco/myllama.cpp/internal/api"
	"github.com/anomalyco/myllama.cpp/internal/config"
	"github.com/anomalyco/myllama.cpp/internal/multimodal"
	"github.com/anomalyco/myllama.cpp/internal/scheduler"
)

var (
	configPath = flag.String("config", "configs/config.yaml", "Path to config file")
)

func main() {
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := cfg.ApplyPresetToConfig(); err != nil {
		log.Fatalf("Failed to apply memory preset: %v", err)
	}

	totalInstances := 0
	for _, m := range cfg.Models {
		totalInstances += m.Instances
	}

	log.Printf("Starting llama-go-server with config:")
	log.Printf("  Models: %d", len(cfg.Models))
	log.Printf("  Total Instances: %d", totalInstances)
	log.Printf("  Base Port: %d", cfg.Pool.BasePort)
	log.Printf("  Batch Size: %d", cfg.Batcher.MaxBatchSize)
	log.Printf("  Max Wait: %dms", cfg.Batcher.MaxWaitMs)
	log.Printf("  KV Cache: %v", cfg.KVCache.Enabled)

	sched := scheduler.New(cfg)

	if err := sched.Init(); err != nil {
		log.Fatalf("Failed to initialize scheduler: %v", err)
	}

	sched.Start()

	multimodalCfg := &multimodal.Config{
		ComfyUI: multimodal.ComfyUIConfig{
			Enabled:   cfg.Multimodal.ComfyUI.Enabled,
			Host:      cfg.Multimodal.ComfyUI.Host,
			Port:      cfg.Multimodal.ComfyUI.Port,
			OutputDir: cfg.Multimodal.ComfyUI.OutputDir,
			QueueSize: cfg.Multimodal.ComfyUI.QueueSize,
			Timeout:   cfg.Multimodal.ComfyUI.Timeout,
		},
		FFmpeg: multimodal.FFmpegConfig{
			Enabled:      cfg.Multimodal.FFmpeg.Enabled,
			BinaryPath:   cfg.Multimodal.FFmpeg.BinaryPath,
			OutputDir:    cfg.Multimodal.FFmpeg.OutputDir,
			OutputFormat: cfg.Multimodal.FFmpeg.OutputFormat,
			VideoCodec:   cfg.Multimodal.FFmpeg.VideoCodec,
			AudioCodec:   cfg.Multimodal.FFmpeg.AudioCodec,
			VideoBitrate: cfg.Multimodal.FFmpeg.VideoBitrate,
			FrameRate:    cfg.Multimodal.FFmpeg.FrameRate,
		},
	}
	multimodalMgr := multimodal.NewManager(multimodalCfg)
	multimodalMgr.Start()

	server := api.New(cfg, sched, multimodalMgr)
	server.Start()

	log.Printf("Server ready at http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Endpoints:")
	log.Printf("  - POST /v1/chat/completions")
	log.Printf("  - POST /v1/completions")
	log.Printf("  - GET  /health")
	log.Printf("  - GET  /stats")
	log.Printf("  - GET  /models")
	if multimodalMgr.IsHealthy() {
		log.Printf("  - POST /v1/image/generate")
		log.Printf("  - POST /v1/video/generate")
		log.Printf("  - POST /v1/video/compose")
		log.Printf("  - POST /v1/image/upscale")
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	server.Stop()
	sched.Stop()
	fmt.Println("Server stopped")
}
