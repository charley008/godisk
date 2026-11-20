package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"goDisk/internal/domain"
	"goDisk/internal/storage"
)

func TestCaptureAndHistoryRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "history.sqlite")
	store, err := storage.OpenAt(path)
	if err != nil {
		t.Fatal(err)
	}
	app := App{Store: store}
	page := make([]byte, 512)
	page[1], page[2], page[3], page[4] = 62, 1, 100, 10
	original := domain.Health{At: time.Now(), Protocol: "NVMe", Capture: page}
	captureFile := filepath.Join(dir, "capture.json")
	if err := app.ExportHealth(captureFile, original); err != nil {
		t.Fatal(err)
	}
	if err := app.ExportHealth(captureFile, original); !os.IsExist(err) {
		t.Fatalf("export should preserve the original file: %v", err)
	}
	report, err := app.ImportHealth(captureFile)
	if err != nil || report.Temperature == nil || *report.Temperature != 45 || !report.At.Equal(original.At) {
		t.Fatalf("replay failed: %v %v", report.Temperature, err)
	}
	report.Capture = nil
	if err := store.Add(ctx, "benchmark", `E:\`, original.At, domain.BenchmarkResult{Started: original.At}); err != nil {
		t.Fatal(err)
	}
	for range 100 {
		if err := store.Add(ctx, "health", "synthetic disk", report.At, report); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.OpenAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	history, err := store.RecentBenchmarks(ctx)
	if err != nil || len(history) != 1 || history[0].Label != `E:\` {
		t.Fatalf("benchmark history should survive reopen and ignore newer health snapshots: %+v %v", history, err)
	}
}
