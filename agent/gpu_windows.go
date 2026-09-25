//go:build windows

package main

import (
	"context"
	"encoding/json"
	"math"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

var gpuState struct {
	sync.Mutex
	started bool
	value   *float64
}

// Sample outside the heartbeat path: a slow WMI provider must not delay reconnects.
func currentGPU() *float64 {
	gpuState.Lock()
	defer gpuState.Unlock()
	if !gpuState.started {
		gpuState.started = true
		go func() {
			for {
				value := sampleGPU()
				gpuState.Lock()
				gpuState.value = value
				gpuState.Unlock()
				time.Sleep(5 * time.Second)
			}
		}()
	}
	return gpuState.value
}

type gpuEngine struct {
	Name                  string
	UtilizationPercentage float64
}

func sampleGPU() *float64 {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		"$ErrorActionPreference='Stop'; $rows=@(Get-CimInstance Win32_PerfFormattedData_GPUPerformanceCounters_GPUEngine | Select-Object Name,UtilizationPercentage); ConvertTo-Json -InputObject $rows -Compress")
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	raw, err := cmd.Output()
	if err != nil {
		return nil
	}
	var rows []gpuEngine
	if json.Unmarshal(raw, &rows) != nil {
		return nil
	}
	return busiestGPU(rows)
}

// Sum processes sharing the same adapter/engine, then report the busiest engine.
// Adding different engines together would overstate total utilization.
func busiestGPU(rows []gpuEngine) *float64 {
	engines := map[string]float64{}
	for _, row := range rows {
		i := strings.Index(row.Name, "luid_")
		if i < 0 || math.IsNaN(row.UtilizationPercentage) || math.IsInf(row.UtilizationPercentage, 0) {
			continue
		}
		engines[row.Name[i:]] += math.Max(0, row.UtilizationPercentage)
	}
	if len(engines) == 0 {
		return nil
	}
	value := 0.0
	for _, usage := range engines {
		value = math.Max(value, usage)
	}
	value = math.Min(100, value)
	return &value
}
