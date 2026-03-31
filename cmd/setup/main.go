package main

import (
	"archive/zip"
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

var (
	llamaCppURL string
	ffmpegURL   string
	projectDir  string
)

type SystemInfo struct {
	OS           string
	Arch         string
	GPUName      string
	GPUMemory    int64
	HasNvidiaSMI bool
}

type Dependency struct {
	Name        string
	Installed   bool
	Path        string
	Version     string
	CUDAVersion string
}

func main() {
	flag.StringVar(&projectDir, "dir", ".", "Project directory")
	flag.Parse()

	fmt.Println("========================================")
	fmt.Println("  llama-go-server Setup Installer")
	fmt.Println("========================================")
	fmt.Println()

	projectDir, _ = filepath.Abs(projectDir)
	os.Chdir(projectDir)

	// Step 1: Check and install nvidia-smi if needed
	fmt.Println("[1/5] Checking NVIDIA Driver...")
	nvidiaDep := checkNvidiaSMI()
	printDependency(nvidiaDep)

	// Step 2: Detect GPU
	fmt.Println("[2/5] Checking GPU...")
	gpuInfo := detectSystem()
	printSystemInfo(gpuInfo)

	// Step 3: Check CUDA version
	cudaVersion := ""
	if gpuInfo.HasNvidiaSMI && nvidiaDep.Installed {
		cudaVersion = nvidiaDep.CUDAVersion
		if cudaVersion != "" {
			fmt.Printf("  CUDA Version: %s\n", cudaVersion)
		} else {
			fmt.Println("  CUDA version not detected, will use default")
		}
	}

	// Step 4: Check and install llama.cpp
	fmt.Println("[3/5] Checking llama.cpp...")
	llamaDep := checkLlamaCpp()
	printDependency(llamaDep)

	// Step 5: Check ComfyUI and FFmpeg
	fmt.Println("[4/5] Checking ComfyUI...")
	comfyDep := checkComfyUI()
	printDependency(comfyDep)

	fmt.Println("[5/5] Checking FFmpeg...")
	ffmpegDep := checkFFmpeg()
	printDependency(ffmpegDep)

	// Install missing dependencies
	needsInstall := !llamaDep.Installed || !ffmpegDep.Installed || !comfyDep.Installed

	if needsInstall {
		fmt.Println()
		fmt.Print("Download and install missing dependencies? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		if input == "y" || input == "yes" {
			if !llamaDep.Installed {
				downloadLlamaCpp(gpuInfo, cudaVersion)
			}
			if !ffmpegDep.Installed {
				downloadFFmpeg()
			}
			if !comfyDep.Installed {
				downloadComfyUI()
			}
		}
	}

	// Create configuration
	fmt.Println()
	fmt.Println("[Final] Creating configuration...")
	createConfigFile(gpuInfo)

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  Setup Complete!")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("To start the server, run:")
	fmt.Println("  .\\bin\\llama-go-server.exe")
}

func detectSystem() SystemInfo {
	info := SystemInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	// Try to get GPU info via nvidia-smi first
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		if len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
			parts := strings.Split(strings.TrimSpace(lines[0]), ",")
			if len(parts) >= 1 {
				info.HasNvidiaSMI = true
				info.GPUName = strings.TrimSpace(parts[0])
			}
			if len(parts) >= 2 {
				// Parse memory in MB, convert to GB
				var memMB int64
				fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &memMB)
				if memMB > 0 {
					info.GPUMemory = memMB / 1024
				}
			}
			return info
		}
	}

	// Fallback to WMIC if nvidia-smi is not available
	cmd = exec.Command("wmic", "path", "win32_VideoController", "get", "name", "/value")
	output, _ = cmd.Output()
	if strings.Contains(string(output), "NVIDIA") {
		info.HasNvidiaSMI = true
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Name=") {
				info.GPUName = strings.TrimSpace(strings.Split(line, "=")[1])
			}
			if strings.Contains(line, "AdapterRAM") {
				ramStr := strings.TrimSpace(strings.Split(line, "=")[1])
				var ram int64
				fmt.Sscanf(ramStr, "%d", &ram)
				if ram > 0 {
					info.GPUMemory = ram / (1024 * 1024 * 1024)
				}
			}
		}
	}

	return info
}

func printSystemInfo(info SystemInfo) {
	fmt.Println("System Information:")
	fmt.Printf("  OS: %s/%s\n", info.OS, info.Arch)
	if info.HasNvidiaSMI {
		fmt.Printf("  GPU: %s\n", info.GPUName)
		if info.GPUMemory > 0 {
			fmt.Printf("  VRAM: %d GB\n", info.GPUMemory)
		}
	}
	fmt.Println()
}

func checkLlamaCpp() Dependency {
	dep := Dependency{Name: "llama.cpp"}

	paths := []string{
		"llama.cpp/llama-server.exe",
		"llama.cpp/llama-server",
		"./llama-server.exe",
		"./llama-server",
	}

	for _, p := range paths {
		// Sanitize the path before using it
		p = filepath.Clean(p)
		if _, err := os.Stat(p); err == nil {
			dep.Installed = true
			dep.Path = p
			cmd := exec.Command(p, "--version")
			output, _ := cmd.Output()
			dep.Version = strings.TrimSpace(string(output))
			break
		}
	}

	return dep
}

func checkComfyUI() Dependency {
	dep := Dependency{Name: "ComfyUI"}

	paths := []string{
		"ComfyUI/ComfyUI.exe",
		"ComfyUI/main.py",
		"./ComfyUI/ComfyUI.exe",
	}

	for _, p := range paths {
		// Sanitize the path before using it
		p = filepath.Clean(p)
		if _, err := os.Stat(p); err == nil {
			dep.Installed = true
			dep.Path = p
			break
		}
	}

	return dep
}

func checkFFmpeg() Dependency {
	dep := Dependency{Name: "FFmpeg"}

	paths := []string{
		"ffmpeg/bin/ffmpeg.exe",
		"ffmpeg/ffmpeg.exe",
		"ffmpeg.exe",
	}

	for _, p := range paths {
		// Sanitize the path before using it
		p = filepath.Clean(p)
		if _, err := os.Stat(p); err == nil {
			dep.Installed = true
			dep.Path = p
			cmd := exec.Command(p, "-version")
			output, _ := cmd.Output()
			lines := strings.Split(string(output), "\n")
			if len(lines) > 0 {
				dep.Version = strings.TrimSpace(lines[0])
			}
			break
		}
	}

	return dep
}

func checkNvidiaSMI() Dependency {
	dep := Dependency{Name: "nvidia-smi"}

	paths := []string{
		"nvidia-smi.exe",
		"C:\\Windows\\System32\\nvidia-smi.exe",
		"C:\\Program Files\\NVIDIA Corporation\\NVSMI\\nvidia-smi.exe",
	}

	for _, p := range paths {
		// First check if nvidia-smi exists
		cmd := exec.Command(p, "--version")
		if err := cmd.Run(); err == nil {
			dep.Installed = true
			dep.Path = p
			output, _ := cmd.Output()
			dep.Version = strings.TrimSpace(string(output))

			// Then query CUDA version
			cmd = exec.Command(p, "--query-gpu=driver_version,cuda_version", "--format=csv,noheader")
			cudaOutput, err := cmd.Output()
			if err == nil {
				lines := strings.Split(string(cudaOutput), "\n")
				if len(lines) > 0 {
					parts := strings.Split(strings.TrimSpace(lines[0]), ",")
					if len(parts) >= 2 {
						cudaVer := strings.TrimSpace(parts[1])
						// CUDA version format: "12.4" -> convert to "12"
						cudaVerParts := strings.Split(cudaVer, ".")
						if len(cudaVerParts) > 0 {
							dep.CUDAVersion = cudaVerParts[0]
						}
					}
				}
			}
			break
		}
	}

	return dep
}

func printDependency(dep Dependency) {
	if dep.Installed {
		fmt.Printf("  [✓] %s installed\n", dep.Name)
		if dep.Version != "" {
			fmt.Printf("      Version: %s\n", dep.Version)
		}
		if dep.CUDAVersion != "" {
			fmt.Printf("      CUDA Version: %s\n", dep.CUDAVersion)
		}
	} else {
		fmt.Printf("  [✗] %s not found\n", dep.Name)
	}
}

func downloadLlamaCpp(gpuInfo SystemInfo, cudaVersion string) error {
	fmt.Println("  Downloading llama.cpp...")

	downloadURL := llamaCppURL

	// Check if NVIDIA GPU is available
	if gpuInfo.HasNvidiaSMI {
		cudaVer := cudaVersion

		// If CUDA version detected, use appropriate build
		if cudaVer != "" {
			fmt.Printf("  Detected CUDA version: %s\n", cudaVer)
		} else {
			// Default to CUDA 12 if GPU detected but CUDA version unknown
			cudaVer = "12"
			fmt.Println("  NVIDIA GPU detected but CUDA version unknown, defaulting to CUDA 12")
		}

		// Map CUDA version to appropriate llama.cpp build
		cudaBuildMap := map[string]string{
			"12": getEnvOrDefault("LLAMA_CPP_DOWNLOAD_WIN", "https://github.com/ggml-org/llama.cpp/releases/download/b8589/llama-b8589-bin-win-cuda-12.4-x64.zip"),
			"11": getEnvOrDefault("LLAMA_CPP_DOWNLOAD_WIN_CUDA_11", "https://github.com/ggml-org/llama.cpp/releases/download/b8589/llama-b8589-bin-win-cuda-11.7-x64.zip"),
		}

		if url, ok := cudaBuildMap[cudaVer]; ok {
			downloadURL = url
			fmt.Printf("  Using CUDA %s compatible build\n", cudaVer)
		} else {
			// Default to CUDA 12 for unknown versions
			downloadURL = cudaBuildMap["12"]
			fmt.Printf("  Using default CUDA 12 build\n")
		}
	} else {
		fmt.Println("  No NVIDIA GPU detected, using default CPU-only build")
		downloadURL = getEnvOrDefault("LLAMA_CPP_DOWNLOAD_WIN_CPU", "https://github.com/ggml-org/llama.cpp/releases/download/b8589/llama-b8589-bin-win-x64.zip")
	}

	fmt.Printf("  URL: %s\n", downloadURL)

	os.MkdirAll("llama.cpp", 0755)

	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	zipPath := "llama.cpp/llama.zip"
	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	fmt.Println("  Extracting...")
	return extractZip(zipPath, "llama.cpp")
}

func downloadFFmpeg() error {
	fmt.Println("  Downloading FFmpeg...")

	client := &http.Client{
		Timeout: 10 * time.Minute,
	}

	resp, err := client.Get(ffmpegURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	os.MkdirAll("ffmpeg", 0755)

	zipPath := "ffmpeg/ffmpeg.zip"
	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	fmt.Println("  Extracting...")
	return extractZip(zipPath, "ffmpeg")
}

func downloadComfyUI() error {
	downloadURL := getComfyUIURL()
	if downloadURL == "" {
		return fmt.Errorf("ComfyUI download URL not available for %s", runtime.GOOS)
	}

	fmt.Println("  Downloading ComfyUI...")

	switch runtime.GOOS {
	case "windows":
		return downloadComfyUIWindows(downloadURL)
	case "darwin":
		return downloadComfyUIMac(downloadURL)
	case "linux":
		return downloadComfyUILinux()
	}

	return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
}

func downloadComfyUIWindows(downloadURL string) error {
	// Note: downloadURL is kept for compatibility but git clone is used when available
	_ = downloadURL

	// Check if git is available
	gitPath, err := exec.LookPath("git")
	if err != nil {
		// Try common git paths on Windows
		commonPaths := []string{
			"C:\\Program Files\\Git\\cmd\\git.exe",
			"C:\\Program Files (x86)\\Git\\cmd\\git.exe",
		}
		for _, p := range commonPaths {
			if _, err := os.Stat(p); err == nil {
				gitPath = p
				err = nil
				break
			}
		}
	}

	if err != nil || gitPath == "" {
		fmt.Println("  Git not found. Attempting to download ComfyUI as zip...")

		// Fallback: download from GitHub as zip
		zipURL := "https://github.com/comfyanonymous/ComfyUI/archive/refs/heads/master.zip"
		return downloadComfyUIAsZip(zipURL)
	}

	fmt.Println("  Cloning ComfyUI repository (this may take a while)...")
	os.MkdirAll("ComfyUI", 0755)

	cmd := exec.Command(gitPath, "clone", "https://github.com/comfyanonymous/ComfyUI.git", "ComfyUI")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = "."

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	// Install dependencies
	fmt.Println("  Installing Python dependencies...")
	cmd = exec.Command(gitPath, "submodule", "update", "--init", "--recursive")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = "ComfyUI"

	err = cmd.Run()
	if err != nil {
		fmt.Printf("  Warning: submodule update failed: %v\n", err)
		fmt.Println("  You may need to install dependencies manually.")
	}

	fmt.Println("  ComfyUI installed successfully!")
	return nil
}

func downloadComfyUIAsZip(zipURL string) error {
	zipPath := "ComfyUI.zip"

	resp, err := http.Get(zipURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	fmt.Println("  Extracting ComfyUI...")
	err = extractZip(zipPath, "ComfyUI-temp")
	if err != nil {
		// If zip extraction fails, provide manual instructions
		os.Remove(zipPath)
		fmt.Println("  Automatic extraction failed.")
		fmt.Println("  Please manually extract the zip and move ComfyUI folder to the project root.")
		fmt.Println("  Download from: https://github.com/comfyanonymous/ComfyUI")
		return nil
	}

	// Move extracted content to ComfyUI folder
	srcDir := "ComfyUI-temp/ComfyUI-master"
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		srcDir = "ComfyUI-temp/ComfyUI-main"
	}

	if _, err := os.Stat(srcDir); err == nil {
		os.Rename(srcDir, "ComfyUI")
		os.RemoveAll("ComfyUI-temp")
	} else {
		// Try to find any extracted folder
		entries, _ := os.ReadDir("ComfyUI-temp")
		for _, entry := range entries {
			if entry.IsDir() && (strings.Contains(entry.Name(), "ComfyUI")) {
				os.Rename(filepath.Join("ComfyUI-temp", entry.Name()), "ComfyUI")
				break
			}
		}
		os.RemoveAll("ComfyUI-temp")
	}

	os.Remove(zipPath)
	fmt.Println("  ComfyUI installed successfully!")
	return nil
}

func downloadComfyUIMac(downloadURL string) error {
	dmgPath := "comfyui-installer.dmg"

	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	out, err := os.Create(dmgPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	fmt.Println("  Mounting DMG...")
	cmd := exec.Command("hdiutil", "attach", dmgPath, "-nobrowse")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to mount DMG: %w", err)
	}

	fmt.Println("  Installing ComfyUI to Applications...")
	cmd = exec.Command("cp", "-r", "/Volumes/ComfyUI/ComfyUI.app", "/Applications/")
	if err := cmd.Run(); err != nil {
		fmt.Printf("  Manual install may be required\n")
	}

	fmt.Println("  Unmounting DMG...")
	cmd = exec.Command("hdiutil", "detach", "/Volumes/ComfyUI")
	cmd.Run()

	os.Remove(dmgPath)
	return nil
}

func downloadComfyUILinux() error {
	fmt.Println("  ComfyUI Desktop for Linux is not yet available.")
	fmt.Println("  Please use manual installation: https://docs.comfy.org/installation/manual_install")
	return nil
}

func extractZip(zipPath, destDir string) error {
	// Convert relative path to absolute for path security checks
	destDirAbs, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	zipFile, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	// Clean and validate the absolute destination directory
	destDirAbs = filepath.Clean(destDirAbs)

	for _, file := range zipFile.Reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		// Clean the file path and check for path traversal attempts
		name := filepath.Clean(file.Name)
		if strings.Contains(name, "..") {
			log.Printf("Skipping potentially malicious path: %s", name)
			continue
		}

		// Handle different zip structures
		// llama.cpp: bin/llama-server.exe -> llama-server.exe
		// ffmpeg: ffmpeg-8.1-essentials_build/bin/ffmpeg.exe -> bin/ffmpeg.exe
		prefixes := []string{"bin\\", "llama.cpp\\", "bin/", "ffmpeg-8.1-essentials_build\\", "ffmpeg-7.1-essentials_build\\"}
		for _, prefix := range prefixes {
			if trimmed, found := strings.CutPrefix(name, prefix); found {
				name = trimmed
				break
			}
		}

		// Build the final output path and verify it's within destDirAbs
		outPath := filepath.Join(destDirAbs, name)
		if !strings.HasPrefix(filepath.Clean(outPath)+string(os.PathSeparator), destDirAbs+string(os.PathSeparator)) {
			log.Printf("Skipping path traversal attempt: %s", outPath)
			continue
		}

		os.MkdirAll(filepath.Dir(outPath), 0755)

		outFile, err := os.Create(outPath)
		if err != nil {
			log.Printf("Failed to create file %s: %v", outPath, err)
			continue
		}

		rc, err := file.Open()
		if err != nil {
			outFile.Close()
			log.Printf("Failed to open zip entry %s: %v", name, err)
			continue
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()

		if err != nil {
			log.Printf("Failed to copy file %s: %v", outPath, err)
			os.Remove(outPath)
		}
	}

	os.Remove(zipPath)
	return nil
}

func createConfigFile(gpuInfo SystemInfo) error {
	memoryGB := 8
	if gpuInfo.GPUMemory > 0 {
		memoryGB = int(gpuInfo.GPUMemory)
	} else if strings.Contains(gpuInfo.GPUName, "2070") {
		memoryGB = 8
	} else if strings.Contains(gpuInfo.GPUName, "3060") {
		memoryGB = 12
	} else if strings.Contains(gpuInfo.GPUName, "4070") {
		memoryGB = 12
	} else if strings.Contains(gpuInfo.GPUName, "4090") {
		memoryGB = 24
	} else if gpuInfo.GPUName == "" {
		fmt.Println("  Warning: No GPU detected, using default 8GB")
	}

	llamaPath := "./llama.cpp/llama-server.exe"
	ffmpegPath := "./ffmpeg/bin/ffmpeg.exe"

	if _, err := os.Stat(llamaPath); os.IsNotExist(err) {
		llamaPath = "./llama.cpp/llama-server"
	}
	if _, err := os.Stat(ffmpegPath); os.IsNotExist(err) {
		ffmpegPath = "ffmpeg"
	}

	config := fmt.Sprintf(`server:
  host: "0.0.0.0"
  port: 8080

llama:
  binary: "%s"
  context_size: 2048
  prompt_batch: 512
  generation_batch: 32
  gpu_layers: 100

# GPU Configuration
gpu:
  memory_gb: %d
  auto_detect: true
  gpu_ids: [0]

models:
  - name: "llama-7b"
    path: "./models/llama-7b-chat.Q4_K_M.gguf"
    instances: 1
    context_size: 2048
    priority: 1

pool:
  base_port: 8081
  health_check_interval: 5s

batcher:
  max_wait_ms: 100
  max_batch_size: 512
  queue_size: 100

kv_cache:
  enabled: true
  cache_dir: "./cache"
  max_size_mb: 1024
  max_age_hours: 24

# Multimodal Configuration (Optional)
multimodal:
  comfyui:
    enabled: false
    host: "127.0.0.1"
    port: 8188
    output_dir: "./output/images"
  ffmpeg:
    enabled: false
    binary_path: "%s"
    output_dir: "./output/videos"
`, llamaPath, memoryGB, ffmpegPath)

	err := os.WriteFile("configs/config.yaml", []byte(config), 0644)
	if err != nil {
		return fmt.Errorf("failed to create config.yaml: %w", err)
	}

	fmt.Printf("  Created config.yaml (GPU: %dGB)\n", memoryGB)
	return nil
}

func init() {
	log.SetFlags(0)
	log.SetOutput(io.Discard)

	godotenv.Load()

	llamaCppURL = getLlamaCppURL()
	ffmpegURL = getFFmpegURL()
}

func getLlamaCppURL() string {
	switch runtime.GOOS {
	case "windows":
		if url := os.Getenv("LLAMA_CPP_DOWNLOAD_WIN"); url != "" {
			return url
		}
	case "linux":
		if url := os.Getenv("LLAMA_CPP_DOWNLOAD_LINUX"); url != "" {
			return url
		}
	case "darwin":
		if url := os.Getenv("LLAMA_CPP_DOWNLOAD_MAC"); url != "" {
			return url
		}
	}
	return ""
}

func getFFmpegURL() string {
	switch runtime.GOOS {
	case "windows":
		if url := os.Getenv("Ffmpeg_DOWNLOAD_WIN"); url != "" {
			return url
		}
	case "linux":
		if url := os.Getenv("Ffmpeg_DOWNLOAD_LINUX"); url != "" {
			return url
		}
	}
	return ""
}

func getComfyUIURL() string {
	switch runtime.GOOS {
	case "windows":
		if url := os.Getenv("COMFYUI_DOWNLOAD_WIN"); url != "" {
			return url
		}
	case "darwin":
		if url := os.Getenv("COMFYUI_DOWNLOAD_MAC"); url != "" {
			return url
		}
	}
	return ""
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
