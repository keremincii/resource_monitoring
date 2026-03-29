package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// StatInt represents a monitored integer metric and its maximums
type StatInt struct {
	Max     int
	MaxTime string
}

func (s *StatInt) UpdateMax(val int, nowStr string) {
	if s.Max == 0 || val > s.Max {
		s.Max = val
		s.MaxTime = nowStr
	}
}

func (s *StatInt) Reset() {
	s.Max = 0
	s.MaxTime = ""
}

// StatFloat represents a monitored float metric and its maximums
type StatFloat struct {
	Max     float64
	MaxTime string
}

func (s *StatFloat) UpdateMax(val float64, nowStr string) {
	if s.Max == 0 || val > s.Max {
		s.Max = val
		s.MaxTime = nowStr
	}
}

func (s *StatFloat) Reset() {
	s.Max = 0
	s.MaxTime = ""
}

// MonitorStats holds all tracking variables
type MonitorStats struct {
	CPU      StatFloat
	RAM      StatFloat
	Conn     StatInt
	PPSIn    StatInt
	PPSDrop  StatInt
	PPSSq    StatInt
}

func (m *MonitorStats) Reset() {
	m.CPU.Reset()
	m.RAM.Reset()
	m.Conn.Reset()
	m.PPSIn.Reset()
	m.PPSDrop.Reset()
	m.PPSSq.Reset()
}

var (
	stats1h  = MonitorStats{}
	stats24h = MonitorStats{}

	tStart1h  time.Time
	tStart24h time.Time
	lastLog   time.Time
	loc       *time.Location

	prevTotal, prevDrop, prevSq int = -1, -1, -1
)

const logPath = "/tmp/resource.log"

func main() {
	// 1. Saat dilimi ayarı (TRT)
	var err error
	loc, err = time.LoadLocation("Europe/Istanbul")
	if err != nil {
		loc = time.FixedZone("TRT", 3*3600)
	}

	tStart1h = time.Now().In(loc)
	tStart24h = time.Now().In(loc)
	lastLog = time.Now().In(loc)
	prevTickTime := time.Now().In(loc)

	// Isınma turu için ilk okuma
	getSoftnetStats()
	getCPUUsage()

	// 0.5 Saniyelik Ticker
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().In(loc)
		nowStr := now.Format("15:04:05")
		
		// Geçen tam süreyi (milisaniye hassasiyetinde) hesapla
		durationSec := now.Sub(prevTickTime).Seconds()
		if durationSec <= 0 {
			durationSec = 0.5 // Sıfıra bölünme koruması
		}
		prevTickTime = now

		// 1. Verileri Topla
		cpu := getCPUUsage()
		ram := getRamUsage()
		conns := getConnectionCount()
		total, drop, sq := getSoftnetStats()

		// PPS Hesapla (Kusursuz ölçüm için geçen gerçek zamana bölüyoruz)
		ppsIn, ppsDrop, ppsSq := 0, 0, 0
		if prevTotal != -1 {
			ppsIn = int(float64(total - prevTotal) / durationSec)
			ppsDrop = int(float64(drop - prevDrop) / durationSec)
			ppsSq = int(float64(sq - prevSq) / durationSec)
		}
		
		// Negatif koruması
		if ppsIn < 0 { ppsIn = 0 }
		if ppsDrop < 0 { ppsDrop = 0 }
		if ppsSq < 0 { ppsSq = 0 }

		prevTotal, prevDrop, prevSq = total, drop, sq

		// 2. MAX Güncelleme
		stats1h.CPU.UpdateMax(cpu, nowStr)
		stats24h.CPU.UpdateMax(cpu, nowStr)

		stats1h.RAM.UpdateMax(ram, nowStr)
		stats24h.RAM.UpdateMax(ram, nowStr)

		stats1h.Conn.UpdateMax(conns, nowStr)
		stats24h.Conn.UpdateMax(conns, nowStr)

		stats1h.PPSIn.UpdateMax(ppsIn, nowStr)
		stats24h.PPSIn.UpdateMax(ppsIn, nowStr)

		stats1h.PPSDrop.UpdateMax(ppsDrop, nowStr)
		stats24h.PPSDrop.UpdateMax(ppsDrop, nowStr)

		stats1h.PPSSq.UpdateMax(ppsSq, nowStr)
		stats24h.PPSSq.UpdateMax(ppsSq, nowStr)

		// 3. Sıfırlama Mantığı (1H)
		if time.Since(tStart1h).Hours() >= 1 {
			stats1h.Reset()
			tStart1h = time.Now().In(loc)
		}

		// 4. Sıfırlama Mantığı (24H)
		if time.Since(tStart24h).Hours() >= 24 {
			stats24h.Reset()
			tStart24h = time.Now().In(loc)
		}

		// 5. Log Yazma (Akıcı olması için ticker ile uyumlu 0.4 sn seçildi)
		if time.Since(lastLog).Seconds() >= 0.4 {
			writeDashboardLog(nowStr, cpu, ram, conns, ppsIn, ppsDrop, ppsSq)
			lastLog = time.Now().In(loc)
		}
	}
}

func writeDashboardLog(nowStr string, cpu, ram float64, conns, ppsIn, ppsDrop, ppsSq int) {
	tempPath := logPath + ".tmp"
	
	var sb strings.Builder
	
	// Helper for safe formatting
	safeTime := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}

	sb.WriteString("======================================================================\n")
	sb.WriteString("                    R E S O U R C E   M O N I T O R                   \n")
	sb.WriteString("======================================================================\n")
	sb.WriteString(fmt.Sprintf("[%s] | GÜNCEL DURUM\n", time.Now().In(loc).Format("2006-01-02 15:04:05")))
	sb.WriteString("----------------------------------------------------------------------\n")
	sb.WriteString(fmt.Sprintf("CPU  : %6.1f%%       |  PPS IN  : %6d pkt/s\n", cpu, ppsIn))
	sb.WriteString(fmt.Sprintf("RAM  : %6.1f%%       |  DROP    : %6d pkt/s\n", ram, ppsDrop))
	sb.WriteString(fmt.Sprintf("CONN : %6d        |  SQUEEZE : %6d \n", conns, ppsSq))
	sb.WriteString("----------------------------------------------------------------------\n")
	sb.WriteString("[ 1 SAATLİK ZİRVELER ]                [ 24 SAATLİK ZİRVELER ]\n")
	
	sb.WriteString(fmt.Sprintf("CPU  : %6.1f%% (%-8s)            CPU  : %6.1f%% (%-8s)\n", 
		stats1h.CPU.Max, safeTime(stats1h.CPU.MaxTime), stats24h.CPU.Max, safeTime(stats24h.CPU.MaxTime)))
		
	sb.WriteString(fmt.Sprintf("RAM  : %6.1f%% (%-8s)            RAM  : %6.1f%% (%-8s)\n", 
		stats1h.RAM.Max, safeTime(stats1h.RAM.MaxTime), stats24h.RAM.Max, safeTime(stats24h.RAM.MaxTime)))
		
	sb.WriteString(fmt.Sprintf("CONN : %6d (%-8s)            CONN : %6d (%-8s)\n", 
		stats1h.Conn.Max, safeTime(stats1h.Conn.MaxTime), stats24h.Conn.Max, safeTime(stats24h.Conn.MaxTime)))

	sb.WriteString(fmt.Sprintf("PPS  : %6d (%-8s)            PPS  : %6d (%-8s)\n", 
		stats1h.PPSIn.Max, safeTime(stats1h.PPSIn.MaxTime), stats24h.PPSIn.Max, safeTime(stats24h.PPSIn.MaxTime)))
		
	sb.WriteString(fmt.Sprintf("DROP : %6d (%-8s)            DROP : %6d (%-8s)\n", 
		stats1h.PPSDrop.Max, safeTime(stats1h.PPSDrop.MaxTime), stats24h.PPSDrop.Max, safeTime(stats24h.PPSDrop.MaxTime)))
		
	sb.WriteString(fmt.Sprintf("SQZ  : %6d (%-8s)            SQZ  : %6d (%-8s)\n", 
		stats1h.PPSSq.Max, safeTime(stats1h.PPSSq.MaxTime), stats24h.PPSSq.Max, safeTime(stats24h.PPSSq.MaxTime)))
		
	sb.WriteString("======================================================================\n")

	// Atomik yazdırma işlemi (Titremeyi önler)
	err := os.WriteFile(tempPath, []byte(sb.String()), 0644)
	if err == nil {
		os.Rename(tempPath, logPath)
	}
}

// Linux /proc/net/sockstat okur
func getConnectionCount() int {
	data, err := os.ReadFile("/proc/net/sockstat")
	if err != nil {
		return 0
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TCP:") {
			fields := strings.Fields(line)
			if len(fields) > 2 {
				val, _ := strconv.Atoi(fields[2])
				return val
			}
		}
	}
	return 0
}

// Linux /proc/net/softnet_stat okur
func getSoftnetStats() (int, int, int) {
	data, err := os.ReadFile("/proc/net/softnet_stat")
	if err != nil {
		return 0, 0, 0
	}

	total, drop, sq := 0, 0, 0
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			t, _ := strconv.ParseInt(fields[0], 16, 64)
			d, _ := strconv.ParseInt(fields[1], 16, 64)
			s, _ := strconv.ParseInt(fields[2], 16, 64)
			total += int(t)
			drop += int(d)
			sq += int(s)
		}
	}
	return total, drop, sq
}

// Basit RAM kullanımı (/proc/meminfo)
func getRamUsage() float64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}

	var total, available float64
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			total, _ = strconv.ParseFloat(fields[1], 64)
		}
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			available, _ = strconv.ParseFloat(fields[1], 64)
		}
	}
	if total > 0 {
		return 100 * (1 - (available / total))
	}
	return 0
}

var prevIdleTime, prevTotalTime float64

// /proc/stat okuyarak CPU hesaplar (psutil olmadan)
func getCPUUsage() float64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}

	lines := strings.Split(string(data), "\n")
	fields := strings.Fields(lines[0])

	if len(fields) < 5 {
		return 0
	}

	idle, _ := strconv.ParseFloat(fields[4], 64)
	total := 0.0
	for _, val := range fields[1:] {
		v, _ := strconv.ParseFloat(val, 64)
		total += v
	}

	diffIdle := idle - prevIdleTime
	diffTotal := total - prevTotalTime

	prevIdleTime = idle
	prevTotalTime = total

	if diffTotal == 0 {
		return 0
	}
	return 100 * (1 - (diffIdle / diffTotal))
}
