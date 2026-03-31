package multimodal

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type Manager struct {
	comfyUI *ComfyUI
	ffmpeg  *FFmpeg
	config  *Config
	queue   chan *Task
	mu      sync.RWMutex
	tasks   map[string]*Task
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

func NewManager(cfg *Config) *Manager {
	m := &Manager{
		config: cfg,
		queue:  make(chan *Task, cfg.ComfyUI.QueueSize),
		tasks:  make(map[string]*Task),
		stopCh: make(chan struct{}),
	}

	if cfg.ComfyUI.Enabled {
		m.comfyUI = NewComfyUI(
			cfg.ComfyUI.Host,
			cfg.ComfyUI.Port,
			cfg.ComfyUI.Timeout,
		)
	}

	if cfg.FFmpeg.Enabled {
		m.ffmpeg = NewFFmpeg(
			cfg.FFmpeg.BinaryPath,
			cfg.FFmpeg.OutputDir,
			cfg.FFmpeg.OutputFormat,
			cfg.FFmpeg.VideoCodec,
			cfg.FFmpeg.AudioCodec,
			cfg.FFmpeg.VideoBitrate,
			cfg.FFmpeg.FrameRate,
		)
	}

	return m
}

func (m *Manager) Start() {
	if m.comfyUI == nil && m.ffmpeg == nil {
		log.Println("Multimodal manager: no services enabled")
		return
	}

	m.wg.Add(1)
	go m.processQueue()

	log.Println("Multimodal manager started")
}

func (m *Manager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
	log.Println("Multimodal manager stopped")
}

func (m *Manager) processQueue() {
	defer m.wg.Done()

	for {
		select {
		case <-m.stopCh:
			return
		case task := <-m.queue:
			m.processTask(task)
		}
	}
}

func (m *Manager) processTask(task *Task) {
	m.mu.Lock()
	task.Status = "processing"
	now := time.Now()
	task.StartedAt = &now
	m.mu.Unlock()

	var result interface{}
	var err error

	switch task.Type {
	case "image":
		result, err = m.GenerateImage(task.Request.(ImageRequest))
	case "video":
		result, err = m.GenerateVideo(task.Request.(VideoRequest))
	case "upscale":
		result, err = m.UpscaleImage(task.Request.(UpscaleRequest))
	case "compose_video":
		result, err = m.ComposeVideo(task.Request.(ComposeVideoRequest))
	default:
		err = fmt.Errorf("unknown task type: %s", task.Type)
	}

	m.mu.Lock()
	if err != nil {
		task.Status = "failed"
		task.Error = err
	} else {
		task.Status = "completed"
		task.Result = result
	}
	now = time.Now()
	task.CompletedAt = &now
	m.mu.Unlock()

	m.cleanupOldTasks()

	select {
	case task.ResponseCh <- result:
	default:
	}
}

func (m *Manager) cleanupOldTasks() {
	const maxCompletedAge = 5 * time.Minute
	now := time.Now()

	for id, task := range m.tasks {
		if task.Status == "completed" || task.Status == "failed" {
			if task.CompletedAt != nil && now.Sub(*task.CompletedAt) > maxCompletedAge {
				delete(m.tasks, id)
			}
		}
	}
}

func (m *Manager) GenerateImage(req ImageRequest) (ImageResponse, error) {
	if m.comfyUI == nil {
		return ImageResponse{}, fmt.Errorf("comfyui not enabled")
	}

	workflow := NewWorkflowBuilder(m.comfyUI).BuildSimpleImageWorkflow(req)

	promptID, err := m.comfyUI.QueuePrompt(workflow)
	if err != nil {
		return ImageResponse{}, fmt.Errorf("failed to queue workflow: %w", err)
	}

	log.Printf("Queued image generation: prompt_id=%s", promptID)

	history, err := m.comfyUI.WaitForCompletion(promptID, 300*time.Second)
	if err != nil {
		return ImageResponse{}, fmt.Errorf("generation failed: %w", err)
	}

	images, err := m.comfyUI.GetOutputImages(history)
	if err != nil || len(images) == 0 {
		return ImageResponse{}, fmt.Errorf("no output images found")
	}

	imagePath := images[0]
	outputPath := m.config.ComfyUI.OutputDir + "/" + fmt.Sprintf("output_%d.png", time.Now().Unix())

	if err := m.comfyUI.DownloadImage(imagePath, outputPath); err != nil {
		return ImageResponse{}, fmt.Errorf("failed to download image: %w", err)
	}

	log.Printf("Image generated: %s", outputPath)

	return ImageResponse{
		ImagePath: outputPath,
		Seed:      req.Seed,
	}, nil
}

func (m *Manager) GenerateVideo(req VideoRequest) (VideoResponse, error) {
	if m.comfyUI == nil {
		return VideoResponse{}, fmt.Errorf("comfyui not enabled")
	}

	workflow := NewWorkflowBuilder(m.comfyUI).BuildVideoWorkflow(req)

	promptID, err := m.comfyUI.QueuePrompt(workflow)
	if err != nil {
		return VideoResponse{}, fmt.Errorf("failed to queue workflow: %w", err)
	}

	log.Printf("Queued video generation: prompt_id=%s", promptID)

	history, err := m.comfyUI.WaitForCompletion(promptID, 600*time.Second)
	if err != nil {
		return VideoResponse{}, fmt.Errorf("generation failed: %w", err)
	}

	images, err := m.comfyUI.GetOutputImages(history)
	if err != nil || len(images) == 0 {
		return VideoResponse{}, fmt.Errorf("no output images found")
	}

	imagePaths := make([]string, len(images))
	for i, img := range images {
		outputPath := m.config.ComfyUI.OutputDir + fmt.Sprintf("/frame_%05d.png", i)
		if err := m.comfyUI.DownloadImage(img, outputPath); err != nil {
			return VideoResponse{}, fmt.Errorf("failed to download frame: %w", err)
		}
		imagePaths[i] = outputPath
	}

	videoPath, err := m.ffmpeg.ImagesToVideo(imagePaths, fmt.Sprintf("video_%d", time.Now().Unix()), req.FPS)
	if err != nil {
		return VideoResponse{}, fmt.Errorf("failed to compose video: %w", err)
	}

	return VideoResponse{
		VideoPath:  videoPath,
		FrameCount: req.Frames,
		FPS:        req.FPS,
	}, nil
}

func (m *Manager) UpscaleImage(req UpscaleRequest) (ImageResponse, error) {
	if m.comfyUI == nil {
		return ImageResponse{}, fmt.Errorf("comfyui not enabled")
	}

	workflow := NewWorkflowBuilder(m.comfyUI).BuildUpscaleWorkflow(req)

	promptID, err := m.comfyUI.QueuePrompt(workflow)
	if err != nil {
		return ImageResponse{}, fmt.Errorf("failed to queue workflow: %w", err)
	}

	history, err := m.comfyUI.WaitForCompletion(promptID, 300*time.Second)
	if err != nil {
		return ImageResponse{}, fmt.Errorf("upscale failed: %w", err)
	}

	images, err := m.comfyUI.GetOutputImages(history)
	if err != nil || len(images) == 0 {
		return ImageResponse{}, fmt.Errorf("no output images found")
	}

	imagePath := images[0]
	outputPath := m.config.ComfyUI.OutputDir + fmt.Sprintf("/upscaled_%d.png", time.Now().Unix())

	if err := m.comfyUI.DownloadImage(imagePath, outputPath); err != nil {
		return ImageResponse{}, fmt.Errorf("failed to download image: %w", err)
	}

	return ImageResponse{
		ImagePath: outputPath,
	}, nil
}

type ComposeVideoRequest struct {
	ImagePaths         []string
	AudioPath          string
	OutputName         string
	FPS                int
	Transition         string
	TransitionDuration float64
}

func (m *Manager) ComposeVideo(req ComposeVideoRequest) (string, error) {
	if m.ffmpeg == nil {
		return "", fmt.Errorf("ffmpeg not enabled")
	}

	if len(req.ImagePaths) == 0 {
		return "", fmt.Errorf("no images provided")
	}

	videoPath, err := m.ffmpeg.ImagesToVideo(req.ImagePaths, req.OutputName, req.FPS)
	if err != nil {
		return "", err
	}

	if req.AudioPath != "" {
		audioName := req.OutputName + "_with_audio"
		videoPath, err = m.ffmpeg.VideoWithAudio(videoPath, req.AudioPath, audioName)
		if err != nil {
			return "", err
		}
	}

	return videoPath, nil
}

func (m *Manager) SubmitTask(task *Task) {
	m.mu.Lock()
	m.tasks[task.ID] = task
	m.mu.Unlock()

	m.queue <- task
}

func (m *Manager) GetTask(id string) (*Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasks[id]
	return task, ok
}

func (m *Manager) IsHealthy() bool {
	if m.comfyUI != nil {
		return m.comfyUI.IsReady()
	}
	return true
}
