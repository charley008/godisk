package benchmark

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"goDisk/internal/domain"
	platform "goDisk/internal/platform/windows"
)

func TestDiskSpdReadSmoke(t *testing.T) {
	tool, root := os.Getenv("GODISK_SMOKE_DISKSPD"), os.Getenv("GODISK_SMOKE_VOLUME")
	if tool == "" || root == "" {
		t.Skip("requires explicit test volume and Microsoft DiskSpd 2.3 x64")
	}
	ctx := context.Background()
	_, volumes, err := platform.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	config := domain.BenchmarkConfig{Tool: tool, Mode: domain.Both, Standard: true, SizeMiB: 16, Seconds: 1, Threads: 1, Queue: 1, Repetitions: 1}
	config.WarmupSeconds = 1
	for _, volume := range volumes {
		if strings.EqualFold(root, volume.Root) {
			config.Volume = volume
			break
		}
	}
	t.Logf("target=%s mapped disks=%v", root, config.Volume.Disks)
	before := testDirs(t, config.Volume.GUID)
	result := Run(ctx, config, func(progress domain.Progress) { t.Log(progress.Phase) }, nil)
	if result.State != "completed" || result.Issue != "" || len(result.Scores) != 4 {
		t.Fatalf("run failed: state=%s issue=%s scores=%+v", result.State, result.Issue, result.Scores)
	}
	for i, score := range result.Scores {
		if score.Workload != domain.Workloads(config)[i].Key() || !score.Complete || score.Bytes == 0 ||
			score.Operations == 0 || score.WriteBytes != 0 || score.P99MS == nil {
			t.Fatalf("unexpected score: %+v", score)
		}
	}
	config.Mode, config.Seconds = domain.Random, 15
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var timer *time.Timer
	result = Run(cancelCtx, config, func(p domain.Progress) {
		if timer == nil && strings.Contains(p.Phase, "测量中") {
			timer = time.AfterFunc(550*time.Millisecond, cancel)
		}
	}, nil)
	if timer != nil {
		timer.Stop()
	}
	if result.State != "cancelled" || len(result.Scores) != 1 || result.Scores[0].Complete {
		t.Fatalf("cancellation failed: state=%s issue=%s scores=%+v", result.State, result.Issue, result.Scores)
	}
	after := testDirs(t, config.Volume.GUID)
	for name := range after {
		if !before[name] {
			t.Errorf("benchmark folder left behind: %s", name)
		}
	}
}

func testDirs(t *testing.T, volume string) map[string]bool {
	t.Helper()
	if volume == "" {
		t.Fatal("test volume was not found")
	}
	items, err := os.ReadDir(volume)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]bool)
	for _, item := range items {
		if item.IsDir() && strings.HasPrefix(item.Name(), "goDisk-test-") {
			out[item.Name()] = true
		}
	}
	return out
}
