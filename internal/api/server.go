package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/anomalyco/myllama.cpp/internal/batcher"
	"github.com/anomalyco/myllama.cpp/internal/config"
	"github.com/anomalyco/myllama.cpp/internal/multimodal"
	"github.com/anomalyco/myllama.cpp/internal/scheduler"
	"github.com/gorilla/websocket"
)

type Server struct {
	cfg           *config.Config
	scheduler     *scheduler.Scheduler
	multimodalMgr *multimodal.Manager
	httpServer    *http.Server
}

func New(cfg *config.Config, sched *scheduler.Scheduler, multimodalMgr *multimodal.Manager) *Server {
	return &Server{
		cfg:           cfg,
		scheduler:     sched,
		multimodalMgr: multimodalMgr,
	}
}

func (s *Server) Start() {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("/v1/completions", s.handleCompletions)
	mux.HandleFunc("/v1/chat/completions_stream", s.handleChatCompletionsStream)
	mux.HandleFunc("/v1/completions_stream", s.handleCompletionsStream)
	mux.HandleFunc("/ws/chat", s.handleWebSocketChat)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/models", s.handleModels)

	if s.multimodalMgr != nil {
		mux.HandleFunc("/v1/image/generate", s.handleImageGenerate)
		mux.HandleFunc("/v1/video/generate", s.handleVideoGenerate)
		mux.HandleFunc("/v1/video/compose", s.handleComposeVideo)
		mux.HandleFunc("/v1/image/upscale", s.handleUpscaleImage)
	}

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 0, // No timeout for streaming
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("HTTP server starting on %s:%d", s.cfg.Server.Host, s.cfg.Server.Port)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()
}

func (s *Server) Stop() error {
	return s.httpServer.Close()
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func formatChatPrompt(messages []ChatMessage) string {
	var prompt string
	for _, msg := range messages {
		if msg.Role == "user" {
			prompt += "<|user|>\n" + msg.Content + "\n"
		} else if msg.Role == "assistant" {
			prompt += "<|assistant|>\n" + msg.Content + "\n"
		} else if msg.Role == "system" {
			prompt += "<|system|>\n" + msg.Content + "\n"
		}
	}
	prompt += "<|assistant|>\n"
	return prompt
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(req.Messages) == 0 {
		http.Error(w, "messages is required", http.StatusBadRequest)
		return
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 512
	}

	temp := req.Temperature
	if temp == 0 {
		temp = 0.7
	}

	prompt := ""
	for _, msg := range req.Messages {
		prompt += msg.Content + "\n"
	}

	formattedPrompt := formatChatPrompt(req.Messages)

	// Support streaming based on request parameter
	if req.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		inst := s.scheduler.GetFreeInstance()
		if inst == nil {
			fmt.Fprintf(w, "data: {\"error\":\"no available instances\"}\n\n")
			flusher.Flush()
			return
		}

		log.Printf("[handleChatCompletions] Starting streaming for prompt length: %d, maxTokens: %d", len(formattedPrompt), maxTokens)
		log.Printf("[handleChatCompletions] Original prompt: %s", formattedPrompt)

		tokenCount := 0
		err := inst.CompleteStream(formattedPrompt, maxTokens, temp, func(token string) {
			tokenCount++
			log.Printf("[handleChatCompletions] Writing token %d to client: %s", tokenCount, token)
			escaped := strings.ReplaceAll(token, "\\", "\\\\")
			escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
			escaped = strings.ReplaceAll(escaped, "\n", "\\n")
			escaped = strings.ReplaceAll(escaped, "\r", "\\r")
			escaped = strings.ReplaceAll(escaped, "\t", "\\t")
			_, writeErr := fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"%s\"}}]}\n\n", escaped)
			if writeErr != nil {
				log.Printf("[handleChatCompletions] Error writing token %d: %v", tokenCount, writeErr)
			}
			flusher.Flush()
		})

		log.Printf("[handleChatCompletions] Streaming completed. Total tokens: %d, error: %v", tokenCount, err)

		s.scheduler.ReleaseInstance(inst)

		if err != nil {
			errMsg := strings.ReplaceAll(err.Error(), "\\", "\\\\")
			errMsg = strings.ReplaceAll(errMsg, "\"", "\\\"")
			errMsg = strings.ReplaceAll(errMsg, "\n", "\\n")
			errMsg = strings.ReplaceAll(errMsg, "\r", "\\r")
			errMsg = strings.ReplaceAll(errMsg, "\t", "\\t")
			fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", errMsg)
			flusher.Flush()
		} else {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}
		return
	}

	// Non-streaming request
	respCh := s.scheduler.Submit(formattedPrompt, maxTokens, temp, "", req.Model)

	var result *batcher.Response
	select {
	case result = <-respCh:
	case <-time.After(30 * time.Minute):
		http.Error(w, "Request timeout", http.StatusGatewayTimeout)
		return
	}

	if result == nil {
		http.Error(w, "Internal error: nil response", http.StatusInternalServerError)
		return
	}

	if result.Error != nil {
		http.Error(w, result.Error.Error(), http.StatusInternalServerError)
		return
	}

	response := ChatResponse{
		ID:      "chatcmpl-" + fmt.Sprintf("%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    "assistant",
					Content: result.Content,
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			TotalTokens: len(prompt) / 4,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type CompletionRequest struct {
	Prompt      string  `json:"prompt"`
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Stream      bool    `json:"stream,omitempty"`
}

type CompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

func (s *Server) handleCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 512
	}

	temp := req.Temperature
	if temp == 0 {
		temp = 0.7
	}

	// Support streaming based on request parameter
	if req.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		inst := s.scheduler.GetFreeInstance()
		if inst == nil {
			fmt.Fprintf(w, "data: {\"error\":\"no available instances\"}\n\n")
			flusher.Flush()
			return
		}

		err := inst.CompleteStream(req.Prompt, maxTokens, temp, func(token string) {
			escaped := strings.ReplaceAll(token, "\\", "\\\\")
			escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
			escaped = strings.ReplaceAll(escaped, "\n", "\\n")
			escaped = strings.ReplaceAll(escaped, "\r", "\\r")
			escaped = strings.ReplaceAll(escaped, "\t", "\\t")
			fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"text\":\"%s\"}]}\n\n", escaped)
			flusher.Flush()
		})

		s.scheduler.ReleaseInstance(inst)

		if err != nil {
			errMsg := strings.ReplaceAll(err.Error(), "\\", "\\\\")
			errMsg = strings.ReplaceAll(errMsg, "\"", "\\\"")
			errMsg = strings.ReplaceAll(errMsg, "\n", "\\n")
			errMsg = strings.ReplaceAll(errMsg, "\r", "\\r")
			errMsg = strings.ReplaceAll(errMsg, "\t", "\\t")
			fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", errMsg)
			flusher.Flush()
		} else {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}
		return
	}

	// Non-streaming request
	respCh := s.scheduler.Submit(req.Prompt, maxTokens, temp, "", "default")

	var result *batcher.Response
	select {
	case result = <-respCh:
	case <-time.After(30 * time.Minute):
		http.Error(w, "Request timeout", http.StatusGatewayTimeout)
		return
	}

	if result == nil {
		http.Error(w, "Internal error: nil response", http.StatusInternalServerError)
		return
	}

	if result.Error != nil {
		http.Error(w, result.Error.Error(), http.StatusInternalServerError)
		return
	}

	response := CompletionResponse{
		ID:      "cmpl-" + fmt.Sprintf("%d", time.Now().Unix()),
		Object:  "text_completion",
		Created: time.Now().Unix(),
		Choices: []Choice{
			{
				Index: 0,
				Message: ChatMessage{
					Content: result.Content,
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			TotalTokens: len(req.Prompt) / 4,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type HealthResponse struct {
	Status string `json:"status"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(HealthResponse{Status: "ok"})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := s.scheduler.GetStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

type ModelList struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	OwnedBy string `json:"owned_by"`
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	modelList := ModelList{
		Object: "list",
		Data: []Model{
			{
				ID:      "llama-7b",
				Object:  "model",
				Created: 1677610602,
				OwnedBy: "meta",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(modelList)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	HandshakeTimeout: 10 * time.Second,
}

func (s *Server) handleWebSocketChat(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var req ChatRequest
		if err := json.Unmarshal(msg, &req); err != nil {
			conn.WriteJSON(map[string]string{"error": err.Error()})
			continue
		}

		maxTokens := req.MaxTokens
		if maxTokens == 0 {
			maxTokens = 512
		}
		temp := req.Temperature
		if temp == 0 {
			temp = 0.7
		}

		prompt := ""
		for _, msg := range req.Messages {
			prompt += msg.Content + "\n"
		}

		inst := s.scheduler.GetFreeInstance()
		if inst == nil {
			conn.WriteJSON(map[string]string{"error": "no available instances"})
			continue
		}

		err = inst.CompleteStream(prompt, maxTokens, temp, func(token string) {
			conn.WriteJSON(map[string]string{
				"content": token,
				"type":    "token",
			})
		})

		s.scheduler.ReleaseInstance(inst)

		if err != nil {
			conn.WriteJSON(map[string]string{"error": err.Error()})
		} else {
			conn.WriteJSON(map[string]string{"type": "done"})
		}
	}
}

func (s *Server) handleChatCompletionsStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 512
	}
	temp := req.Temperature
	if temp == 0 {
		temp = 0.7
	}

	prompt := ""
	for _, msg := range req.Messages {
		prompt += msg.Content + "\n"
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	inst := s.scheduler.GetFreeInstance()
	if inst == nil {
		fmt.Fprintf(w, "data: {\"error\":\"no available instances\"}\n\n")
		flusher.Flush()
		return
	}

	err := inst.CompleteStream(prompt, maxTokens, temp, func(token string) {
		escaped := strings.ReplaceAll(token, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		escaped = strings.ReplaceAll(escaped, "\r", "\\r")
		escaped = strings.ReplaceAll(escaped, "\t", "\\t")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\n\n", escaped)
		flusher.Flush()
	})

	s.scheduler.ReleaseInstance(inst)

	if err != nil {
		errMsg := strings.ReplaceAll(err.Error(), "\\", "\\\\")
		errMsg = strings.ReplaceAll(errMsg, "\"", "\\\"")
		errMsg = strings.ReplaceAll(errMsg, "\n", "\\n")
		errMsg = strings.ReplaceAll(errMsg, "\r", "\\r")
		errMsg = strings.ReplaceAll(errMsg, "\t", "\\t")
		fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", errMsg)
		flusher.Flush()
	} else {
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}
}

func (s *Server) handleCompletionsStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 512
	}
	temp := req.Temperature
	if temp == 0 {
		temp = 0.7
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	inst := s.scheduler.GetFreeInstance()
	if inst == nil {
		fmt.Fprintf(w, "data: {\"error\":\"no available instances\"}\n\n")
		flusher.Flush()
		return
	}

	err := inst.CompleteStream(req.Prompt, maxTokens, temp, func(token string) {
		escaped := strings.ReplaceAll(token, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		escaped = strings.ReplaceAll(escaped, "\r", "\\r")
		escaped = strings.ReplaceAll(escaped, "\t", "\\t")
		fmt.Fprintf(w, "data: {\"choices\":[{\"text\":\"%s\"}]}\n\n", escaped)
		flusher.Flush()
	})

	s.scheduler.ReleaseInstance(inst)

	if err != nil {
		errMsg := strings.ReplaceAll(err.Error(), "\\", "\\\\")
		errMsg = strings.ReplaceAll(errMsg, "\"", "\\\"")
		errMsg = strings.ReplaceAll(errMsg, "\n", "\\n")
		errMsg = strings.ReplaceAll(errMsg, "\r", "\\r")
		errMsg = strings.ReplaceAll(errMsg, "\t", "\\t")
		fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", errMsg)
		flusher.Flush()
	} else {
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}
}

type ImageGenerateRequest struct {
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
}

type ControlNet struct {
	Enabled      bool    `json:"enabled"`
	ImageBase64  string  `json:"image_base64,omitempty"`
	Preprocessor string  `json:"preprocessor,omitempty"`
	Model        string  `json:"model,omitempty"`
	Weight       float64 `json:"weight,omitempty"`
}

type VideoGenerateRequest struct {
	ImageGenerateRequest
	Frames      int     `json:"frames,omitempty"`
	FPS         int     `json:"fps,omitempty"`
	Duration    float64 `json:"duration,omitempty"`
	Loop        bool    `json:"loop,omitempty"`
	AnimateDiff string  `json:"animate_diff,omitempty"`
	MotionLora  string  `json:"motion_lora,omitempty"`
}

type ComposeVideoRequest struct {
	ImagePaths []string `json:"image_paths"`
	AudioPath  string   `json:"audio_path,omitempty"`
	OutputName string   `json:"output_name,omitempty"`
	FPS        int      `json:"fps,omitempty"`
}

type ImageGenerateResponse struct {
	ImagePath string `json:"image_path,omitempty"`
	ImageURL  string `json:"image_url,omitempty"`
	Seed      int64  `json:"seed,omitempty"`
}

type VideoGenerateResponse struct {
	VideoPath  string `json:"video_path,omitempty"`
	VideoURL   string `json:"video_url,omitempty"`
	FrameCount int    `json:"frame_count,omitempty"`
	FPS        int    `json:"fps,omitempty"`
}

func (s *Server) handleImageGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ImageGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	imgReq := multimodal.ImageRequest{
		Prompt:         req.Prompt,
		NegativePrompt: req.NegativePrompt,
		Width:          req.Width,
		Height:         req.Height,
		Steps:          req.Steps,
		CFG:            req.CFG,
		Seed:           req.Seed,
		Model:          req.Model,
		Sampler:        req.Sampler,
	}

	result, err := s.multimodalMgr.GenerateImage(imgReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := ImageGenerateResponse{
		ImagePath: result.ImagePath,
		Seed:      result.Seed,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleVideoGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req VideoGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	videoReq := multimodal.VideoRequest{
		ImageRequest: multimodal.ImageRequest{
			Prompt:         req.Prompt,
			NegativePrompt: req.NegativePrompt,
			Width:          req.Width,
			Height:         req.Height,
			Steps:          req.Steps,
			CFG:            req.CFG,
			Seed:           req.Seed,
			Model:          req.Model,
			Sampler:        req.Sampler,
		},
		Frames:      req.Frames,
		FPS:         req.FPS,
		Duration:    req.Duration,
		Loop:        req.Loop,
		AnimateDiff: req.AnimateDiff,
		MotionLora:  req.MotionLora,
	}

	result, err := s.multimodalMgr.GenerateVideo(videoReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := VideoGenerateResponse{
		VideoPath:  result.VideoPath,
		FrameCount: result.FrameCount,
		FPS:        result.FPS,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleComposeVideo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ComposeVideoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	composeReq := multimodal.ComposeVideoRequest{
		ImagePaths: req.ImagePaths,
		AudioPath:  req.AudioPath,
		OutputName: req.OutputName,
		FPS:        req.FPS,
	}

	result, err := s.multimodalMgr.ComposeVideo(composeReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"video_path": result,
	})
}

func (s *Server) handleUpscaleImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ImagePath string  `json:"image_path"`
		Scale     int     `json:"scale,omitempty"`
		Model     string  `json:"model,omitempty"`
		Denoising float64 `json:"denoising_strength,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	upscaleReq := multimodal.UpscaleRequest{
		ImageBase64: "",
		Scale:       req.Scale,
		Model:       req.Model,
		Denoising:   req.Denoising,
	}

	result, err := s.multimodalMgr.UpscaleImage(upscaleReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"image_path": result.ImagePath,
	})
}
