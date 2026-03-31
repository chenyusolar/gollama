package runner

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/anomalyco/myllama.cpp/internal/config"
)

type LlamaInstance struct {
	ID          int
	Port        int
	GPUID       int
	Model       string
	Cmd         *exec.Cmd
	Stdin       io.WriteCloser
	Stdout      io.Reader
	Busy        bool
	Mu          sync.Mutex
	Ready       bool
	StopCh      chan struct{}
	KVCachePath string
	CacheMgr    *KVCacheManager
	DoneCh      chan struct{}
}

type LlamaRunner struct {
	cfg       *config.LlamaConfig
	instances []*LlamaInstance
}

type CompletionRequest struct {
	Prompt string  `json:"prompt"`
	Temp   float64 `json:"temperature,omitempty"`
	Tokens int     `json:"max_tokens,omitempty"`
	Stream bool    `json:"stream,omitempty"`
}

type CompletionResponse struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Text         string `json:"text"`
	Index        int    `json:"index"`
	FinishReason string `json:"finish_reason"`
}

func NewLlamaRunner(cfg *config.LlamaConfig) *LlamaRunner {
	return &LlamaRunner{
		cfg: cfg,
	}
}

func (r *LlamaRunner) StartInstance(id, port int, gpuID int) (*LlamaInstance, error) {
	log.Printf("Starting llama.cpp instance %d on port %d (GPU %d)...", id, port, gpuID)
	log.Printf("  Binary: %s", r.cfg.Binary)
	log.Printf("  Model: %s", r.cfg.Model)

	cmd := exec.Command(
		r.cfg.Binary,
		"-m", r.cfg.Model,
		"-ngl", fmt.Sprintf("%d", r.cfg.GpuLayers),
		"-c", fmt.Sprintf("%d", r.cfg.ContextSize),
		"-b", fmt.Sprintf("%d", r.cfg.PromptBatch),
		"--port", fmt.Sprintf("%d", port),
		"--parallel", "1",
	)

	if gpuID >= 0 {
		cmd.Env = append(os.Environ(), fmt.Sprintf("CUDA_VISIBLE_DEVICES=%d", gpuID))
		log.Printf("  CUDA_VISIBLE_DEVICES=%d", gpuID)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start llama-server: %w", err)
	}

	inst := &LlamaInstance{
		ID:     id,
		Port:   port,
		Cmd:    cmd,
		Stdin:  stdin,
		Stdout: stdout,
		Busy:   false,
		Ready:  false,
		StopCh: make(chan struct{}),
		DoneCh: make(chan struct{}),
	}

	go inst.waitForReady()

	return inst, nil
}

func (i *LlamaInstance) waitForReady() {
	timeout := 120 * time.Second
	start := time.Now()

	for {
		if i.Cmd.Process != nil {
			ps := i.Cmd.ProcessState
			if ps != nil && ps.Exited() {
				log.Printf("Instance %d process exited with code: %d", i.ID, ps.ExitCode())
				return
			}
		}

		url := fmt.Sprintf("http://localhost:%d/health", i.Port)
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				i.Ready = true
				log.Printf("Instance %d is ready (via health check)", i.ID)
				select {
				case <-i.DoneCh:
				default:
					close(i.DoneCh)
				}
				return
			}
		}

		if time.Since(start) > timeout {
			log.Printf("Instance %d ready check timeout after %v", i.ID, timeout)
			select {
			case <-i.DoneCh:
			default:
				close(i.DoneCh)
			}
			return
		}

		select {
		case <-i.StopCh:
			select {
			case <-i.DoneCh:
			default:
				close(i.DoneCh)
			}
			return
		default:
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func (i *LlamaInstance) IsHealthy() bool {
	if !i.Ready || i.Cmd.Process == nil {
		return false
	}

	url := fmt.Sprintf("http://localhost:%d/health", i.Port)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func (i *LlamaInstance) Complete(prompt string, maxTokens int, temperature float64) (string, error) {
	i.Mu.Lock()
	defer i.Mu.Unlock()

	if !i.Ready {
		return "", fmt.Errorf("instance not ready")
	}

	log.Printf("[Instance %d] Complete request: prompt=%s, maxTokens=%d, temp=%f", i.ID, prompt, maxTokens, temperature)

	cachePath := ""
	if i.CacheMgr != nil {
		if cachedPath, ok := i.CacheMgr.Get(prompt, i.Model); ok {
			cachePath = cachedPath
		}
	}

	url := fmt.Sprintf("http://localhost:%d/v1/completions", i.Port)

	reqBody := CompletionRequest{
		Prompt: prompt,
		Temp:   temperature,
		Tokens: maxTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	log.Printf("[Instance %d] Sending request to %s", i.ID, url)

	// Use longer timeout for large requests (30 minutes)
	client := &http.Client{
		Timeout: 30 * time.Minute,
		// Keep connection alive for streaming
		Transport: &http.Transport{
			DisableKeepAlives: false,
		},
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		log.Printf("[Instance %d] HTTP request failed: %v", i.ID, err)
		return "", err
	}
	defer resp.Body.Close()

	log.Printf("[Instance %d] Response status: %s", i.ID, resp.Status)

	bodyBytes, _ := io.ReadAll(resp.Body)
	log.Printf("[Instance %d] Response body: %s", i.ID, string(bodyBytes))

	var result CompletionResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		log.Printf("[Instance %d] JSON decode failed: %v", i.ID, err)
		return "", err
	}

	var content string
	if len(result.Choices) > 0 {
		content = result.Choices[0].Text
	}

	if i.CacheMgr != nil && cachePath == "" && content != "" {
		cachePath = i.CacheMgr.GetCachePath(prompt, i.Model)
		if _, err := os.Stat(cachePath); err == nil {
			i.CacheMgr.Set(prompt, i.Model, cachePath)
		}
	}

	log.Printf("[Instance %d] Completion result: %s", i.ID, content)
	return content, nil
}

func (i *LlamaInstance) CompleteStream(prompt string, maxTokens int, temperature float64, onToken func(string)) error {
	i.Mu.Lock()
	defer i.Mu.Unlock()

	if !i.Ready {
		return fmt.Errorf("instance not ready")
	}

	url := fmt.Sprintf("http://localhost:%d/v1/completions", i.Port)

	reqBody := CompletionRequest{
		Prompt: prompt,
		Temp:   temperature,
		Tokens: maxTokens,
		Stream: true,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	log.Printf("[Instance %d] CompleteStream: sending request to %s", i.ID, url)
	log.Printf("[Instance %d] CompleteStream: request body: %s", i.ID, string(jsonBody))

	client := &http.Client{
		Timeout: 30 * time.Minute,
		Transport: &http.Transport{
			DisableKeepAlives: false,
		},
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	log.Printf("[Instance %d] CompleteStream: reading response", i.ID)

	// Read SSE response line by line
	scanner := newLineScanner(resp.Body)
	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineCount++

		log.Printf("[Instance %d] CompleteStream: line %d: %s", i.ID, lineCount, line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Parse SSE format: "data: {...}"
		if len(line) > 6 && line[:6] == "data: " {
			jsonData := line[6:]

			// Check for [DONE]
			if jsonData == "[DONE]" {
				log.Printf("[Instance %d] CompleteStream: received [DONE]", i.ID)
				break
			}

			// Parse JSON
			var result CompletionResponse
			if err := json.Unmarshal([]byte(jsonData), &result); err != nil {
				log.Printf("[Instance %d] JSON decode error: %v, data: %s", i.ID, err, jsonData)
				continue
			}

			// Call callback with token
			if len(result.Choices) > 0 && result.Choices[0].Text != "" {
				log.Printf("[Instance %d] CompleteStream: calling onToken with: %s", i.ID, result.Choices[0].Text)
				onToken(result.Choices[0].Text)
			} else {
				log.Printf("[Instance %d] CompleteStream: empty text in choices", i.ID)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[Instance %d] CompleteStream: scanner error: %v", i.ID, err)
		return err
	}

	log.Printf("[Instance %d] CompleteStream: finished, total lines: %d", i.ID, lineCount)
	return nil
}

// Helper to scan lines from response body
func newLineScanner(r io.Reader) *lineScanner {
	return &lineScanner{scanner: bufio.NewScanner(r)}
}

type lineScanner struct {
	scanner *bufio.Scanner
}

func (ls *lineScanner) Scan() bool {
	return ls.scanner.Scan()
}

func (ls *lineScanner) Text() string {
	return ls.scanner.Text()
}

func (ls *lineScanner) Err() error {
	return ls.scanner.Err()
}

func (i *LlamaInstance) Stop() {
	close(i.StopCh)
	select {
	case <-i.DoneCh:
	case <-time.After(5 * time.Second):
	}
	if i.Cmd != nil && i.Cmd.Process != nil {
		i.Cmd.Process.Kill()
		i.Cmd.Wait()
	}
	if i.Stdin != nil {
		i.Stdin.Close()
	}
}

func (r *LlamaRunner) StartAll(count int, basePort int, gpuIDs []int) ([]*LlamaInstance, error) {
	r.instances = make([]*LlamaInstance, 0, count)

	for i := range count {
		port := basePort + i
		gpuID := -1
		if gpuIDs != nil && i < len(gpuIDs) {
			gpuID = gpuIDs[i]
		}
		inst, err := r.StartInstance(i, port, gpuID)
		if err != nil {
			for _, inst := range r.instances {
				inst.Stop()
			}
			return nil, fmt.Errorf("failed to start instance %d: %w", i, err)
		}

		time.Sleep(500 * time.Millisecond)

		r.instances = append(r.instances, inst)
	}

	log.Printf("Waiting for instances to be ready...")
	for _, inst := range r.instances {
		timeout := 120 * time.Second
		start := time.Now()
		for !inst.Ready {
			if time.Since(start) > timeout {
				log.Printf("Instance %d failed to start within %v", inst.ID, timeout)
				return nil, fmt.Errorf("instance %d failed to start within %v", inst.ID, timeout)
			}
			time.Sleep(100 * time.Millisecond)
		}
		log.Printf("Instance %d is ready on port %d", inst.ID, inst.Port)
	}

	return r.instances, nil
}

func (r *LlamaRunner) StopAll() {
	for _, inst := range r.instances {
		inst.Stop()
	}
}
