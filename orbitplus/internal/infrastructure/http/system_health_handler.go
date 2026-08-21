package http

import (
	"net/http"
	"runtime"
	"runtime/metrics"
	"sync"
	"time"
)

// SystemHealthHandler reports protected Go process runtime health for the admin dashboard.
type SystemHealthHandler struct {
	startedAt    time.Time
	mutex        sync.Mutex
	lastTotalCPU float64
	lastIdleCPU  float64
	lastAt       time.Time
}

// NewSystemHealthHandler constructs the protected process-health endpoint.
func NewSystemHealthHandler(startedAt time.Time) *SystemHealthHandler {
	return &SystemHealthHandler{startedAt: startedAt.UTC()}
}

// ServeHTTP returns a snapshot of the current Go process runtime state.
func (handler *SystemHealthHandler) ServeHTTP(response http.ResponseWriter, _ *http.Request) {
	writeJSONData(response, handler.snapshot(time.Now().UTC()))
}

func (handler *SystemHealthHandler) snapshot(now time.Time) systemHealthResponse {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return systemHealthResponse{
		Status:                  "UP",
		StartedAt:               handler.startedAt,
		UptimeSeconds:           uptimeSeconds(handler.startedAt, now),
		GoCPUCapacityPercent:    handler.goCPUCapacityPercent(now),
		GoVersion:               runtime.Version(),
		OperatingSystem:         runtime.GOOS,
		Architecture:            runtime.GOARCH,
		LogicalCPUs:             runtime.NumCPU(),
		GoMaxProcs:              runtime.GOMAXPROCS(0),
		Goroutines:              runtime.NumGoroutine(),
		HeapAllocBytes:          memory.HeapAlloc,
		HeapInUseBytes:          memory.HeapInuse,
		HeapSystemBytes:         memory.HeapSys,
		GoRuntimeSystemBytes:    memory.Sys,
		HeapObjects:             memory.HeapObjects,
		GCCycles:                memory.NumGC,
		LastGCAt:                optionalSystemHealthTime(memory.LastGC),
		TotalGCPauseNanoseconds: memory.PauseTotalNs,
		NextGCTargetBytes:       memory.NextGC,
	}
}

type systemHealthResponse struct {
	Status                  string     `json:"status"`
	StartedAt               time.Time  `json:"startedAt"`
	UptimeSeconds           int64      `json:"uptimeSeconds"`
	GoCPUCapacityPercent    *float64   `json:"goCpuCapacityPercent"`
	GoVersion               string     `json:"goVersion"`
	OperatingSystem         string     `json:"operatingSystem"`
	Architecture            string     `json:"architecture"`
	LogicalCPUs             int        `json:"logicalCPUs"`
	GoMaxProcs              int        `json:"goMaxProcs"`
	Goroutines              int        `json:"goroutines"`
	HeapAllocBytes          uint64     `json:"heapAllocBytes"`
	HeapInUseBytes          uint64     `json:"heapInUseBytes"`
	HeapSystemBytes         uint64     `json:"heapSystemBytes"`
	GoRuntimeSystemBytes    uint64     `json:"goRuntimeSystemBytes"`
	HeapObjects             uint64     `json:"heapObjects"`
	GCCycles                uint32     `json:"gcCycles"`
	LastGCAt                *time.Time `json:"lastGCAt"`
	TotalGCPauseNanoseconds uint64     `json:"totalGCPauseNanoseconds"`
	NextGCTargetBytes       uint64     `json:"nextGCTargetBytes"`
}

func (handler *SystemHealthHandler) goCPUCapacityPercent(now time.Time) *float64 {
	samples := []metrics.Sample{
		{Name: "/cpu/classes/total:cpu-seconds"},
		{Name: "/cpu/classes/idle:cpu-seconds"},
	}
	metrics.Read(samples)
	if samples[0].Value.Kind() != metrics.KindFloat64 || samples[1].Value.Kind() != metrics.KindFloat64 {
		return nil
	}
	currentTotal := samples[0].Value.Float64()
	currentIdle := samples[1].Value.Float64()
	handler.mutex.Lock()
	defer handler.mutex.Unlock()
	if handler.lastAt.IsZero() {
		handler.lastAt = now
		handler.lastTotalCPU = currentTotal
		handler.lastIdleCPU = currentIdle
		return nil
	}
	available := currentTotal - handler.lastTotalCPU
	idle := currentIdle - handler.lastIdleCPU
	handler.lastAt = now
	handler.lastTotalCPU = currentTotal
	handler.lastIdleCPU = currentIdle
	if available <= 0 || idle < 0 {
		return nil
	}
	used := available - idle
	if used < 0 {
		used = 0
	}
	percent := used / available * 100
	if percent > 100 {
		percent = 100
	}
	return &percent
}

func uptimeSeconds(startedAt, now time.Time) int64 {
	if now.Before(startedAt) {
		return 0
	}
	return int64(now.Sub(startedAt).Seconds())
}

func optionalSystemHealthTime(unixNanoseconds uint64) *time.Time {
	if unixNanoseconds == 0 {
		return nil
	}
	value := time.Unix(0, int64(unixNanoseconds)).UTC()
	return &value
}
