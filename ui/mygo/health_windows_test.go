package desktop

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"goDisk/internal/domain"
)

func TestThemeSwitchInPageToolbar(t *testing.T) {
	d := &desktop{}
	for _, page := range []int{0, 1, 2} {
		d.page = page
		view := ui.NewTester(d.view, windowWidth, windowHeight)
		bounds, found := view.Find("深色主题")
		if !found || bounds.X < windowWidth-160 || bounds.X+bounds.W > windowWidth || bounds.Y > 44 {
			t.Fatalf("page %d theme switch is outside the top-right toolbar: %v", page, bounds)
		}
		if page == 1 && !view.HasText("高级设置") {
			t.Fatal("theme switch displaced advanced settings")
		}
	}
}

func TestCompactHealthTableAtMinimumWidth(t *testing.T) {
	disk := domain.Disk{ID: "sample", Model: "Sample NVMe", Bus: "NVMe", Number: 0, Details: domain.DeviceDetails{
		Serial: "EXAMPLEDEVICE0123", Standard: "NVMe 1.3", CurrentLink: "PCIe 4.0 x4", MaxLink: "PCIe 4.0 x4",
		SMART: true, TRIM: true, VolatileWriteCache: true,
	}}
	at := time.Date(2026, 1, 2, 15, 4, 5, 0, time.Local)
	d := &desktop{version: "1.2.3", diskID: disk.ID, disks: []domain.Disk{disk}, report: domain.Health{
		At: at, Protocol: "NVMe", State: "正常",
		Metrics: []domain.Metric{
			{Name: "Composite Temperature", Value: "47 °C", Raw: "0140"},
			{Name: "Thermal Management 2 Time", Value: "0 秒", Raw: "0"},
		},
	}}
	if title := d.windowTitle(); !strings.HasPrefix(title, "goDisk v1.2.3 — ") ||
		!strings.Contains(title, disk.Model) || !strings.Contains(title, "采集于 01-02 15:04:05") {
		t.Fatalf("unexpected health caption: %q", title)
	}
	view := ui.NewTester(d.view, windowWidth, 490)
	for _, label := range []string{"转速", "固态 (SSD)", "ID", "SMART 属性", "当前读数", "原始值 (HEX)", "02",
		"综合温度", "00000000000140", "1D", "NVMe 1.3", "PCIe 4.0 x4 | PCIe 4.0 x4", "S.M.A.R.T.、TRIM、VolatileWriteCache", "•••• 0123"} {
		bounds, found := view.Find(label)
		if !found || bounds.X < 0 || bounds.Y < 0 || bounds.X+bounds.W > windowWidth || bounds.Y+bounds.H > 490 {
			t.Errorf("missing or clipped %q: %v", label, bounds)
		}
	}
	header, ok := view.Find("SMART 属性")
	if !ok || header.Y > 240 {
		t.Errorf("SMART header uses too much vertical space: %v", header)
	}
	if view.HasText("采集于") {
		t.Error("capture time belongs in the window caption")
	}
	if view.HasText("刷新") {
		t.Error("health refresh must happen automatically")
	}
	if view.HasText(disk.Details.Serial) {
		t.Error("serial should be masked by default")
	}
	if err := view.Click("切换序列号显示 •••• 0123"); err != nil {
		t.Fatal(err)
	}
	if !view.HasText(disk.Details.Serial) {
		t.Error("serial should appear on request")
	}
	d.page = 1
	if title := d.windowTitle(); title != "goDisk v1.2.3 — 性能测试" {
		t.Fatalf("unexpected benchmark caption: %q", title)
	}
	d.page = 2
	if title := d.windowTitle(); title != "goDisk v1.2.3 — 历史记录" {
		t.Fatalf("unexpected history caption: %q", title)
	}
}

func TestDiskRailScrollsToSixthDisk(t *testing.T) {
	var disks []domain.Disk
	for n := 1; n <= 6; n++ {
		disks = append(disks, domain.Disk{ID: fmt.Sprint(n), Model: fmt.Sprintf("Demo disk %d", n), Number: n})
	}
	d := &desktop{disks: disks, diskID: "1"}
	view := ui.NewTester(d.view, windowWidth, windowHeight)
	first, found := view.Find("Demo disk 1")
	if !found {
		t.Fatal("first disk is missing")
	}
	if last, ok := view.Find("Demo disk 6"); ok && last.X+last.W <= windowWidth {
		t.Fatal("sixth disk should initially overflow the rail")
	}
	view.Scroll(first.X+5, first.Y+5, 1600, 0)
	last, found := view.Find("Demo disk 6")
	if !found || last.X < 0 || last.X+last.W > windowWidth {
		t.Fatalf("could not scroll to sixth disk: %v", last)
	}
}

func TestHealthPollPreservesOfflineSnapshot(t *testing.T) {
	first := time.Now()
	live := domain.Health{At: first, State: "正常"}
	d := &desktop{diskID: "disk", report: live, summaries: make(map[string]domain.Health)}
	newer := domain.Health{At: first.Add(time.Second), State: "需关注"}
	d.applyHealthPoll("disk", newer)
	if d.report.State != newer.State {
		t.Fatal("live health did not refresh")
	}
	d.offline, d.report = true, live
	latest := domain.Health{At: first.Add(2 * time.Second), State: "有警告"}
	d.applyHealthPoll("disk", latest)
	d.applyHealthPoll("disk", newer)
	if d.report.At != first || d.summaries["disk"].State != latest.State {
		t.Fatal("automatic polling replaced the offline snapshot or regressed the live summary")
	}
}

func TestHealthRotationStatus(t *testing.T) {
	if got := rotationText(domain.Health{RotationRate: 7200}, domain.Disk{Bus: "SATA"}); got != "7200 RPM" {
		t.Errorf("unexpected HDD rotation: %q", got)
	}
	if got := rotationText(domain.Health{}, domain.Disk{Bus: "SATA", RotationRate: 5400}); got != "5400 RPM" {
		t.Errorf("unexpected scanned HDD rotation: %q", got)
	}
	if got := rotationText(domain.Health{}, domain.Disk{Bus: "USB"}); got != "未知" {
		t.Errorf("unknown rotation should remain unknown: %q", got)
	}
}

func TestHealthReadingsMatchScreenshotCounters(t *testing.T) {
	for _, test := range []struct{ field, bytes, expected string }{
		{"Host Data Read", "36431525376000 B", "33929 GiB"},
		{"Host Data Written", "32309691392000 B", "30090 GiB"},
		{"Host Data Read", "36430594560000 B", "33928 GiB"},
		{"Host Data Written", "32306878464000 B", "30088 GiB"},
		{"Host Data Read", "19807040628566084398385987584 B", "18446744073709551616 GiB"},
		{"Host Data Read", "未报告", "未报告"},
	} {
		if got := friendly(test.field, test.bytes); got != test.expected {
			t.Errorf("%s: got %q, want %q", test.bytes, got, test.expected)
		}
	}
	if got := diskCapacity(1_000_202_273_280); got != "1000.2 GB" {
		t.Errorf("unexpected capacity: %s", got)
	}
	const wide = "123456789ABCDEF0123456789ABCDEF"
	if got := displayRaw("NVMe", wide); got != wide {
		t.Errorf("wide hex value was changed: %s", got)
	}
}
