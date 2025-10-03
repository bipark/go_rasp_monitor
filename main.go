package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
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
	"github.com/shirou/gopsutil/v3/mem"
	gopsnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
	"net"
)

const (
	updateInterval = time.Second
	historySize    = 20
	
	// GPIO Pin definitions for buttons (BCM numbering)
	// Based on your hardware configuration
	buttonUp     = 3   // KEY1 - Up
	buttonDown   = 5   // KEY2 - Down
	buttonLeft   = 6   // KEY3 - Left
	buttonRight  = 16  // KEY4 - Right
	buttonA      = 13  // KEY5 - A button
	buttonB      = 26  // KEY6 - B button
	buttonX      = 19  // KEY7 - X button
	buttonY      = 21  // KEY8 - Y button
	buttonStart  = 20  // KEY9 - Start
	buttonSelect = 15  // KEY10 - Select
	buttonL      = 12  // KEY11 - L button
	buttonR      = 14  // KEY12 - R button
	buttonCenter = 23  // KEY13 - Center/Menu
)

type ProcessInfo struct {
	PID      int32
	Name     string
	CPU      float64
	Memory   float64
	Status   string
	Username string
}

type SystemStats struct {
	CPUPercent   []float64
	MemPercent   float64
	MemUsed      uint64
	MemTotal     uint64
	DiskPercent  float64
	Temperature  float64
	Uptime       uint64
	NetSent      uint64
	NetRecv      uint64
	ProcessCount uint64
	AllProcesses []ProcessInfo
	IPAddress    string
	APMode       string
}

type Dashboard struct {
	mainList        *widgets.List
	helpParagraph   *widgets.Paragraph
	currentView     int // 0: System info, 1: Process, 2: Network
	selectedProcess int
	prevNetSent     uint64
	prevNetRecv     uint64
	
	// Button press tracking
	lastButtonState map[int]int
	gpioEnabled     bool // Track if GPIO is available
}

func main() {
	// Setup log file
	logFile, err := os.OpenFile("raspi-monitor.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
	} else {
		defer logFile.Close()
		log.SetOutput(logFile)
	}
	
	log.Println("=== Raspi Monitor Started ===")
	
	// Check if gpioget command is available (for Raspberry Pi 5)
	gpioAvailable := false
	
	// Check for gpioget at absolute path
	if _, err := os.Stat("/usr/bin/gpioget"); err == nil {
		gpioAvailable = true
		log.Println("GPIO initialized successfully (using gpiochip0)")
	} else {
		log.Println("Warning: gpioget not found at /usr/bin/gpioget")
		log.Println("Install with: sudo apt-get install gpiod")
		log.Println("Running without button support.")
	}

	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

	dashboard := NewDashboard()
	dashboard.InitWidgets()
	if gpioAvailable {
		dashboard.InitGPIO()
	} else {
		log.Println("Button controls disabled. Use keyboard: TAB=switch, q=quit, arrows=navigate")
	}
	dashboard.UpdateStats()
	dashboard.Render()

	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()

	dashboard.EventLoop(ticker)
}

func NewDashboard() *Dashboard {
	return &Dashboard{
		currentView:     0,
		selectedProcess: 0,
		lastButtonState: make(map[int]int),
		gpioEnabled:     false,
	}
}

func (d *Dashboard) InitWidgets() {
	// Main list - full screen utilization
	d.mainList = widgets.NewList()
	d.mainList.Title = "System Monitor"
	d.mainList.SetRect(0, 0, 30, 30) // 240x240 = approx 30x30 chars
	d.mainList.TextStyle = ui.NewStyle(ui.ColorWhite)
	d.mainList.BorderStyle = ui.NewStyle(ui.ColorCyan)

	// Bottom help - removed to save space
	d.helpParagraph = widgets.NewParagraph()
	d.helpParagraph.Title = ""
	d.helpParagraph.Text = ""
	d.helpParagraph.SetRect(0, 30, 30, 30)
	d.helpParagraph.BorderStyle = ui.NewStyle(ui.ColorYellow)
}

// InitGPIO initializes GPIO pins for button input using gpioget
func (d *Dashboard) InitGPIO() {
	log.Println("Initializing GPIO pins via gpiochip0...")
	
	// Initialize last button states (all HIGH/1 initially with pull-up)
	pins := []int{buttonUp, buttonDown, buttonLeft, buttonRight, 
		buttonA, buttonB, buttonX, buttonY, 
		buttonStart, buttonSelect, buttonL, buttonR, buttonCenter}
	
	for _, pin := range pins {
		d.lastButtonState[pin] = 1 // HIGH = not pressed
	}
	
	d.gpioEnabled = true
	log.Println("GPIO ready - press buttons to test")
}

// readGPIOValue reads the current value of a GPIO pin using gpioget
func readGPIOValue(pin int) int {
	cmd := exec.Command("/usr/bin/gpioget", "gpiochip0", fmt.Sprintf("%d", pin))
	output, err := cmd.Output()
	if err != nil {
		return 1 // Default to HIGH on error
	}
	
	value := strings.TrimSpace(string(output))
	if value == "0" {
		return 0 // LOW = pressed
	}
	return 1 // HIGH = not pressed
}

func (d *Dashboard) UpdateStats() {
	stats := getSystemStats()

	switch d.currentView {
	case 0:
		d.updateSystemView(stats)
	case 1:
		d.updateProcessView(stats)
	case 2:
		d.updateNetworkView(stats)
	}
}

func (d *Dashboard) updateSystemView(stats SystemStats) {
	avgCPU := calculateAverage(stats.CPUPercent)
	days, hours, _ := formatUptime(stats.Uptime)
	tempStr := formatTemperature(stats.Temperature)

	d.mainList.Title = "System (1/3) [A/B:Switch]"
	d.mainList.Rows = []string{
		"",
		fmt.Sprintf("[CPU:](fg:cyan) %.1f%%", avgCPU),
		getBar(avgCPU, 20),
		"",
		fmt.Sprintf("[MEM:](fg:yellow) %.1f%%", stats.MemPercent),
		getBar(stats.MemPercent, 20),
		"",
		fmt.Sprintf("[DSK:](fg:magenta) %.1f%%", stats.DiskPercent),
		getBar(stats.DiskPercent, 20),
		"",
		"[--System Info--](fg:white)",
		fmt.Sprintf("Temp: %s", tempStr),
		fmt.Sprintf("Uptime: %dd %dh", days, hours),
		fmt.Sprintf("Cores: %d", runtime.NumCPU()),
		fmt.Sprintf("Procs: %d", stats.ProcessCount),
		"",
		"[--Network Info--](fg:green)",
		fmt.Sprintf("IP: %s", stats.IPAddress),
		fmt.Sprintf("Mode: %s", stats.APMode),
		"",
	}
}

func (d *Dashboard) updateProcessView(stats SystemStats) {
	totalProcesses := len(stats.AllProcesses)
	if totalProcesses == 0 {
		d.mainList.Rows = []string{"No processes found"}
		return
	}

	if d.selectedProcess >= totalProcesses {
		d.selectedProcess = totalProcesses - 1
	}
	if d.selectedProcess < 0 {
		d.selectedProcess = 0
	}

	d.mainList.Title = fmt.Sprintf("Process (2/3) %d/%d [↑↓:Move]", d.selectedProcess+1, totalProcesses)

	rows := []string{
		"[PID   Name         CPU%](fg:cyan)",
		"---------------------------",
	}

	// Visible processes count (about 27 lines)
	visibleHeight := 27
	startIdx := d.selectedProcess
	if startIdx > totalProcesses-visibleHeight {
		startIdx = totalProcesses - visibleHeight
	}
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := startIdx + visibleHeight
	if endIdx > totalProcesses {
		endIdx = totalProcesses
	}

	for i := startIdx; i < endIdx; i++ {
		proc := stats.AllProcesses[i]
		name := truncateString(proc.Name, 12)
		
		if i == d.selectedProcess {
			rows = append(rows,
				fmt.Sprintf("[[%-5d] [%-12s] [%4.1f]](bg:white,fg:black)",
					proc.PID, name, proc.CPU))
		} else {
			rows = append(rows,
				fmt.Sprintf("[%-5d](fg:cyan) %-12s [%4.1f](fg:red)",
					proc.PID, name, proc.CPU))
		}
	}

	d.mainList.Rows = rows
}

func (d *Dashboard) updateNetworkView(stats SystemStats) {
	sentDiff := stats.NetSent - d.prevNetSent
	recvDiff := stats.NetRecv - d.prevNetRecv
	d.prevNetSent = stats.NetSent
	d.prevNetRecv = stats.NetRecv

	d.mainList.Title = "Network (3/3) [A/B:Switch]"
	d.mainList.Rows = []string{
		"",
		"[--Total Transfer--](fg:cyan)",
		"",
		fmt.Sprintf("Total Upload:"),
		fmt.Sprintf("  %.1f MB", bytesToMB(stats.NetSent)),
		"",
		fmt.Sprintf("Total Download:"),
		fmt.Sprintf("  %.1f MB", bytesToMB(stats.NetRecv)),
		"",
		"[--Current Speed--](fg:magenta)",
		"",
		fmt.Sprintf("Upload:"),
		fmt.Sprintf("  %.1f KB/s", bytesToKB(sentDiff)),
		"",
		fmt.Sprintf("Download:"),
		fmt.Sprintf("  %.1f KB/s", bytesToKB(recvDiff)),
		"",
	}
}

func (d *Dashboard) Render() {
	ui.Render(d.mainList)
}

func (d *Dashboard) EventLoop(ticker *time.Ticker) {
	uiEvents := ui.PollEvents()
	stats := getSystemStats()
	
	for {
		select {
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return
			case "<Tab>":
				d.currentView = (d.currentView + 1) % 3
				d.UpdateStats()
				d.Render()
			case "<Up>":
				if d.currentView == 1 && d.selectedProcess > 0 {
					d.selectedProcess--
					d.UpdateStats()
					d.Render()
				}
			case "<Down>":
				if d.currentView == 1 {
					stats = getSystemStats()
					if d.selectedProcess < len(stats.AllProcesses)-1 {
						d.selectedProcess++
						d.UpdateStats()
						d.Render()
					}
				}
			case "<Resize>":
				d.handleResize(e.Payload.(ui.Resize))
			}
		case <-ticker.C:
			d.UpdateStats()
			d.Render()
		}
	}
}

func (d *Dashboard) handleResize(resize ui.Resize) {
	width := resize.Width
	height := resize.Height
	
	d.mainList.SetRect(0, 0, width, height)
	
	ui.Clear()
	d.Render()
}

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
	}

	stats.Temperature = getCPUTemperature()

	if hostInfo, err := host.Info(); err == nil {
		stats.Uptime = hostInfo.Uptime
		stats.ProcessCount = hostInfo.Procs
	}

	if netStats, err := gopsnet.IOCounters(false); err == nil && len(netStats) > 0 {
		stats.NetSent = netStats[0].BytesSent
		stats.NetRecv = netStats[0].BytesRecv
	}

	stats.AllProcesses = getAllProcesses()
	stats.IPAddress = getIPAddress()
	stats.APMode = getAPMode()

	return stats
}

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

		memPercent := 0.0
		if memInfo != nil && totalMem.Total > 0 {
			memPercent = float64(memInfo.RSS) / float64(totalMem.Total) * 100
		}

		statusStr := "?"
		if len(status) > 0 {
			statusStr = string(status[0])
		}

		procInfo := ProcessInfo{
			PID:      p.Pid,
			Name:     name,
			CPU:      cpuPercent,
			Memory:   memPercent,
			Status:   statusStr,
			Username: username,
		}

		processInfos = append(processInfos, procInfo)
	}

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

func getBar(percent float64, width int) string {
	filled := int(percent / 100.0 * float64(width))
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled)
	empty := strings.Repeat("░", width-filled)
	return fmt.Sprintf("[%s%s](fg:green)", bar, empty)
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
	return s[:maxLen-2] + ".."
}

// getIPAddress returns the current IP address
func getIPAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "N/A"
	}

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			// Return first non-loopback IPv4 address
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}

	return "No IP"
}

// getAPMode checks if the system is in AP mode
func getAPMode() string {
	// Check for hostapd process (common AP mode daemon)
	processes, err := process.Processes()
	if err != nil {
		return "Unknown"
	}

	for _, p := range processes {
		name, err := p.Name()
		if err != nil {
			continue
		}
		
		if strings.Contains(strings.ToLower(name), "hostapd") {
			return "AP Mode"
		}
	}

	// Check for wlan0 in master mode (alternative check)
	data, err := os.ReadFile("/sys/class/net/wlan0/operstate")
	if err == nil {
		state := strings.TrimSpace(string(data))
		if state == "up" {
			// Additional check for AP mode via iwconfig or similar
			return "Client Mode"
		}
	}

	return "Client Mode"
}