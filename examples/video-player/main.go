package main

import (
	"bufio"
	"embed"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

//go:embed video-bridge.js
var videoBridgeJS string

const (
	canvasWidth  = 960
	canvasHeight = 540
	targetFPS    = 24
)

type FrameData struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Frame  string `json:"frame"` // base64-encoded JPEG
}

type App struct {
	godom.Component
	Player   FrameData
	Playing  bool
	FrameNum int
	Total    int
	Status   string

	mu       sync.Mutex
	frames   [][]byte // all decoded frames
	videoSrc string
}

func (a *App) PlayPause() {
	a.Playing = !a.Playing
}

func (a *App) Stop() {
	a.Playing = false
	a.FrameNum = 0
	a.showFrame(0)
}

func (a *App) Forward() {
	a.mu.Lock()
	total := len(a.frames)
	a.mu.Unlock()
	if total == 0 {
		return
	}
	next := a.FrameNum + targetFPS*5 // skip 5 seconds
	if next >= total {
		next = total - 1
	}
	a.FrameNum = next
	a.showFrame(next)
}

func (a *App) Backward() {
	prev := a.FrameNum - targetFPS*5 // back 5 seconds
	if prev < 0 {
		prev = 0
	}
	a.FrameNum = prev
	a.showFrame(prev)
}

func (a *App) showFrame(idx int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if idx < 0 || idx >= len(a.frames) {
		return
	}
	a.Player = FrameData{
		Width:  canvasWidth,
		Height: canvasHeight,
		Frame:  base64.StdEncoding.EncodeToString(a.frames[idx]),
	}
}

func (a *App) decodeVideo() {
	a.Status = "Decoding video..."
	a.Refresh()

	cmd := exec.Command("ffmpeg",
		"-i", a.videoSrc,
		"-vf", fmt.Sprintf("fps=%d,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
			targetFPS, canvasWidth, canvasHeight, canvasWidth, canvasHeight),
		"-f", "image2pipe",
		"-c:v", "mjpeg",
		"-q:v", "5",
		"-an",
		"pipe:1",
	)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		a.Status = "Error: " + err.Error()
		a.Refresh()
		return
	}

	if err := cmd.Start(); err != nil {
		a.Status = "Error starting ffmpeg: " + err.Error()
		a.Refresh()
		return
	}

	reader := bufio.NewReaderSize(stdout, 512*1024)
	var frames [][]byte

	for {
		frame, err := readJPEGFrame(reader)
		if err != nil || frame == nil {
			break
		}
		frames = append(frames, frame)
		if len(frames)%targetFPS == 0 {
			a.Status = fmt.Sprintf("Decoded %d frames (%.1fs)...", len(frames), float64(len(frames))/targetFPS)
			a.Refresh()
		}
	}

	cmd.Wait()

	a.mu.Lock()
	a.frames = frames
	a.mu.Unlock()

	a.Total = len(frames)
	a.Status = fmt.Sprintf("Ready — %d frames (%.1fs)", len(frames), float64(len(frames))/targetFPS)
	if len(frames) > 0 {
		a.showFrame(0)
	}
	a.Refresh()
}

// readJPEGFrame reads one JPEG image from a concatenated MJPEG stream.
// It looks for SOI (0xFFD8) and EOI (0xFFD9) markers.
func readJPEGFrame(r *bufio.Reader) ([]byte, error) {
	// Find SOI marker
	for {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b == 0xFF {
			b2, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			if b2 == 0xD8 {
				break // found SOI
			}
		}
	}

	buf := []byte{0xFF, 0xD8}
	for {
		b, err := r.ReadByte()
		if err != nil {
			return buf, err
		}
		buf = append(buf, b)
		if b == 0xFF {
			b2, err := r.ReadByte()
			if err != nil {
				return buf, err
			}
			buf = append(buf, b2)
			if b2 == 0xD9 {
				return buf, nil // found EOI
			}
		}
	}
}

func (a *App) run() {
	go a.decodeVideo()

	ticker := time.NewTicker(time.Second / targetFPS)
	for range ticker.C {
		if !a.Playing {
			continue
		}
		a.mu.Lock()
		total := len(a.frames)
		a.mu.Unlock()
		if total == 0 {
			continue
		}
		if a.FrameNum >= total {
			a.Playing = false
			a.FrameNum = total - 1
			a.Refresh()
			continue
		}
		a.showFrame(a.FrameNum)
		a.FrameNum++
		a.Refresh()
	}
}

func main() {
	// Scan os.Args for -video flag manually to avoid conflicts with godom's flags
	videoPath := ""
	for i, arg := range os.Args[1:] {
		if (arg == "-video" || arg == "--video") && i+1 < len(os.Args)-1 {
			videoPath = os.Args[i+2]
			break
		}
	}
	if videoPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: video-player -video <path> [godom flags]\n")
		os.Exit(1)
	}
	if _, err := os.Stat(videoPath); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot open video: %s\n", err)
		os.Exit(1)
	}

	// Probe video duration for display
	dur := probeSeconds(videoPath)

	eng := godom.NewEngine()
	eng.SetFS(ui)
	eng.RegisterPlugin("videocanvas", videoBridgeJS)

	root := &App{
		videoSrc: videoPath,
		Status:   fmt.Sprintf("Loading: %s (%.0fs)", videoPath, dur),
	}
	root.Player = FrameData{Width: canvasWidth, Height: canvasHeight}
	go root.run()

	fmt.Printf("Video Player — %s\n", videoPath)
	eng.Mount(root, "ui/index.html")
	log.Fatal(eng.Start())
}

func probeSeconds(path string) float64 {
	out, err := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	).Output()
	if err != nil {
		return 0
	}
	d, _ := strconv.ParseFloat(string(out[:len(out)-1]), 64)
	return d
}
