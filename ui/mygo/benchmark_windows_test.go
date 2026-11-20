package desktop

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"goDisk/internal/domain"
)

func TestBenchmarkGridUsesCompletedRoundsAndReadsLegacyHistory(t *testing.T) {
	workload := domain.Workload{Name: domain.Sequential, Queue: 1, Threads: 1}
	legacy := domain.Score{Profile: domain.Sequential + " 读 · 1/1", Seconds: 2, Bytes: 20_000_000, Complete: true}
	recent := domain.Score{Workload: workload.Key(), Seconds: 1, Bytes: 7_000_000, Complete: true}
	partial := domain.Score{Workload: workload.Key(), Seconds: 1, Bytes: 99_000_000}
	value, ok, incomplete := scoreValue([]domain.Score{legacy, recent, partial}, workload, false, "MB/s", false)
	if !ok || incomplete || value != 9 {
		t.Fatalf("unexpected complete result: %v %v %v", value, ok, incomplete)
	}
	value, ok, incomplete = scoreValue([]domain.Score{legacy, recent, partial}, workload, false, "MB/s", true)
	if !ok || incomplete || value != 10 {
		t.Fatalf("expected the fastest completed round: %v %v %v", value, ok, incomplete)
	}
	value, ok, incomplete = scoreValue([]domain.Score{partial}, workload, false, "MB/s", true)
	if !ok || !incomplete || value != 99 {
		t.Fatalf("unexpected partial result: %v %v %v", value, ok, incomplete)
	}
	if _, ok, _ := scoreValue([]domain.Score{legacy, recent}, workload, true, "MB/s", true); ok {
		t.Fatal("read results leaked into the write column")
	}
}

func TestBenchmarkPageAtMinimumWidth(t *testing.T) {
	config := domain.BenchmarkConfig{Mode: domain.Both, Standard: true, SizeMiB: domain.DefaultBenchmarkSizeMiB, Repetitions: 1}
	first := domain.Workloads(config)[0]
	d := &desktop{page: 1, unit: "MB/s", mode: config.Mode, sizeLabel: domain.BenchmarkSizeLabel(config.SizeMiB), repeat: 1,
		bench: domain.BenchmarkResult{Config: config, Started: time.Now(), State: "completed", Scores: []domain.Score{
			{Workload: first.Key(), Seconds: 1, Bytes: 3_200_000_000, Complete: true},
		}},
	}
	view := ui.NewTester(d.view, windowWidth, 490)
	for _, label := range []string{"1 GiB", "读取 · MB/s", "写入 · MB/s", "Q8 · T1", "Q32 · T1", "3200.00", "未启用"} {
		bounds, found := view.Find(label)
		if !found || bounds.X < 0 || bounds.Y < 0 || bounds.X+bounds.W > windowWidth || bounds.Y+bounds.H > 490 {
			t.Errorf("missing or clipped benchmark label %q: %v", label, bounds)
		}
	}
	if view.HasText("选择工具") {
		t.Error("the bundled benchmark engine should not require a picker")
	}
	if got := selectedFileSize("64 GiB"); got != 65536 || selectedFileSize("100 MiB") != 0 {
		t.Fatalf("unexpected size selection: %d", got)
	}
}

func TestHistoryShowsFirstCompletedSequentialRead(t *testing.T) {
	config := domain.BenchmarkConfig{Mode: domain.Both, Standard: true, PeakRounds: true}
	workloads := domain.Workloads(config)
	result := domain.BenchmarkResult{Config: config, Started: time.Now(), Scores: []domain.Score{
		{Workload: workloads[0].Key(), Seconds: 1, Bytes: 3_200_000_000, Complete: true},
		{Workload: workloads[0].Key(), Seconds: 1, Bytes: 7_000_000_000},
		{Workload: workloads[0].Key(), Seconds: 1, Bytes: 8_000_000_000, Complete: true, Write: true},
		{Workload: workloads[1].Key(), Seconds: 1, Bytes: 4_000_000_000, Complete: true},
	}}
	entry := benchmarkHistoryEntry{at: result.Started, label: `E:\`, result: result}
	d := &desktop{page: 2, history: []benchmarkHistoryEntry{entry}}
	view := ui.NewTester(d.view, windowWidth, windowHeight)
	label := entry.at.Local().Format("2006-01-02 15:04:05") + `  ·  E:\  ·  顺序读取 Q8T1 3200.00 MB/s`
	if !view.HasText(label) {
		t.Fatalf("missing history row: %q", label)
	}
	if err := view.Click(label); err != nil || d.page != 1 || !d.bench.Started.Equal(result.Started) {
		t.Fatalf("opening the history row did not restore its benchmark: %v", err)
	}
	result.Config.Mode = domain.Random
	if got := firstSequentialRead(result); got != "—" {
		t.Fatalf("random-only benchmark displayed a sequential result: %s", got)
	}
}
