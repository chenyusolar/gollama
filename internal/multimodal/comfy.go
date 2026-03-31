package multimodal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type ComfyUI struct {
	client  *http.Client
	host    string
	port    int
	baseURL string
}

func NewComfyUI(host string, port int, timeout int) *ComfyUI {
	return &ComfyUI{
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
		host:    host,
		port:    port,
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
	}
}

func (c *ComfyUI) IsReady() bool {
	resp, err := c.client.Get(c.baseURL + "/system_stats")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func (c *ComfyUI) GetQueue() (int, int, error) {
	resp, err := c.client.Get(c.baseURL + "/queue")
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, 0, err
	}

	queueRunning, _ := result["queue_running"].([]interface{})
	queuePending, _ := result["queue_pending"].([]interface{})

	return len(queueRunning), len(queuePending), nil
}

func (c *ComfyUI) QueuePrompt(workflow map[string]interface{}) (string, error) {
	promptBytes, err := json.Marshal(map[string]interface{}{"prompt": workflow})
	if err != nil {
		return "", err
	}

	resp, err := c.client.Post(c.baseURL+"/prompt", "application/json", bytes.NewReader(promptBytes))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	promptID, _ := result["prompt_id"].(string)
	return promptID, nil
}

func (c *ComfyUI) GetHistory(promptID string) (map[string]interface{}, error) {
	resp, err := c.client.Get(c.baseURL + "/history/" + promptID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if data, ok := result[promptID].(map[string]interface{}); ok {
		return data, nil
	}

	return nil, fmt.Errorf("prompt not found")
}

func (c *ComfyUI) WaitForCompletion(promptID string, timeout time.Duration) (map[string]interface{}, error) {
	startTime := time.Now()

	for {
		history, err := c.GetHistory(promptID)
		if err == nil {
			if status, ok := history["status"].(map[string]interface{}); ok {
				if statusStr, ok := status["status_str"].(string); ok {
					if statusStr == "completed" {
						return history, nil
					}
					if statusStr == "error" {
						return nil, fmt.Errorf("comfyui error")
					}
				}
			}
		}

		if time.Since(startTime) > timeout {
			return nil, fmt.Errorf("timeout waiting for completion")
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func (c *ComfyUI) GetOutputImages(history map[string]interface{}) ([]string, error) {
	var outputs []string

	if outputsObj, ok := history["outputs"].(map[string]interface{}); ok {
		for _, nodeOutput := range outputsObj {
			if node, ok := nodeOutput.(map[string]interface{}); ok {
				if images, ok := node["images"].([]interface{}); ok {
					for _, img := range images {
						if imgData, ok := img.(map[string]interface{}); ok {
							filename, _ := imgData["filename"].(string)
							subfolder, _ := imgData["subfolder"].(string)
							if filename == "" {
								continue
							}
							imagePath := ""
							if subfolder != "" {
								imagePath = fmt.Sprintf("output/%s/%s", subfolder, filename)
							} else {
								imagePath = fmt.Sprintf("output/%s", filename)
							}
							outputs = append(outputs, imagePath)
						}
					}
				}
			}
		}
	}

	return outputs, nil
}

func (c *ComfyUI) DownloadImage(imagePath, outputPath string) error {
	resp, err := c.client.Get(c.baseURL + "/view?filename=" + filepath.Base(imagePath))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to download image: %s", resp.Status)
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	return err
}

type WorkflowBuilder struct {
	comfy *ComfyUI
}

func NewWorkflowBuilder(comfy *ComfyUI) *WorkflowBuilder {
	return &WorkflowBuilder{comfy: comfy}
}

func (w *WorkflowBuilder) BuildSimpleImageWorkflow(req ImageRequest) map[string]interface{} {
	workflow := map[string]interface{}{}

	width := req.Width
	if width == 0 {
		width = 512
	}
	height := req.Height
	if height == 0 {
		height = 512
	}
	seed := req.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	steps := req.Steps
	if steps == 0 {
		steps = 20
	}
	cfg := req.CFG
	if cfg == 0 {
		cfg = 7.0
	}
	negative := req.NegativePrompt
	if negative == "" {
		negative = "low quality, blurry, distorted"
	}
	sampler := req.Sampler
	if sampler == "" {
		sampler = "euler"
	}

	workflow["1"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"ckpt_name": req.Model,
		},
		"class_type": "CheckpointLoaderSimple",
	}

	workflow["2"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"text": req.Prompt,
			"clip": []interface{}{"1", []interface{}{1}},
		},
		"class_type": "CLIPTextEncode",
	}

	workflow["3"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"text": negative,
			"clip": []interface{}{"1", []interface{}{1}},
		},
		"class_type": "CLIPTextEncode",
	}

	workflow["4"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"width":      width,
			"height":     height,
			"batch_size": 1,
		},
		"class_type": "EmptyLatentImage",
	}

	workflow["5"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"seed":         seed,
			"steps":        steps,
			"cfg":          cfg,
			"sampler_name": sampler,
			"scheduler":    "normal",
			"positive":     []interface{}{"2", []interface{}{0}},
			"negative":     []interface{}{"3", []interface{}{0}},
			"latent_image": []interface{}{"4", []interface{}{0}},
			"denoise":      1.0,
		},
		"class_type": "KSampler",
	}

	workflow["6"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"samples": []interface{}{"5", []interface{}{0}},
			"vae":     []interface{}{"1", []interface{}{2}},
		},
		"class_type": "VAEDecode",
	}

	workflow["7"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"images":          []interface{}{"6", []interface{}{0}},
			"filename_prefix": "output",
		},
		"class_type": "SaveImage",
	}

	return workflow
}

func (w *WorkflowBuilder) BuildControlNetWorkflow(req ImageRequest) map[string]interface{} {
	workflow := w.BuildSimpleImageWorkflow(req)

	workflow["8"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"image":  req.ControlNets[0].ImageBase64,
			"upload": "image",
		},
		"class_type": "LoadImage",
	}

	workflow["9"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"preprocessor": req.ControlNets[0].Preprocessor,
			"image":        []interface{}{"8", []interface{}{0}},
		},
		"class_type": "ControlNetApply",
	}

	inputs := workflow["5"].(map[string]interface{})["inputs"].(map[string]interface{})
	inputs["control"] = []interface{}{"9", []interface{}{0}}

	return workflow
}

func (w *WorkflowBuilder) BuildUpscaleWorkflow(req UpscaleRequest) map[string]interface{} {
	scale := req.Scale
	if scale == 0 {
		scale = 2
	}

	workflow := map[string]interface{}{}

	workflow["1"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"image":  req.ImageBase64,
			"upload": "image",
		},
		"class_type": "LoadImage",
	}

	workflow["2"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"model_name":       req.Model,
			"scale":            scale,
			"keep_proportions": true,
		},
		"class_type": "UpscaleModel",
	}

	workflow["3"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"model":      []interface{}{"2", []interface{}{0}},
			"image":      []interface{}{"1", []interface{}{0}},
			"blend":      0.0,
			"denoise":    req.Denoising,
			"tile_width": 512,
		},
		"class_type": "ImageUpscaleWithModel",
	}

	workflow["4"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"images":          []interface{}{"3", []interface{}{0}},
			"filename_prefix": "upscaled",
		},
		"class_type": "SaveImage",
	}

	return workflow
}

func (w *WorkflowBuilder) BuildVideoWorkflow(req VideoRequest) map[string]interface{} {
	workflow := map[string]interface{}{}

	width := req.Width
	if width == 0 {
		width = 512
	}
	height := req.Height
	if height == 0 {
		height = 512
	}
	frames := req.Frames
	if frames == 0 {
		frames = 24
	}
	fps := req.FPS
	if fps == 0 {
		fps = 8
	}
	seed := req.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	workflow["1"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"ckpt_name": req.Model,
		},
		"class_type": "CheckpointLoaderSimple",
	}

	workflow["2"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"text": req.Prompt,
			"clip": []interface{}{"1", []interface{}{1}},
		},
		"class_type": "CLIPTextEncode",
	}

	negative := req.NegativePrompt
	if negative == "" {
		negative = "low quality, blurry, distorted"
	}
	workflow["3"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"text": negative,
			"clip": []interface{}{"1", []interface{}{1}},
		},
		"class_type": "CLIPTextEncode",
	}

	workflow["4"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"width":      width,
			"height":     height,
			"batch_size": frames,
		},
		"class_type": "EmptyLatentImage",
	}

	workflow["5"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"model":        []interface{}{"1", []interface{}{0}},
			"clip":         []interface{}{"1", []interface{}{1}},
			"vae":          []interface{}{"1", []interface{}{2}},
			"latent_image": []interface{}{"4", []interface{}{0}},
			"seed":         seed,
			"steps":        20,
			"cfg":          7.0,
			"sampler_name": "euler",
			"scheduler":    "normal",
			"positive":     []interface{}{"2", []interface{}{0}},
			"negative":     []interface{}{"3", []interface{}{0}},
			"denoise":      1.0,
		},
		"class_type": "KSampler",
	}

	workflow["6"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"samples": []interface{}{"5", []interface{}{0}},
			"vae":     []interface{}{"1", []interface{}{2}},
		},
		"class_type": "VAEDecode",
	}

	workflow["7"] = map[string]interface{}{
		"inputs": map[string]interface{}{
			"images":          []interface{}{"6", []interface{}{0}},
			"filename_prefix": "video",
			"format":          "GIF",
			"loop":            0,
		},
		"class_type": "GIF",
	}

	return workflow
}
