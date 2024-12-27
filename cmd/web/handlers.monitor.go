package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"talent-rodeo/cmd/web/pages"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"

	datastar "github.com/starfederation/datastar/sdk/go"
)

type SystemMonitorSignals struct {
	MemTotal       string `json:"memTotal,omitempty"`
	MemUsed        string `json:"memUsed,omitempty"`
	MemUsedPercent string `json:"memUsedPercent,omitempty"`
	CpuUser        string `json:"cpuUser,omitempty"`
	CpuSystem      string `json:"cpuSystem,omitempty"`
	CpuIdle        string `json:"cpuIdle,omitempty"`
}

func (app *application) monitor(w http.ResponseWriter, r *http.Request) {
	pages.MonitorInitial().Render(r.Context(), w)
}

func (app *application) monitorEvents(w http.ResponseWriter, r *http.Request) {
	memT := time.NewTicker(time.Second)
	defer memT.Stop()

	cpuT := time.NewTicker(time.Second)
	defer cpuT.Stop()

	sse := datastar.NewSSE(w, r)
	for {
		select {
		case <-r.Context().Done():
			app.logger.Debug("client disconnected")
			return

		case <-memT.C:
			m, err := mem.VirtualMemory()
			if err != nil {
				app.logger.Error("unable to get mem stats", slog.String("error", err.Error()))
				return
			}

			memStats := SystemMonitorSignals{
				MemTotal:       fmt.Sprintf("%d", m.Total),
				MemUsed:        fmt.Sprintf("%d", m.Used),
				MemUsedPercent: fmt.Sprintf("%.2f%%", m.UsedPercent),
			}

			sse.MarshalAndMergeSignals(memStats)

		case <-cpuT.C:
			c, err := cpu.Times(false)
			if err != nil {
				app.logger.Error("unable to get cpu stats", slog.String("error", err.Error()))
				return
			}

			cpuStats := SystemMonitorSignals{
				CpuUser:   fmt.Sprintf("%.2f", c[0].User),
				CpuSystem: fmt.Sprintf("%.2f", c[0].System),
				CpuIdle:   fmt.Sprintf("%.2f", c[0].Idle),
			}

			sse.MarshalAndMergeSignals(cpuStats)
		}
	}
}
