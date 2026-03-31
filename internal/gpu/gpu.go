package gpu

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type GPU struct {
	ID            int
	Name          string
	MemoryTotal   int64
	MemoryUsed    int64
	MemoryFree    int64
	Utilization   int
	Temperature   int
	DriverVersion string
	CUDAVersion   string
}

type GPUManager struct {
	gpus           []*GPU
	mu             sync.RWMutex
	interval       time.Duration
	stopCh         chan struct{}
	configMemoryGB int
}

func NewGPUManager(configMemoryGB int) *GPUManager {
	return &GPUManager{
		gpus:           make([]*GPU, 0),
		interval:       5 * time.Second,
		stopCh:         make(chan struct{}),
		configMemoryGB: configMemoryGB,
	}
}

func (g *GPUManager) DetectGPUs() ([]*GPU, error) {
	if runtime.GOOS == "windows" {
		return g.detectWindows()
	}
	return g.detectLinux()
}

func (g *GPUManager) detectWindows() ([]*GPU, error) {
	var output []byte
	var err error
	var usedPath string

	// Use 'where' command to find nvidia-smi, then try each path
	cmd := exec.Command("where", "nvidia-smi")
	whereOutput, whereErr := cmd.Output()
	if whereErr == nil && len(whereOutput) > 0 {
		paths := strings.Split(strings.TrimSpace(string(whereOutput)), "\n")
		for _, nvidiaSmiPath := range paths {
			nvidiaSmiPath = strings.TrimSpace(nvidiaSmiPath)
			if nvidiaSmiPath == "" {
				continue
			}
			cmd := exec.Command(nvidiaSmiPath, "--query-gpu=index,name,memory.total,memory.used,memory.free,utilization.gpu,temperature.gpu,driver_version,cuda_version", "--format=csv,noheader,nounits")
			output, err = cmd.Output()
			if err == nil && len(output) > 0 {
				usedPath = nvidiaSmiPath
				log.Printf("Using nvidia-smi from: %s", usedPath)
				break
			}
		}
	}

	// Fallback to standard paths if 'where' command fails
	if err != nil || len(output) == 0 {
		nvidiaSmiPaths := []string{
			"nvidia-smi.exe",
			"C:\\Windows\\System32\\nvidia-smi.exe",
			"C:\\Program Files\\NVIDIA Corporation\\NVSMI\\nvidia-smi.exe",
		}

		for _, path := range nvidiaSmiPaths {
			cmd := exec.Command(path, "--query-gpu=index,name,memory.total,memory.used,memory.free,utilization.gpu,temperature.gpu,driver_version,cuda_version", "--format=csv,noheader,nounits")
			output, err = cmd.Output()
			if err == nil && len(output) > 0 {
				usedPath = path
				log.Printf("Using nvidia-smi from: %s", usedPath)
				break
			} else if err != nil {
				// Log the error but continue trying other paths
				log.Printf("Failed to execute nvidia-smi from %s: %v", path, err)
			}
		}
	}

	// Try nvidia-smi from CUDA installation paths
	if err != nil || len(output) == 0 {
		cudaPaths := []string{
			"C:\\Program Files\\NVIDIA GPU Computing Toolkit\\CUDA\\v12.6\\bin",
			"C:\\Program Files\\NVIDIA GPU Computing Toolkit\\CUDA\\v12.5\\bin",
			"C:\\Program Files\\NVIDIA GPU Computing Toolkit\\CUDA\\v12.4\\bin",
			"C:\\Program Files\\NVIDIA GPU Computing Toolkit\\CUDA\\v12.3\\bin",
			"C:\\Program Files\\NVIDIA GPU Computing Toolkit\\CUDA\\v12.2\\bin",
			"C:\\Program Files\\NVIDIA GPU Computing Toolkit\\CUDA\\v12.1\\bin",
			"C:\\Program Files\\NVIDIA GPU Computing Toolkit\\CUDA\\v12.0\\bin",
			"C:\\Program Files\\NVIDIA GPU Computing Toolkit\\CUDA\\v11.8\\bin",
		}

		for _, cudaPath := range cudaPaths {
			fullPath := cudaPath + "\\nvidia-smi.exe"
			cmd := exec.Command(fullPath, "--query-gpu=index,name,memory.total,memory.used,memory.free,utilization.gpu,temperature.gpu,driver_version,cuda_version", "--format=csv,noheader,nounits")
			output, err = cmd.Output()
			if err == nil && len(output) > 0 {
				usedPath = fullPath
				log.Printf("Using nvidia-smi from: %s", usedPath)
				break
			} else if err != nil {
				log.Printf("Failed to execute nvidia-smi from CUDA path %s: %v", fullPath, err)
			}
		}
	}

	if err != nil || len(output) == 0 {
		log.Printf("nvidia-smi not available: %v", err)

		// Try WMIC as fallback
		cmd := exec.Command("wmic", "path", "win32_VideoController", "get", "name,adapterram", "/format:csv")
		output, err = cmd.Output()
		if err == nil && len(output) > 0 {
			lines := strings.Split(string(output), "\n")
			var gpus []*GPU
			for i, line := range lines {
				if i == 0 || strings.TrimSpace(line) == "" {
					continue
				}
				fields := strings.Split(line, ",")
				if len(fields) >= 3 {
					ram := strings.TrimSpace(fields[2])
					ram = strings.ReplaceAll(ram, " ", "")
					memBytes, _ := strconv.ParseInt(ram, 10, 64)
					memMB := memBytes / (1024 * 1024)

					if strings.Contains(fields[1], "NVIDIA") {
						gpus = append(gpus, &GPU{
							ID:          len(gpus),
							Name:        strings.TrimSpace(fields[1]),
							MemoryTotal: memMB,
							MemoryUsed:  0,
							MemoryFree:  memMB,
						})
					}
				}
			}
			if len(gpus) > 0 {
				log.Printf("Detected %d GPU(s) via WMI", len(gpus))
				return gpus, nil
			}
		}

		// Use config file memory value as last resort
		defaultMemory := int64(8 * 1024) // 8GB default
		if g.configMemoryGB > 0 {
			defaultMemory = int64(g.configMemoryGB * 1024)
			log.Printf("Using config file GPU memory setting: %dGB", g.configMemoryGB)
		} else {
			log.Printf("Using default GPU (8GB)")
		}

		return []*GPU{
			{
				ID:            0,
				Name:          "NVIDIA GPU",
				MemoryTotal:   defaultMemory,
				MemoryUsed:    0,
				MemoryFree:    defaultMemory,
				Utilization:   0,
				Temperature:   45,
				DriverVersion: "Unknown",
				CUDAVersion:   "Unknown",
			},
		}, nil
	}

	var gpus []*GPU
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Split(line, ",")
		if len(fields) < 9 {
			continue
		}

		id, _ := strconv.Atoi(strings.TrimSpace(fields[0]))
		memTotal, _ := strconv.ParseInt(strings.TrimSpace(fields[2]), 10, 64)
		memUsed, _ := strconv.ParseInt(strings.TrimSpace(fields[3]), 10, 64)
		memFree, _ := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64)
		util, _ := strconv.Atoi(strings.TrimSpace(fields[5]))
		temp, _ := strconv.Atoi(strings.TrimSpace(fields[6]))

		gpu := &GPU{
			ID:            id,
			Name:          strings.TrimSpace(fields[1]),
			MemoryTotal:   memTotal,
			MemoryUsed:    memUsed,
			MemoryFree:    memFree,
			Utilization:   util,
			Temperature:   temp,
			DriverVersion: strings.TrimSpace(fields[7]),
			CUDAVersion:   strings.TrimSpace(fields[8]),
		}
		gpus = append(gpus, gpu)
	}

	return gpus, nil
}

func (g *GPUManager) detectLinux() ([]*GPU, error) {
	var output []byte
	var err error
	var usedPath string

	// Use 'which' command to find nvidia-smi
	cmd := exec.Command("which", "nvidia-smi")
	whichOutput, whichErr := cmd.Output()
	if whichErr == nil && len(whichOutput) > 0 {
		nvidiaSmiPath := strings.TrimSpace(strings.Split(string(whichOutput), "\n")[0])
		cmd = exec.Command(nvidiaSmiPath, "--query-gpu=index,name,memory.total,memory.used,memory.free,utilization.gpu,temperature.gpu,driver_version,cuda_version", "--format=csv,noheader,nounits")
		output, err = cmd.Output()
		if err == nil && len(output) > 0 {
			usedPath = nvidiaSmiPath
			log.Printf("Using nvidia-smi from: %s", usedPath)
		}
	}

	// Fallback to direct command
	if err != nil || len(output) == 0 {
		cmd = exec.Command("nvidia-smi", "--query-gpu=index,name,memory.total,memory.used,memory.free,utilization.gpu,temperature.gpu,driver_version,cuda_version", "--format=csv,noheader,nounits")
		output, err = cmd.Output()
		if err == nil && len(output) > 0 {
			usedPath = "nvidia-smi"
			log.Printf("Using nvidia-smi from: %s", usedPath)
		}
	}

	if err != nil || len(output) == 0 {
		log.Printf("nvidia-smi not available: %v", err)

		// Use config file memory value as last resort
		defaultMemory := int64(8 * 1024) // 8GB default
		if g.configMemoryGB > 0 {
			defaultMemory = int64(g.configMemoryGB * 1024)
			log.Printf("Using config file GPU memory setting: %dGB", g.configMemoryGB)
		} else {
			log.Printf("Using default GPU (8GB)")
		}

		return []*GPU{
			{
				ID:            0,
				Name:          "NVIDIA GPU",
				MemoryTotal:   defaultMemory,
				MemoryUsed:    0,
				MemoryFree:    defaultMemory,
				Utilization:   0,
				Temperature:   45,
				DriverVersion: "Unknown",
				CUDAVersion:   "Unknown",
			},
		}, nil
	}

	var gpus []*GPU
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Split(line, ",")
		if len(fields) < 9 {
			continue
		}

		id, _ := strconv.Atoi(strings.TrimSpace(fields[0]))
		memTotal, _ := strconv.ParseInt(strings.TrimSpace(fields[2]), 10, 64)
		memUsed, _ := strconv.ParseInt(strings.TrimSpace(fields[3]), 10, 64)
		memFree, _ := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64)
		util, _ := strconv.Atoi(strings.TrimSpace(fields[5]))
		temp, _ := strconv.Atoi(strings.TrimSpace(fields[6]))

		gpu := &GPU{
			ID:            id,
			Name:          strings.TrimSpace(fields[1]),
			MemoryTotal:   memTotal,
			MemoryUsed:    memUsed,
			MemoryFree:    memFree,
			Utilization:   util,
			Temperature:   temp,
			DriverVersion: strings.TrimSpace(fields[7]),
			CUDAVersion:   strings.TrimSpace(fields[8]),
		}
		gpus = append(gpus, gpu)
	}

	return gpus, nil
}

func (g *GPUManager) Start() error {
	gpus, err := g.DetectGPUs()
	if err != nil {
		return fmt.Errorf("failed to detect GPUs: %w", err)
	}

	g.mu.Lock()
	g.gpus = gpus
	g.mu.Unlock()

	log.Printf("Detected %d GPU(s):", len(gpus))
	for _, gpu := range gpus {
		memGB := float64(gpu.MemoryTotal) / 1024
		memUsedGB := float64(gpu.MemoryUsed) / 1024
		if memGB >= 1000 {
			memGB /= 1024
			memUsedGB /= 1024
		}
		log.Printf("  GPU %d: %s (%.0fGB, %.0fGB used)",
			gpu.ID, gpu.Name, memGB, memUsedGB)
	}

	go g.monitor()

	return nil
}

func (g *GPUManager) Stop() {
	g.mu.Lock()
	defer g.mu.Unlock()

	select {
	case <-g.stopCh:
		// Already closed
	default:
		close(g.stopCh)
	}
}

func (g *GPUManager) monitor() {
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()

	for {
		select {
		case <-g.stopCh:
			return
		case <-ticker.C:
			g.refresh()
		}
	}
}

func (g *GPUManager) refresh() {
	gpus := g.detectGPUSilent()

	g.mu.Lock()
	g.gpus = gpus
	g.mu.Unlock()
}

func (g *GPUManager) detectGPUSilent() []*GPU {
	var output []byte
	var err error

	// Try to find nvidia-smi using OS-appropriate command
	if runtime.GOOS == "windows" {
		cmd := exec.Command("where", "nvidia-smi")
		whereOutput, whereErr := cmd.Output()
		if whereErr == nil && len(whereOutput) > 0 {
			paths := strings.Split(strings.TrimSpace(string(whereOutput)), "\n")
			for _, nvidiaSmiPath := range paths {
				nvidiaSmiPath = strings.TrimSpace(nvidiaSmiPath)
				if nvidiaSmiPath == "" {
					continue
				}
				cmd = exec.Command(nvidiaSmiPath, "--query-gpu=index,name,memory.total,memory.used,memory.free", "--format=csv,noheader,nounits")
				output, err = cmd.Output()
				if err == nil && len(output) > 0 {
					return g.parseNvidiaSmiOutput(output)
				}
			}
		}
	} else {
		cmd := exec.Command("which", "nvidia-smi")
		whichOutput, whichErr := cmd.Output()
		if whichErr == nil && len(whichOutput) > 0 {
			paths := strings.Split(strings.TrimSpace(string(whichOutput)), "\n")
			for _, nvidiaSmiPath := range paths {
				nvidiaSmiPath = strings.TrimSpace(nvidiaSmiPath)
				if nvidiaSmiPath == "" {
					continue
				}
				cmd = exec.Command(nvidiaSmiPath, "--query-gpu=index,name,memory.total,memory.used,memory.free", "--format=csv,noheader,nounits")
				output, err = cmd.Output()
				if err == nil && len(output) > 0 {
					return g.parseNvidiaSmiOutput(output)
				}
			}
		}
	}

	// Fallback to direct command
	cmd := exec.Command("nvidia-smi", "--query-gpu=index,name,memory.total,memory.used,memory.free", "--format=csv,noheader,nounits")
	output, err = cmd.Output()
	if err == nil && len(output) > 0 {
		return g.parseNvidiaSmiOutput(output)
	}

	// Return cached GPUs if all detection methods fail
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.gpus
}

func (g *GPUManager) parseNvidiaSmiOutput(output []byte) []*GPU {
	var gpus []*GPU
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 5 {
			continue
		}
		id, _ := strconv.Atoi(strings.TrimSpace(fields[0]))
		memTotal, _ := strconv.ParseInt(strings.TrimSpace(fields[2]), 10, 64)
		memUsed, _ := strconv.ParseInt(strings.TrimSpace(fields[3]), 10, 64)
		memFree, _ := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64)

		gpus = append(gpus, &GPU{
			ID:          id,
			Name:        strings.TrimSpace(fields[1]),
			MemoryTotal: memTotal,
			MemoryUsed:  memUsed,
			MemoryFree:  memFree,
		})
	}
	return gpus
}

func (g *GPUManager) GetGPU(id int) *GPU {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, gpu := range g.gpus {
		if gpu.ID == id {
			return gpu
		}
	}
	return nil
}

func (g *GPUManager) GetAll() []*GPU {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]*GPU, len(g.gpus))
	copy(result, g.gpus)
	return result
}

func (g *GPUManager) GetFreeGPU(minMemory int64) *GPU {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var bestGPU *GPU
	var maxFree int64

	for _, gpu := range g.gpus {
		if gpu.MemoryFree >= minMemory {
			if gpu.MemoryFree > maxFree {
				maxFree = gpu.MemoryFree
				bestGPU = gpu
			}
		}
	}

	return bestGPU
}

func (g *GPUManager) GetLeastUsedGPU() *GPU {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var bestGPU *GPU
	minUtil := 100

	for _, gpu := range g.gpus {
		if gpu.Utilization < minUtil {
			minUtil = gpu.Utilization
			bestGPU = gpu
		}
	}

	return bestGPU
}

func (g *GPUManager) GetCoolestGPU(maxTemp int) *GPU {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var bestGPU *GPU
	minTemp := 100

	for _, gpu := range g.gpus {
		if gpu.Temperature < minTemp && gpu.Temperature < maxTemp {
			minTemp = gpu.Temperature
			bestGPU = gpu
		}
	}

	return bestGPU
}

func (g *GPUManager) Count() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.gpus)
}

func (g *GPUManager) GetStats() GPUStats {
	g.mu.RLock()
	defer g.mu.RUnlock()

	stats := GPUStats{
		Count: len(g.gpus),
		GPUs:  make([]GPUInfo, 0, len(g.gpus)),
	}

	var totalMem, usedMem int64
	for _, gpu := range g.gpus {
		stats.GPUs = append(stats.GPUs, GPUInfo{
			ID:          gpu.ID,
			Name:        gpu.Name,
			MemoryTotal: gpu.MemoryTotal,
			MemoryUsed:  gpu.MemoryUsed,
			MemoryFree:  gpu.MemoryFree,
			Utilization: gpu.Utilization,
			Temperature: gpu.Temperature,
		})
		totalMem += gpu.MemoryTotal
		usedMem += gpu.MemoryUsed
	}

	stats.TotalMemory = totalMem
	stats.UsedMemory = usedMem
	stats.FreeMemory = totalMem - usedMem

	return stats
}

type GPUStats struct {
	Count       int       `json:"count"`
	TotalMemory int64     `json:"total_memory_mb"`
	UsedMemory  int64     `json:"used_memory_mb"`
	FreeMemory  int64     `json:"free_memory_mb"`
	GPUs        []GPUInfo `json:"gpus"`
}

type GPUInfo struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	MemoryTotal int64  `json:"memory_total_mb"`
	MemoryUsed  int64  `json:"memory_used_mb"`
	MemoryFree  int64  `json:"memory_free_mb"`
	Utilization int    `json:"utilization_percent"`
	Temperature int    `json:"temperature_celsius"`
}
