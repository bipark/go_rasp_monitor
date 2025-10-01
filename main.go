package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

type SystemStats struct {
	CPUPercent    []float64
	MemPercent    float64
	MemUsed       uint64
	MemTotal      uint64
	DiskPercent   float64
	DiskUsed      uint64
	DiskTotal     uint64
	Temperature   float64
	Uptime        uint64
	NetSent       uint64
	NetRecv       uint64
	LoadAvg       []float64
	ProcessCount  uint64
}

func main() {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

	// Create widgets
	cpuGauge := widgets.NewGauge()
	cpuGauge.Title = "CPU Usage"
	cpuGauge.SetRect(0, 0, 50, 3)
	cpuGauge.BarColor = ui.ColorGreen
	cpuGauge.LabelStyle = ui.NewStyle(ui.ColorWhite)

	memGauge := widgets.NewGauge()
	memGauge.Title = "Memory Usage"
	memGauge.SetRect(0, 3, 50, 6)
	memGauge.BarColor = ui.ColorYellow
	memGauge.LabelStyle = ui.NewStyle(ui.ColorWhite)

	diskGauge := widgets.NewGauge()
	diskGauge.Title = "Disk Usage"
	diskGauge.SetRect(0, 6, 50, 9)
	diskGauge.BarColor = ui.ColorCyan
	diskGauge.LabelStyle = ui.NewStyle(ui.ColorWhite)

	cpuChart := widgets.NewSparkline()
	cpuChart.LineColor = ui.ColorGreen
	cpuChart.TitleStyle.Fg = ui.ColorWhite
	cpuChartGroup := widgets.NewSparklineGroup(cpuChart)
	cpuChartGroup.Title = "CPU History"
	cpuChartGroup.SetRect(50, 0, 100, 9)

	infoList := widgets.NewList()
	infoList.Title = "System Information"
	infoList.SetRect(0, 9, 50, 20)
	infoList.TextStyle = ui.NewStyle(ui.ColorWhite)
	infoList.BorderStyle = ui.NewStyle(ui.ColorCyan)

	netList := widgets.NewList()
	netList.Title = "Network Statistics"
	netList.SetRect(50, 9, 100, 20)
	netList.TextStyle = ui.NewStyle(ui.ColorWhite)
	netList.BorderStyle = ui.NewStyle(ui.ColorMagenta)

	helpParagraph := widgets.NewParagraph()
	helpParagraph.Title = "Controls"
	helpParagraph.Text = "q: Quit | r: Refresh"
	helpParagraph.SetRect(0, 20, 100, 23)
	helpParagraph.BorderStyle = ui.NewStyle(ui.ColorYellow)

	// CPU history data
	cpuHistory := make([]float64, 50)
	var prevNetSent, prevNetRecv uint64

	// Update function
	updateStats := func() {
		stats := getSystemStats()

		// Update CPU gauge
		avgCPU := 0.0
		for _, c := range stats.CPUPercent {
			avgCPU += c
		}
		if len(stats.CPUPercent) > 0 {
			avgCPU /= float64(len(stats.CPUPercent))
		}
		cpuGauge.Percent = int(avgCPU)
		cpuGauge.Label = fmt.Sprintf("%.2f%%", avgCPU)

		if avgCPU > 80 {
			cpuGauge.BarColor = ui.ColorRed
		} else if avgCPU > 50 {
			cpuGauge.BarColor = ui.ColorYellow
		} else {
			cpuGauge.BarColor = ui.ColorGreen
		}

		// Update memory gauge
		memGauge.Percent = int(stats.MemPercent)
		memGauge.Label = fmt.Sprintf("%.2f%% (%.2f GB / %.2f GB)", 
			stats.MemPercent, 
			float64(stats.MemUsed)/1024/1024/1024,
			float64(stats.MemTotal)/1024/1024/1024)

		if stats.MemPercent > 80 {
			memGauge.BarColor = ui.ColorRed
		} else if stats.MemPercent > 50 {
			memGauge.BarColor = ui.ColorYellow
		} else {
			memGauge.BarColor = ui.ColorGreen
		}

		// Update disk gauge
		diskGauge.Percent = int(stats.DiskPercent)
		diskGauge.Label = fmt.Sprintf("%.2f%% (%.2f GB / %.2f GB)", 
			stats.DiskPercent,
			float64(stats.DiskUsed)/1024/1024/1024,
			float64(stats.DiskTotal)/1024/1024/1024)

		if stats.DiskPercent > 80 {
			diskGauge.BarColor = ui.ColorRed
		} else if stats.DiskPercent > 50 {
			diskGauge.BarColor = ui.ColorYellow
		} else {
			diskGauge.BarColor = ui.ColorCyan
		}

		// Update CPU history
		cpuHistory = append(cpuHistory[1:], avgCPU)
		cpuChart.Data = cpuHistory

		// Update system info
		uptimeDays := stats.Uptime / 86400
		uptimeHours := (stats.Uptime % 86400) / 3600
		uptimeMinutes := (stats.Uptime % 3600) / 60

		loadAvgStr := "N/A"
		if len(stats.LoadAvg) >= 3 {
			loadAvgStr = fmt.Sprintf("%.2f, %.2f, %.2f", 
				stats.LoadAvg[0], stats.LoadAvg[1], stats.LoadAvg[2])
		}

		tempStr := "N/A"
		if stats.Temperature > 0 {
			tempStr = fmt.Sprintf("%.1f°C", stats.Temperature)
		}

		infoList.Rows = []string{
			fmt.Sprintf("[Hostname:](fg:cyan) %s", getHostname()),
			fmt.Sprintf("[OS:](fg:cyan) %s", runtime.GOOS),
			fmt.Sprintf("[Architecture:](fg:cyan) %s", runtime.GOARCH),
			fmt.Sprintf("[CPU Cores:](fg:cyan) %d", runtime.NumCPU()),
			fmt.Sprintf("[Temperature:](fg:cyan) %s", tempStr),
			fmt.Sprintf("[Uptime:](fg:cyan) %dd %dh %dm", uptimeDays, uptimeHours, uptimeMinutes),
			fmt.Sprintf("[Load Average:](fg:cyan) %s", loadAvgStr),
			fmt.Sprintf("[Processes:](fg:cyan) %d", stats.ProcessCount),
		}

		// Calculate network speed
		netSentDiff := stats.NetSent - prevNetSent
		netRecvDiff := stats.NetRecv - prevNetRecv
		prevNetSent = stats.NetSent
		prevNetRecv = stats.NetRecv

		netList.Rows = []string{
			fmt.Sprintf("[Total Sent:](fg:magenta) %.2f MB", float64(stats.NetSent)/1024/1024),
			fmt.Sprintf("[Total Received:](fg:magenta) %.2f MB", float64(stats.NetRecv)/1024/1024),
			fmt.Sprintf("[Upload Speed:](fg:magenta) %.2f KB/s", float64(netSentDiff)/1024),
			fmt.Sprintf("[Download Speed:](fg:magenta) %.2f KB/s", float64(netRecvDiff)/1024),
		}
	}

	// Initial update
	updateStats()

	// Render
	ui.Render(cpuGauge, memGauge, diskGauge, cpuChartGroup, infoList, netList, helpParagraph)

	// Update ticker
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// Event loop
	uiEvents := ui.PollEvents()
	for {
		select {
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return
			case "r":
				updateStats()
				ui.Render(cpuGauge, memGauge, diskGauge, cpuChartGroup, infoList, netList, helpParagraph)
			case "<Resize>":
				payload := e.Payload.(ui.Resize)
				width := payload.Width
				height := payload.Height

				// Recalculate widget positions
				halfWidth := width / 2
				cpuGauge.SetRect(0, 0, halfWidth, 3)
				memGauge.SetRect(0, 3, halfWidth, 6)
				diskGauge.SetRect(0, 6, halfWidth, 9)
				cpuChartGroup.SetRect(halfWidth, 0, width, 9)
				infoList.SetRect(0, 9, halfWidth, height-3)
				netList.SetRect(halfWidth, 9, width, height-3)
				helpParagraph.SetRect(0, height-3, width, height)

				ui.Clear()
				ui.Render(cpuGauge, memGauge, diskGauge, cpuChartGroup, infoList, netList, helpParagraph)
			}
		case <-ticker.C:
			updateStats()
			ui.Render(cpuGauge, memGauge, diskGauge, cpuChartGroup, infoList, netList, helpParagraph)
		}
	}
}

func getSystemStats() SystemStats {
	stats := SystemStats{}

	// CPU
	if cpuPercents, err := cpu.Percent(0, true); err == nil {
		stats.CPUPercent = cpuPercents
	}

	// Memory
	if memInfo, err := mem.VirtualMemory(); err == nil {
		stats.MemPercent = memInfo.UsedPercent
		stats.MemUsed = memInfo.Used
		stats.MemTotal = memInfo.Total
	}

	// Disk
	if diskInfo, err := disk.Usage("/"); err == nil {
		stats.DiskPercent = diskInfo.UsedPercent
		stats.DiskUsed = diskInfo.Used
		stats.DiskTotal = diskInfo.Total
	}

	// Temperature (Raspberry Pi specific)
	stats.Temperature = getCPUTemperature()

	// Host info
	if hostInfo, err := host.Info(); err == nil {
		stats.Uptime = hostInfo.Uptime
		stats.ProcessCount = hostInfo.Procs
	}

	// Load average
	if loadAvg, err := host.LoadAverage(); err == nil {
		stats.LoadAvg = []float64{loadAvg.Load1, loadAvg.Load5, loadAvg.Load15}
	}

	// Network
	if netStats, err := net.IOCounters(false); err == nil && len(netStats) > 0 {
		stats.NetSent = netStats[0].BytesSent
		stats.NetRecv = netStats[0].BytesRecv
	}

	return stats
}

func getCPUTemperature() float64 {
	// Try to read from Raspberry Pi thermal zone
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0
	}

	tempStr := strings.TrimSpace(string(data))
	temp, err := strconv.ParseFloat(tempStr, 64)
	if err != nil {
		return 0
	}

	return temp / 1000.0 // Convert from millidegrees to degrees
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}