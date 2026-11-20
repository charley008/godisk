package desktop

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"goDisk/internal/application"
	"goDisk/internal/domain"
)

const (
	windowWidth           = 744
	windowHeight          = 817
	healthRefreshInterval = 30 * time.Second
)

type desktop struct {
	ctx       context.Context
	win       *mygo.Window
	caption   string
	version   string
	app       application.App
	page      int
	darkTheme bool
	themeMenu *mygo.MenuItem
	disks     []domain.Disk
	volumes   []domain.Volume
	summaries map[string]domain.Health

	diskID       string
	revealSerial bool
	report       domain.Health
	table        ui.ListState
	scanning     bool
	reading      bool
	readSequence uint64
	offline      bool
	scanMessage  string
	healthNotice string
	storageIssue string

	tool, selectedVolume, mode      string
	sizeLabel                       string
	seconds, queue, threads, repeat float64
	writes                          bool
	advanced, custom                bool
	unit                            string
	running, closing, stopping      bool
	cancelBenchmark                 context.CancelFunc
	progress                        domain.Progress
	bench                           domain.BenchmarkResult
	benchNotice                     string

	history       []benchmarkHistoryEntry
	historyNotice string
}

type benchmarkHistoryEntry struct {
	at     time.Time
	label  string
	result domain.BenchmarkResult
	err    error
}

func Open(ctx context.Context, app application.App, storeErr error) *mygo.Window {
	ctx, cancel := context.WithCancel(ctx)
	mygo.Theme.SetSource(savedAppearance())
	d := &desktop{ctx: ctx, app: app, mode: domain.Both, version: mygo.App.Version(), darkTheme: mygo.Theme.IsDark(),
		sizeLabel: domain.BenchmarkSizeLabel(domain.DefaultBenchmarkSizeMiB), seconds: 5,
		queue: 1, threads: 1, repeat: 1, unit: "MB/s", summaries: make(map[string]domain.Health)}
	if storeErr != nil {
		d.storageIssue = "历史数据库不可用 · " + storeErr.Error()
	}
	win := mygo.NewWindow(mygo.WindowOptions{
		Title: d.windowTitle(), Width: windowWidth, Height: windowHeight, DisableResize: true, DisableMaximize: true, Content: ui.View(d.view),
	})
	d.win = win
	d.installMenu()
	if root, err := mygo.App.Path(mygo.PathResources); err == nil {
		d.tool = filepath.Join(root, "diskspd", "diskspd.exe")
	}
	win.OnClose(func(event *mygo.CloseEvent) {
		if d.running {
			event.PreventDefault()
			d.closing = true
			d.stopBenchmark()
		}
	})
	win.OnClosed(cancel)
	d.scan()
	go d.pollHealth()
	d.loadHistory()
	return win
}

func (d *desktop) pollHealth() {
	ticker := time.NewTicker(healthRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.win.Update(d.scan)
		}
	}
}

func (d *desktop) scan() {
	if d.ctx.Err() != nil || d.scanning || d.reading || d.running {
		return
	}
	d.scanning = true
	d.scanMessage = ""
	go func() {
		disks, volumes, err := d.app.Disks(d.ctx)
		d.win.Update(func() {
			if d.ctx.Err() != nil {
				return
			}
			if err != nil {
				d.scanMessage = err.Error()
				if disks == nil {
					d.scanning = false
					return
				}
			}
			d.disks, d.volumes = disks, volumes
			for id := range d.summaries {
				if !slices.ContainsFunc(disks, func(disk domain.Disk) bool { return disk.ID == id }) {
					delete(d.summaries, id)
				}
			}
			current := d.currentDisk()
			skip := ""
			if !d.offline && current.ID == "" {
				if len(disks) != 0 {
					current = disks[0]
					skip = current.ID
					d.selectDisk(current)
				} else {
					d.diskID, d.report = "", domain.Health{}
				}
			}
			if d.chosenVolume().Root == "" {
				d.selectedVolume = ""
			}
			go d.readSummaries(current.ID, skip, disks)
		})
	}()
}

func (d *desktop) readSummaries(selected, skip string, disks []domain.Disk) {
	defer d.win.Update(func() { d.scanning = false })
	for _, disk := range disks {
		if d.ctx.Err() != nil {
			return
		}
		if disk.ID == skip || (disk.Bus != "NVMe" && disk.ID != selected) {
			continue
		}
		report := d.app.ReadHealth(d.ctx, disk)
		d.win.Update(func() { d.applyHealthPoll(disk.ID, report) })
	}
}

func (d *desktop) applyHealthPoll(id string, report domain.Health) {
	if d.summaries[id].At.After(report.At) {
		return
	}
	d.summaries[id] = report
	if !d.offline && !d.reading && d.diskID == id && !d.report.At.After(report.At) {
		d.report = report
	}
}

func (d *desktop) currentDisk() domain.Disk {
	for _, disk := range d.disks {
		if disk.ID == d.diskID {
			return disk
		}
	}
	return domain.Disk{}
}

func (d *desktop) selectDisk(disk domain.Disk) {
	d.diskID = disk.ID
	d.revealSerial = false
	d.report, d.offline, d.reading = domain.Health{}, false, true
	d.healthNotice = ""
	d.readSequence++
	current := d.readSequence
	go func() {
		report := d.app.ReadHealth(d.ctx, disk)
		d.win.Update(func() {
			if current != d.readSequence {
				return
			}
			if latest := d.summaries[disk.ID]; latest.At.After(report.At) {
				report = latest
			}
			d.report, d.reading = report, false
			d.summaries[disk.ID] = report
		})
	}()
}

func (d *desktop) chosenVolume() domain.Volume {
	for _, volume := range d.volumes {
		if volumeLabel(volume) == d.selectedVolume {
			return volume
		}
	}
	return domain.Volume{}
}

func (d *desktop) benchmarkConfig() domain.BenchmarkConfig {
	return domain.BenchmarkConfig{
		Volume: d.chosenVolume(), Tool: d.tool, Mode: d.mode, SizeMiB: selectedFileSize(d.sizeLabel), Seconds: int(d.seconds),
		Queue: int(d.queue), Threads: int(d.threads), Repetitions: int(d.repeat), Writes: d.writes, Standard: !d.custom,
		WarmupSeconds: 5, IntervalSeconds: 5, PeakRounds: true,
	}
}

func (d *desktop) startBenchmark() {
	if d.running || d.scanning || d.reading {
		return
	}
	config := d.benchmarkConfig()
	if err := d.app.ValidateBenchmark(config); err != nil {
		d.benchNotice = err.Error()
		return
	}
	d.running, d.stopping = true, false
	d.benchNotice = ""
	d.bench = domain.BenchmarkResult{Config: config, Started: time.Now(), State: "running"}
	d.progress = domain.Progress{Phase: "检查测速目标"}
	ctx, cancel := context.WithCancel(d.ctx)
	d.cancelBenchmark = cancel
	disks := slices.Clone(d.disks)
	go func() {
		defer cancel()
		result := d.app.Benchmark(ctx, config, disks, func(progress domain.Progress) {
			d.win.Update(func() {
				if progress.Score != nil {
					d.bench.Scores = append(d.bench.Scores, *progress.Score)
				}
				if !d.stopping || progress.Fraction == 1 {
					d.progress = progress
				}
			})
		})
		d.win.Update(func() {
			d.running, d.stopping = false, false
			d.cancelBenchmark, d.bench = nil, result
			if d.closing {
				d.win.Close()
			} else {
				d.loadHistory()
				d.scan()
			}
		})
	}()
}

func (d *desktop) stopBenchmark() {
	if d.cancelBenchmark != nil && !d.stopping {
		d.stopping = true
		d.progress.Phase = "正在取消并回收测速进程"
		d.cancelBenchmark()
	}
}

func (d *desktop) exportCapture() {
	report := d.report
	if len(report.Capture) == 0 {
		d.healthNotice = "当前没有可导出的 NVMe 健康日志。"
		return
	}
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent: d.win, Title: "导出 NVMe 健康快照", DefaultPath: "goDisk-health-" + time.Now().Format("20060102-150405") + ".json",
		Filters: []mygo.FileFilter{{Name: "goDisk JSON", Extensions: []string{"json"}}},
	})
	if err == nil && path != "" {
		err = d.app.ExportHealth(path, report)
		if err == nil {
			d.healthNotice = "已导出 " + filepath.Base(path)
		}
	}
	if err != nil {
		d.healthNotice = "导出失败 · " + err.Error()
	}
}

func (d *desktop) importCapture() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: d.win, Title: "打开 goDisk 健康快照",
		Filters: []mygo.FileFilter{{Name: "goDisk JSON", Extensions: []string{"json"}}}})
	if err == nil && len(paths) != 0 {
		var report domain.Health
		report, err = d.app.ImportHealth(paths[0])
		if err == nil {
			d.readSequence++
			d.report, d.offline, d.reading = report, true, false
			d.healthNotice = "正在查看离线快照 " + filepath.Base(paths[0])
		}
	}
	if err != nil {
		d.healthNotice = "导入失败 · " + err.Error()
	}
}

func (d *desktop) loadHistory() {
	if d.app.Store == nil {
		return
	}
	go func() {
		records, err := d.app.Store.RecentBenchmarks(d.ctx)
		history := make([]benchmarkHistoryEntry, len(records))
		for i, record := range records {
			entry := &history[i]
			entry.at, entry.label = record.At, record.Label
			entry.err = json.Unmarshal(record.Payload, &entry.result)
		}
		d.win.Update(func() {
			if err != nil {
				d.historyNotice = err.Error()
				return
			}
			d.history, d.historyNotice = history, ""
		})
	}()
}

func (d *desktop) restoreHistory(entry benchmarkHistoryEntry) {
	if d.running {
		return
	}
	d.historyNotice = ""
	if entry.err != nil {
		d.historyNotice = entry.err.Error()
		return
	}
	d.bench, d.page = entry.result, 1
}

func (d *desktop) windowTitle() string {
	title := "goDisk"
	if d.version != "" {
		title += " v" + d.version
	}
	switch d.page {
	case 1:
		return title + " — 性能测试"
	case 2:
		return title + " — 历史记录"
	}
	if d.offline {
		title += " — 离线健康快照"
	} else if disk := d.currentDisk(); disk.Model != "" {
		title += " — " + disk.Model
	}
	if !d.report.At.IsZero() {
		title += " · 采集于 " + d.report.At.Local().Format("01-02 15:04:05")
	}
	return title
}

func volumeLabel(volume domain.Volume) string {
	label := volume.Root
	if volume.Label != "" {
		label += "  " + volume.Label
	}
	if volume.System {
		label += "  · 系统"
	}
	return label
}

func (d *desktop) volumeModel(volume domain.Volume) string {
	if len(volume.Disks) != 1 {
		return "卷映射不明确"
	}
	for _, disk := range d.disks {
		if disk.Number == volume.Disks[0] {
			return disk.Model
		}
	}
	return "未知磁盘"
}
