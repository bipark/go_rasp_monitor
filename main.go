package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	updateInterval = time.Second
	historySize    = 50
)

// ProcessInfo represents information about a single process
type ProcessInfo struct {
	PID         int32
	Name        string
	CPU         float64
	Memory      float64
	Status      string
	Username    string
	PPID        int32
	Connections int
	Ports       []uint32
}

// SystemStats holds all system statistics
type SystemStats struct {
	CPUPercent   []float64
	MemPercent   float64
	MemUsed      uint64
	MemTotal     uint64
	DiskPercent  float64
	DiskUsed     uint64
	DiskTotal    uint64
	Temperature  float64
	Uptime       uint64
	NetSent      uint64
	NetRecv      uint64
	LoadAvg      []float64
	ProcessCount uint64
	AllProcesses []ProcessInfo
}

// Dashboard manages all UI widgets and state
type Dashboard struct {
	cpuGauge        *widgets.Gauge
	memGauge        *widgets.Gauge
	diskGauge       *widgets.Gauge
	cpuChart        *widgets.SparklineGroup
	infoList        *widgets.List
	netList         *widgets.List
	allProcessList  *widgets.List
	helpParagraph   *widgets.Paragraph

	cpuHistory      []float64
	prevNetSent     uint64
	prevNetRecv     uint64
	cpuSparkline    *widgets.Sparkline
	selectedProcess int
}

func main() {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

	dashboard := NewDashboard()
	dashboard.InitWidgets()
	dashboard.UpdateStats()
	dashboard.Render()

	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()

	dashboard.EventLoop(ticker)
}

// NewDashboard creates a new dashboard instance
func NewDashboard() *Dashboard {
	return &Dashboard{
		cpuHistory:      make([]float64, historySize),
		selectedProcess: 0,
	}
}

// InitWidgets initializes all UI widgets
func (d *Dashboard) InitWidgets() {
	d.cpuGauge = d.createGauge("CPU Usage", 0, 0, 50, 3, ui.ColorGreen)
	d.memGauge = d.createGauge("Memory Usage", 0, 3, 50, 6, ui.ColorYellow)
	d.diskGauge = d.createGauge("Disk Usage", 0, 6, 50, 9, ui.ColorCyan)

	d.cpuSparkline = widgets.NewSparkline()
	d.cpuSparkline.LineColor = ui.ColorGreen
	d.cpuSparkline.TitleStyle.Fg = ui.ColorWhite
	d.cpuChart = widgets.NewSparklineGroup(d.cpuSparkline)
	d.cpuChart.Title = "CPU History"
	d.cpuChart.SetRect(50, 0, 100, 9)

	d.infoList = d.createList("System Information", 0, 9, 50, 20, ui.ColorCyan)
	d.netList = d.createList("Network Statistics", 50, 9, 100, 20, ui.ColorMagenta)
	
	// All processes widget - bigger now
	d.allProcessList = d.createList("All Processes (↑↓: Select & Scroll)", 0, 20, 100, 50, ui.ColorBlue)

	d.helpParagraph = widgets.NewParagraph()
	d.helpParagraph.Title = "Controls"
	d.helpParagraph.Text = "q: Quit | r: Refresh | ↑↓: Navigate Processes | PgUp/PgDn: Fast Scroll | Home: Top | End: Bottom"
	d.helpParagraph.SetRect(0, 50, 100, 54)
	d.helpParagraph.BorderStyle = ui.NewStyle(ui.ColorYellow)
}

// createGauge creates a gauge widget with the given parameters
func (d *Dashboard) createGauge(title string, x1, y1, x2, y2 int, color ui.Color) *widgets.Gauge {
	gauge := widgets.NewGauge()
	gauge.Title = title
	gauge.SetRect(x1, y1, x2, y2)
	gauge.BarColor = color
	gauge.LabelStyle = ui.NewStyle(ui.ColorWhite)
	return gauge
}

// createList creates a list widget with the given parameters
func (d *Dashboard) createList(title string, x1, y1, x2, y2 int, borderColor ui.Color) *widgets.List {
	list := widgets.NewList()
	list.Title = title
	list.SetRect(x1, y1, x2, y2)
	list.TextStyle = ui.NewStyle(ui.ColorWhite)
	list.BorderStyle = ui.NewStyle(borderColor)
	return list
}

// UpdateStats updates all system statistics and widgets
func (d *Dashboard) UpdateStats() {
	stats := getSystemStats()

	d.updateCPUGauge(stats)
	d.updateMemGauge(stats)
	d.updateDiskGauge(stats)
	d.updateCPUChart(stats)
	d.updateSystemInfo(stats)
	d.updateNetworkInfo(stats)
	d.updateAllProcessList(stats)
}

// updateCPUGauge updates the CPU gauge widget
func (d *Dashboard) updateCPUGauge(stats SystemStats) {
	avgCPU := calculateAverage(stats.CPUPercent)
	d.cpuGauge.Percent = int(avgCPU)
	d.cpuGauge.Label = fmt.Sprintf("%.2f%%", avgCPU)
	d.cpuGauge.BarColor = getColorByThreshold(avgCPU, 50, 80)
}

// updateMemGauge updates the memory gauge widget
func (d *Dashboard) updateMemGauge(stats SystemStats) {
	d.memGauge.Percent = int(stats.MemPercent)
	d.memGauge.Label = fmt.Sprintf("%.2f%% (%.2f GB / %.2f GB)",
		stats.MemPercent,
		bytesToGB(stats.MemUsed),
		bytesToGB(stats.MemTotal))
	d.memGauge.BarColor = getColorByThreshold(stats.MemPercent, 50, 80)
}

// updateDiskGauge updates the disk gauge widget
func (d *Dashboard) updateDiskGauge(stats SystemStats) {
	d.diskGauge.Percent = int(stats.DiskPercent)
	d.diskGauge.Label = fmt.Sprintf("%.2f%% (%.2f GB / %.2f GB)",
		stats.DiskPercent,
		bytesToGB(stats.DiskUsed),
		bytesToGB(stats.DiskTotal))
	d.diskGauge.BarColor = getColorByThreshold(stats.DiskPercent, 50, 80)
}

// updateCPUChart updates the CPU history chart
func (d *Dashboard) updateCPUChart(stats SystemStats) {
	avgCPU := calculateAverage(stats.CPUPercent)
	d.cpuHistory = append(d.cpuHistory[1:], avgCPU)
	d.cpuSparkline.Data = d.cpuHistory
}

// updateSystemInfo updates the system information list
func (d *Dashboard) updateSystemInfo(stats SystemStats) {
	days, hours, minutes := formatUptime(stats.Uptime)
	loadAvgStr := formatLoadAverage(stats.LoadAvg)
	tempStr := formatTemperature(stats.Temperature)

	d.infoList.Rows = []string{
		fmt.Sprintf("[Hostname:](fg:cyan) %s", getHostname()),
		fmt.Sprintf("[OS:](fg:cyan) %s", runtime.GOOS),
		fmt.Sprintf("[Architecture:](fg:cyan) %s", runtime.GOARCH),
		fmt.Sprintf("[CPU Cores:](fg:cyan) %d", runtime.NumCPU()),
		fmt.Sprintf("[Temperature:](fg:cyan) %s", tempStr),
		fmt.Sprintf("[Uptime:](fg:cyan) %dd %dh %dm", days, hours, minutes),
		fmt.Sprintf("[Load Average:](fg:cyan) %s", loadAvgStr),
		fmt.Sprintf("[Processes:](fg:cyan) %d", stats.ProcessCount),
	}
}

// updateNetworkInfo updates the network statistics list
func (d *Dashboard) updateNetworkInfo(stats SystemStats) {
	sentDiff := stats.NetSent - d.prevNetSent
	recvDiff := stats.NetRecv - d.prevNetRecv
	d.prevNetSent = stats.NetSent
	d.prevNetRecv = stats.NetRecv

	d.netList.Rows = []string{
		fmt.Sprintf("[Total Sent:](fg:magenta) %.2f MB", bytesToMB(stats.NetSent)),
		fmt.Sprintf("[Total Received:](fg:magenta) %.2f MB", bytesToMB(stats.NetRecv)),
		fmt.Sprintf("[Upload Speed:](fg:magenta) %.2f KB/s", bytesToKB(sentDiff)),
		fmt.Sprintf("[Download Speed:](fg:magenta) %.2f KB/s", bytesToKB(recvDiff)),
	}
}

// updateAllProcessList updates the all process list widget with selection
func (d *Dashboard) updateAllProcessList(stats SystemStats) {
	totalProcesses := len(stats.AllProcesses)
	if totalProcesses == 0 {
		d.allProcessList.Rows = []string{"No processes found"}
		return
	}

	// Ensure selected process is within bounds
	if d.selectedProcess >= totalProcesses {
		d.selectedProcess = totalProcesses - 1
	}
	if d.selectedProcess < 0 {
		d.selectedProcess = 0
	}

	d.allProcessList.Title = fmt.Sprintf("All Processes (Total: %d, Selected: %d)", totalProcesses, d.selectedProcess+1)

	rows := []string{
		fmt.Sprintf("[%-6s](fg:cyan) %-25s [%-6s](fg:red) [%-6s](fg:yellow) [%-3s](fg:white) [%-6s](fg:white) [%-6s](fg:white) [%-10s](fg:white)",
			"PID", "Name", "CPU%", "Mem%", "St", "PPID", "Conns", "User"),
		"─────────────────────────────────────────────────────────────────────────────────────────────────",
	}

	// Calculate visible range based on selection
	visibleHeight := d.allProcessList.Inner.Dy() - 2 // Subtract header rows
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	// Keep selected item in view
	startIdx := 0
	if d.selectedProcess >= visibleHeight {
		startIdx = d.selectedProcess - visibleHeight + 1
	}
	endIdx := startIdx + visibleHeight

	if endIdx > totalProcesses {
		endIdx = totalProcesses
	}
	if startIdx < 0 {
		startIdx = 0
	}

	for i := startIdx; i < endIdx; i++ {
		proc := stats.AllProcesses[i]
		statusColor := getStatusColor(proc.Status)
		
		// Highlight selected process
		if i == d.selectedProcess {
			rows = append(rows,
				fmt.Sprintf("[[%-6d] [%-25s] [%-6.1f] [%-6.1f] [%-3s] [%-6d] [%-6d] [%-10s]](bg:white,fg:black)",
					proc.PID,
					truncateString(proc.Name, 25),
					proc.CPU,
					proc.Memory,
					proc.Status,
					proc.PPID,
					proc.Connections,
					truncateString(proc.Username, 10)))
		} else {
			rows = append(rows,
				fmt.Sprintf("[%-6d](fg:cyan) %-25s [%-6.1f](fg:red) [%-6.1f](fg:yellow) [%-3s](%s) [%-6d](fg:white) [%-6d](fg:white) [%-10s](fg:white)",
					proc.PID,
					truncateString(proc.Name, 25),
					proc.CPU,
					proc.Memory,
					proc.Status,
					statusColor,
					proc.PPID,
					proc.Connections,
					truncateString(proc.Username, 10)))
		}
	}

	d.allProcessList.Rows = rows
}

// Render renders all widgets
func (d *Dashboard) Render() {
	ui.Render(
		d.cpuGauge,
		d.memGauge,
		d.diskGauge,
		d.cpuChart,
		d.infoList,
		d.netList,
		d.allProcessList,
		d.helpParagraph,
	)
}

// EventLoop handles UI events
func (d *Dashboard) EventLoop(ticker *time.Ticker) {
	uiEvents := ui.PollEvents()
	stats := getSystemStats()
	
	for {
		select {
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return
			case "r":
				d.UpdateStats()
				d.Render()
			case "<Up>":
				if d.selectedProcess > 0 {
					d.selectedProcess--
					d.Render()
				}
			case "<Down>":
				stats = getSystemStats()
				if d.selectedProcess < len(stats.AllProcesses)-1 {
					d.selectedProcess++
					d.Render()
				}
			case "<PageUp>":
				d.selectedProcess -= 10
				if d.selectedProcess < 0 {
					d.selectedProcess = 0
				}
				d.Render()
			case "<PageDown>":
				stats = getSystemStats()
				d.selectedProcess += 10
				if d.selectedProcess >= len(stats.AllProcesses) {
					d.selectedProcess = len(stats.AllProcesses) - 1
				}
				d.Render()
			case "<Home>":
				d.selectedProcess = 0
				d.Render()
			case "<End>":
				stats = getSystemStats()
				if len(stats.AllProcesses) > 0 {
					d.selectedProcess = len(stats.AllProcesses) - 1
				}
				d.Render()
			case "<Resize>":
				d.handleResize(e.Payload.(ui.Resize))
			}
		case <-ticker.C:
			d.UpdateStats()
			d.Render()
		}
	}
}

// handleResize handles terminal resize events
func (d *Dashboard) handleResize(resize ui.Resize) {
	width := resize.Width
	height := resize.Height
	halfWidth := width / 2

	infoNetHeight := (height - 13) / 2

	d.cpuGauge.SetRect(0, 0, halfWidth, 3)
	d.memGauge.SetRect(0, 3, halfWidth, 6)
	d.diskGauge.SetRect(0, 6, halfWidth, 9)
	d.cpuChart.SetRect(halfWidth, 0, width, 9)
	d.infoList.SetRect(0, 9, halfWidth, 9+infoNetHeight)
	d.netList.SetRect(halfWidth, 9, width, 9+infoNetHeight)
	d.allProcessList.SetRect(0, 9+infoNetHeight, width, height-4)
	d.helpParagraph.SetRect(0, height-4, width, height)

	ui.Clear()
	d.Render()
}

// getSystemStats collects all system statistics
func getSystemStats() SystemStats {
	stats := SystemStats{}

	if cpuPercents, err := cpu.Percent(0, true); err == nil {
		stats.CPUPercent = cpuPercents
	}

	if memInfo, err := mem.VirtualMemory(); err == nil {
		stats.MemPercent = memInfo.UsedPercent
		stats.MemUsed = memInfo.Used
		stats.MemTotal = memInfo.Total
	}

	if diskInfo, err := disk.Usage("/"); err == nil {
		stats.DiskPercent = diskInfo.UsedPercent
		stats.DiskUsed = diskInfo.Used
		stats.DiskTotal = diskInfo.Total
	}

	stats.Temperature = getCPUTemperature()

	if hostInfo, err := host.Info(); err == nil {
		stats.Uptime = hostInfo.Uptime
		stats.ProcessCount = hostInfo.Procs
	}

	if loadAvg, err := load.Avg(); err == nil {
		stats.LoadAvg = []float64{loadAvg.Load1, loadAvg.Load5, loadAvg.Load15}
	}

	if netStats, err := net.IOCounters(false); err == nil && len(netStats) > 0 {
		stats.NetSent = netStats[0].BytesSent
		stats.NetRecv = netStats[0].BytesRecv
	}

	stats.AllProcesses = getAllProcesses()

	return stats
}

// getCPUTemperature reads CPU temperature from Raspberry Pi thermal zone
func getCPUTemperature() float64 {
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0
	}

	tempStr := strings.TrimSpace(string(data))
	temp, err := strconv.ParseFloat(tempStr, 64)
	if err != nil {
		return 0
	}

	return temp / 1000.0
}

// getHostname returns the system hostname
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// getAllProcesses returns all processes sorted by CPU usage
func getAllProcesses() []ProcessInfo {
	processes, err := process.Processes()
	if err != nil {
		return []ProcessInfo{}
	}

	var processInfos []ProcessInfo
	totalMem, err := mem.VirtualMemory()
	if err != nil {
		return []ProcessInfo{}
	}

	for _, p := range processes {
		name, _ := p.Name()
		cpuPercent, _ := p.CPUPercent()
		memInfo, _ := p.MemoryInfo()
		status, _ := p.Status()
		username, _ := p.Username()
		ppid, _ := p.Ppid()
		connections, _ := p.Connections()

		memPercent := 0.0
		if memInfo != nil && totalMem.Total > 0 {
			memPercent = float64(memInfo.RSS) / float64(totalMem.Total) * 100
		}

		statusStr := "?"
		if len(status) > 0 {
			statusStr = string(status[0])
		}

		// Extract ports from connections
		var ports []uint32
		portMap := make(map[uint32]bool)
		for _, conn := range connections {
			if conn.Laddr.Port > 0 && !portMap[conn.Laddr.Port] {
				ports = append(ports, conn.Laddr.Port)
				portMap[conn.Laddr.Port] = true
			}
		}

		procInfo := ProcessInfo{
			PID:         p.Pid,
			Name:        name,
			CPU:         cpuPercent,
			Memory:      memPercent,
			Status:      statusStr,
			Username:    username,
			PPID:        ppid,
			Connections: len(connections),
			Ports:       ports,
		}

		processInfos = append(processInfos, procInfo)
	}

	// Sort by CPU usage
	sort.Slice(processInfos, func(i, j int) bool {
		return processInfos[i].CPU > processInfos[j].CPU
	})

	return processInfos
}

// Utility functions

func calculateAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func getColorByThreshold(value, mediumThreshold, highThreshold float64) ui.Color {
	if value > highThreshold {
		return ui.ColorRed
	} else if value > mediumThreshold {
		return ui.ColorYellow
	}
	return ui.ColorGreen
}

func getStatusColor(status string) string {
	switch status {
	case "R":
		return "fg:green"
	case "S":
		return "fg:yellow"
	case "Z":
		return "fg:red"
	default:
		return "fg:white"
	}
}

func bytesToGB(bytes uint64) float64 {
	return float64(bytes) / 1024 / 1024 / 1024
}

func bytesToMB(bytes uint64) float64 {
	return float64(bytes) / 1024 / 1024
}

func bytesToKB(bytes uint64) float64 {
	return float64(bytes) / 1024
}

func formatUptime(uptime uint64) (days, hours, minutes uint64) {
	days = uptime / 86400
	hours = (uptime % 86400) / 3600
	minutes = (uptime % 3600) / 60
	return
}

func formatLoadAverage(loadAvg []float64) string {
	if len(loadAvg) >= 3 {
		return fmt.Sprintf("%.2f, %.2f, %.2f", loadAvg[0], loadAvg[1], loadAvg[2])
	}
	return "N/A"
}

func formatTemperature(temp float64) string {
	if temp > 0 {
		return fmt.Sprintf("%.1f°C", temp)
	}
	return "N/A"
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}