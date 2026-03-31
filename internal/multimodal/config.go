package multimodal

import "time"

type Config struct {
	ComfyUI   ComfyUIConfig   `yaml:"comfyui"`
	FFmpeg    FFmpegConfig    `yaml:"ffmpeg"`
	Stability StabilityConfig `yaml:"stability"`
	Output    OutputConfig    `yaml:"output"`
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

type StabilityConfig struct {
	Enabled bool   `yaml:"enabled"`
	APIKey  string `yaml:"api_key"`
	Engine  string `yaml:"engine"`
	Host    string `yaml:"host"`
}

type OutputConfig struct {
	BaseDir  string `yaml:"base_dir"`
	Format   string `yaml:"format"`
	Quality  int    `yaml:"quality"`
	KeepTemp bool   `yaml:"keep_temp"`
}

type ImageRequest struct {
	Prompt         string       `json:"prompt"`
	NegativePrompt string       `json:"negative_prompt,omitempty"`
	Width          int          `json:"width,omitempty"`
	Height         int          `json:"height,omitempty"`
	Steps          int          `json:"steps,omitempty"`
	CFG            float64      `json:"cfg_scale,omitempty"`
	Seed           int64        `json:"seed,omitempty"`
	Model          string       `json:"model,omitempty"`
	Sampler        string       `json:"sampler,omitempty"`
	ControlNets    []ControlNet `json:"control_nets,omitempty"`
	IPAdapters     []IPAdapter  `json:"ip_adapters,omitempty"`
}

type ControlNet struct {
	Enabled      bool    `json:"enabled"`
	ImageBase64  string  `json:"image_base64,omitempty"`
	Preprocessor string  `json:"preprocessor,omitempty"`
	Model        string  `json:"model,omitempty"`
	Weight       float64 `json:"weight,omitempty"`
	StartStep    float64 `json:"start_step,omitempty"`
	EndStep      float64 `json:"end_step,omitempty"`
}

type IPAdapter struct {
	Enabled     bool    `json:"enabled"`
	ImageBase64 string  `json:"image_base64,omitempty"`
	Model       string  `json:"model,omitempty"`
	Weight      float64 `json:"weight,omitempty"`
}

type VideoRequest struct {
	ImageRequest
	Frames      int     `json:"frames,omitempty"`
	FPS         int     `json:"fps,omitempty"`
	Duration    float64 `json:"duration,omitempty"`
	Loop        bool    `json:"loop,omitempty"`
	AnimateDiff string  `json:"animate_diff,omitempty"`
	MotionLora  string  `json:"motion_lora,omitempty"`
	Seed        int64   `json:"seed,omitempty"`
}

type UpscaleRequest struct {
	ImageBase64 string  `json:"image_base64"`
	Scale       int     `json:"scale,omitempty"`
	Model       string  `json:"model,omitempty"`
	Denoising   float64 `json:"denoising_strength,omitempty"`
}

type ImageResponse struct {
	ImageBase64 string `json:"image_base64,omitempty"`
	ImagePath   string `json:"image_path,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	Seed        int64  `json:"seed,omitempty"`
	Steps       int    `json:"steps,omitempty"`
	TimeTaken   int64  `json:"time_taken_ms,omitempty"`
}

type VideoResponse struct {
	VideoPath  string `json:"video_path,omitempty"`
	VideoURL   string `json:"video_url,omitempty"`
	FrameCount int    `json:"frame_count,omitempty"`
	FPS        int    `json:"fps,omitempty"`
	TimeTaken  int64  `json:"time_taken_ms,omitempty"`
}

type Task struct {
	ID          string
	Type        string
	Request     interface{}
	ResponseCh  chan interface{}
	Status      string
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	Result      interface{}
	Error       error
}

func NewTask(taskType string, request interface{}) *Task {
	return &Task{
		ID:         generateTaskID(),
		Type:       taskType,
		Request:    request,
		ResponseCh: make(chan interface{}, 1),
		Status:     "pending",
		CreatedAt:  time.Now(),
	}
}

func generateTaskID() string {
	return time.Now().Format("20060102150405")
}
