package main

import (
	"embed"
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/anupshinde/godom"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

//go:embed ui
var ui embed.FS

type Stat struct {
	Label    string
	Value    string
	Percent  float64
	BarStyle string
}

type App struct {
	godom.Component
	Stats  []Stat
	Uptime string
}

func (a *App) startMonitor() {
	start := time.Now()
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		a.Uptime = formatDuration(time.Since(start))
		a.Stats = collectStats()
		a.Refresh()
	}
}

func collectStats() []Stat {
	var stats []Stat

	// CPU
	if pcts, err := cpu.Percent(0, false); err == nil && len(pcts) > 0 {
		stats = append(stats, makeStat("CPU", fmt.Sprintf("%.1f%%", pcts[0]), pcts[0]))
	}

	// Memory
	if vm, err := mem.VirtualMemory(); err == nil {
		stats = append(stats, makeStat("Memory",
			fmt.Sprintf("%s / %s", formatBytes(vm.Used), formatBytes(vm.Total)),
			vm.UsedPercent))
	}

	// Disk
	if d, err := disk.Usage("/"); err == nil {
		stats = append(stats, makeStat("Disk",
			fmt.Sprintf("%s / %s", formatBytes(d.Used), formatBytes(d.Total)),
			d.UsedPercent))
	}

	// Go runtime
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	stats = append(stats, makeStat("Go Heap", formatBytes(m.Alloc), 0))
	stats = append(stats, makeStat("Goroutines", fmt.Sprintf("%d", runtime.NumGoroutine()), 0))

	return stats
}

func makeStat(label, value string, percent float64) Stat {
	barStyle := ""
	if percent > 0 {
		barStyle = fmt.Sprintf("width: %.1f%%", percent)
	}
	return Stat{Label: label, Value: value, Percent: percent, BarStyle: barStyle}
}

func formatBytes(b uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func main() {
	eng := godom.NewEngine()
	eng.SetFS(ui)
	root := &App{}
	go root.startMonitor()
	eng.Mount(root, "ui/index.html")
	log.Fatal(eng.Start())
}
