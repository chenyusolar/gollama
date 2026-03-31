package multimodal

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type FFmpeg struct {
	binaryPath   string
	outputDir    string
	outputFormat string
	videoCodec   string
	audioCodec   string
	videoBitrate string
	frameRate    int
}

func NewFFmpeg(binaryPath, outputDir, outputFormat, videoCodec, audioCodec, videoBitrate string, frameRate int) *FFmpeg {
	os.MkdirAll(outputDir, 0755)
	return &FFmpeg{
		binaryPath:   binaryPath,
		outputDir:    outputDir,
		outputFormat: outputFormat,
		videoCodec:   videoCodec,
		audioCodec:   audioCodec,
		videoBitrate: videoBitrate,
		frameRate:    frameRate,
	}
}

func (f *FFmpeg) ImagesToVideo(imagePaths []string, outputName string, fps int) (string, error) {
	if fps == 0 {
		fps = f.frameRate
	}
	if fps == 0 {
		fps = 30
	}

	workDir := filepath.Join(f.outputDir, "temp_"+fmt.Sprintf("%d", time.Now().Unix()))
	os.MkdirAll(workDir, 0755)
	defer os.RemoveAll(workDir)

	for i, srcPath := range imagePaths {
		dstPath := filepath.Join(workDir, fmt.Sprintf("frame_%05d.png", i))
		if err := copyFile(srcPath, dstPath); err != nil {
			return "", fmt.Errorf("failed to copy frame %d: %w", i, err)
		}
	}

	outputPath := filepath.Join(f.outputDir, outputName)
	if !strings.HasSuffix(outputPath, ".mp4") {
		outputPath += ".mp4"
	}

	args := []string{
		"-y",
		"-framerate", fmt.Sprintf("%d", fps),
		"-i", filepath.Join(workDir, "frame_%05d.png"),
		"-c:v", f.videoCodec,
		"-b:v", f.videoBitrate,
		"-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
		outputPath,
	}

	log.Printf("Running ffmpeg: ffmpeg %s", strings.Join(args, " "))

	cmd := exec.Command(f.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w\n%s", err, string(output))
	}

	log.Printf("Video created: %s", outputPath)
	return outputPath, nil
}

func (f *FFmpeg) VideoWithAudio(videoPath, audioPath, outputName string) (string, error) {
	outputPath := filepath.Join(f.outputDir, outputName)
	if !strings.HasSuffix(outputPath, ".mp4") {
		outputPath += ".mp4"
	}

	args := []string{
		"-y",
		"-i", videoPath,
		"-i", audioPath,
		"-c:v", "copy",
		"-c:a", f.audioCodec,
		"-shortest",
		outputPath,
	}

	cmd := exec.Command(f.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w\n%s", err, string(output))
	}

	return outputPath, nil
}

func (f *FFmpeg) TrimVideo(videoPath, outputName string, startTime, duration float64) (string, error) {
	outputPath := filepath.Join(f.outputDir, outputName)
	if !strings.HasSuffix(outputPath, ".mp4") {
		outputPath += ".mp4"
	}

	args := []string{
		"-y",
		"-ss", fmt.Sprintf("%f", startTime),
		"-t", fmt.Sprintf("%f", duration),
		"-i", videoPath,
		"-c", "copy",
		outputPath,
	}

	cmd := exec.Command(f.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w\n%s", err, string(output))
	}

	return outputPath, nil
}

func (f *FFmpeg) ConcatenateVideos(videoPaths []string, outputName string) (string, error) {
	if len(videoPaths) == 0 {
		return "", fmt.Errorf("no videos to concatenate")
	}

	listFile := filepath.Join(f.outputDir, "concat_list.txt")
	file, err := os.Create(listFile)
	if err != nil {
		return "", err
	}

	for _, path := range videoPaths {
		fmt.Fprintf(file, "file '%s'\n", path)
	}
	file.Close()

	outputPath := filepath.Join(f.outputDir, outputName)
	if !strings.HasSuffix(outputPath, ".mp4") {
		outputPath += ".mp4"
	}

	args := []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listFile,
		"-c", "copy",
		outputPath,
	}

	cmd := exec.Command(f.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(listFile)
		return "", fmt.Errorf("ffmpeg failed: %w\n%s", err, string(output))
	}

	os.Remove(listFile)
	return outputPath, nil
}

func (f *FFmpeg) ExtractFrames(videoPath, outputDir string, fps int) ([]string, error) {
	if fps == 0 {
		fps = f.frameRate
	}
	if fps == 0 {
		fps = 30
	}

	os.MkdirAll(outputDir, 0755)

	args := []string{
		"-y",
		"-i", videoPath,
		"-vf", fmt.Sprintf("fps=%d", fps),
		filepath.Join(outputDir, "frame_%05d.png"),
	}

	cmd := exec.Command(f.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w\n%s", err, string(output))
	}

	var frames []string
	files, _ := os.ReadDir(outputDir)
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".png") {
			frames = append(frames, filepath.Join(outputDir, file.Name()))
		}
	}

	return frames, nil
}

func (f *FFmpeg) AddWatermark(videoPath, watermarkPath, outputName string, position string) (string, error) {
	outputPath := filepath.Join(f.outputDir, outputName)
	if !strings.HasSuffix(outputPath, ".mp4") {
		outputPath += ".mp4"
	}

	overlayPos := "10:10"
	switch position {
	case "top-left":
		overlayPos = "10:10"
	case "top-right":
		overlayPos = "W-w-10:10"
	case "bottom-left":
		overlayPos = "10:H-h-10"
	case "bottom-right":
		overlayPos = "W-w-10:H-h-10"
	case "center":
		overlayPos = "(W-w)/2:(H-h)/2"
	}

	args := []string{
		"-y",
		"-i", videoPath,
		"-i", watermarkPath,
		"-filter_complex", fmt.Sprintf("[1:v]format=rgba,colorchannelmixer=aa=0.5[watermark];[0:v][watermark]overlay=%s", overlayPos),
		"-c:a", "copy",
		outputPath,
	}

	cmd := exec.Command(f.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w\n%s", err, string(output))
	}

	return outputPath, nil
}

func (f *FFmpeg) ChangeSpeed(videoPath, outputName string, speed float64) (string, error) {
	outputPath := filepath.Join(f.outputDir, outputName)
	if !strings.HasSuffix(outputPath, ".mp4") {
		outputPath += ".mp4"
	}

	args := []string{
		"-y",
		"-i", videoPath,
		"-filter:v", fmt.Sprintf("setpts=%f*PTS", 1.0/speed),
		"-filter:a", fmt.Sprintf("atempo=%f", speed),
		"-c:v", "libx264",
		"-c:a", "aac",
		outputPath,
	}

	cmd := exec.Command(f.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w\n%s", err, string(output))
	}

	return outputPath, nil
}

func (f *FFmpeg) ReverseVideo(videoPath, outputName string) (string, error) {
	outputPath := filepath.Join(f.outputDir, outputName)
	if !strings.HasSuffix(outputPath, ".mp4") {
		outputPath += ".mp4"
	}

	args := []string{
		"-y",
		"-i", videoPath,
		"-vf", "reverse",
		"-af", "areverse",
		outputPath,
	}

	cmd := exec.Command(f.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w\n%s", err, string(output))
	}

	return outputPath, nil
}

func (f *FFmpeg) AddTextOverlay(videoPath, outputName, text, fontPath string, fontSize, x, y int) (string, error) {
	outputPath := filepath.Join(f.outputDir, outputName)
	if !strings.HasSuffix(outputPath, ".mp4") {
		outputPath += ".mp4"
	}

	drawText := fmt.Sprintf("drawtext=text='%s':fontcolor=white:fontsize=%d:x=%d:y=%d:box=1:boxcolor=black@0.5:boxborderw=5", text, fontSize, x, y)

	args := []string{
		"-y",
		"-i", videoPath,
		"-vf", drawText,
		"-c:a", "copy",
		outputPath,
	}

	cmd := exec.Command(f.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w\n%s", err, string(output))
	}

	return outputPath, nil
}

func (f *FFmpeg) GetVideoInfo(videoPath string) (map[string]interface{}, error) {
	args := []string{
		"-i", videoPath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
	}

	cmd := exec.Command(f.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w", err)
	}

	info := make(map[string]any)
	info["raw"] = string(output)

	return info, nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	return err
}
