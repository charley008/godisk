package desktop

import (
	"fmt"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
	"goDisk/internal/domain"
)

func (d *desktop) benchmarkView(c *ui.Context) {
	ui.Column(c).Gap(9).Children(func() {
		d.benchmarkControls(c)
		config := d.benchmarkConfig()
		bench := d.bench
		if !bench.Started.IsZero() {
			config = bench.Config
		}
		workloads := domain.Workloads(config)
		limit := benchmarkLimit(bench.Scores, d.unit)
		panel(c).Padding(10).Gap(6).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "测试项目").Width(162).Bold()
				ui.Text(c, "读取 · "+d.unit).Bold().Grow(1).Basis(0).MinWidth(0)
				ui.Text(c, "写入 · "+d.unit).Bold().Grow(1).Basis(0).MinWidth(0)
			})
			for _, workload := range workloads {
				d.workloadRow(c.Key(workload.Key()), workload, config.Writes, config.PeakRounds, limit)
			}
		})
		if d.running {
			ui.Progress(c, d.progress.Fraction).Label("测试进度")
			note(c, d.progress.Phase)
		} else if bench.Started.IsZero() {
			note(c, fmt.Sprintf("开始后会在目标卷创建 %s 临时文件。", domain.BenchmarkSizeLabel(config.SizeMiB)))
		} else {
			detail := fmt.Sprintf("%s · %s · %s · %s · %d 秒/项 · %d 次",
				statusLabel(bench.State), bench.Config.Volume.Root, bench.Started.Local().Format("01-02 15:04"),
				domain.BenchmarkSizeLabel(bench.Config.SizeMiB), bench.Config.Seconds, bench.Config.Repetitions)
			if bench.Version != "" {
				detail += " · DiskSpd " + bench.Version
			}
			note(c, detail)
			if len(bench.Temperatures) != 0 {
				temperatureView(c, bench.Temperatures)
			}
		}
		for _, message := range []string{d.benchNotice, bench.Issue} {
			if message != "" {
				note(c, message)
			}
		}
	})
}

func (d *desktop) benchmarkControls(c *ui.Context) {
	ui.Row(c).Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
		options := []string{"选择目标卷"}
		for _, volume := range d.volumes {
			if len(volume.Disks) == 1 && len(volume.Identities) == 1 {
				options = append(options, volumeLabel(volume))
			}
		}
		if d.selectedVolume == "" {
			d.selectedVolume = options[0]
		}
		ui.Select(c, &d.selectedVolume, options).Label("目标卷").Width(180).Disabled(d.running || d.scanning)
		ui.Select(c, &d.sizeLabel, fileSizeOptions).Width(105).Label("测试文件大小").Disabled(d.running)
		ui.Select(c, &d.unit, []string{"MB/s", "MiB/s", "IOPS", "平均 ms", "P99 ms"}).Width(96).Label("结果单位")
		ui.Checkbox(c, &d.writes, "加入写入").Disabled(d.running)
		ui.Box(c).Grow(1)
		if d.running {
			if ui.Button(c, "取消测试").Disabled(d.stopping).Clicked() {
				d.stopBenchmark()
			}
		} else if ui.PrimaryButton(c, "开始测试").Disabled(d.scanning || d.reading || d.chosenVolume().Root == "").Clicked() {
			d.startBenchmark()
		}
	})
	if v := d.chosenVolume(); v.Root != "" {
		note(c, fmt.Sprintf("%s · 可用 %s", d.volumeModel(v), capacity(v.Free)))
	}
	if d.advanced {
		panel(c).Padding(8, 10).Gap(6).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
				ui.Select(c, &d.mode, []string{domain.Both, domain.Sequential, domain.Random}).Width(125).Label("测试项目").Disabled(d.running)
				numberField(c.Key("duration"), "每项秒数", &d.seconds, 1, 30, d.running)
				numberField(c.Key("repeat"), "重复次数", &d.repeat, 1, 5, d.running)
				ui.Checkbox(c, &d.custom, "自定义队列和线程").Disabled(d.running)
				if d.custom {
					numberField(c.Key("queue"), "每线程队列", &d.queue, 1, 32, d.running)
					numberField(c.Key("thread"), "线程数", &d.threads, 1, 8, d.running)
				}
			})
			note(c, "各项目先预热 5 秒，项目间隔 5 秒。速度和 IOPS 取最高一轮，延迟显示最近完成一轮。")
		})
	}
}

var fileSizeOptions = func() []string {
	var labels []string
	for _, size := range domain.BenchmarkFileSizes() {
		labels = append(labels, domain.BenchmarkSizeLabel(size))
	}
	return labels
}()

func selectedFileSize(label string) int {
	for _, size := range domain.BenchmarkFileSizes() {
		if domain.BenchmarkSizeLabel(size) == label {
			return size
		}
	}
	return 0
}

func (d *desktop) workloadRow(c *ui.Context, workload domain.Workload, writes, peak bool, limit float64) {
	ui.Row(c).Gap(8).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(162).Shrink(0).Padding(8, 2).Gap(2).Children(func() {
			ui.Text(c, workload.Name).Bold()
			note(c, fmt.Sprintf("Q%d · T%d", workload.Queue, workload.Threads))
		})
		for _, write := range []bool{false, true} {
			value, measured, partial := scoreValue(d.bench.Scores, workload, write, d.unit, peak)
			key := c.Key(write)
			d.resultCell(key, value, limit, measured, partial, write, writes)
		}
	})
}

func (d *desktop) resultCell(c *ui.Context, value, limit float64, measured, partial, write, writes bool) {
	unit := d.unit
	valueText := "—"
	if measured {
		switch unit {
		case "IOPS":
			valueText = fmt.Sprintf("%.0f", value)
		case "平均 ms", "P99 ms":
			valueText = fmt.Sprintf("%.3f", value)
		default:
			valueText = fmt.Sprintf("%.2f", value)
		}
	} else if write && !writes {
		valueText = "未启用"
	}
	indicator := c.Theme().Accent
	if write {
		indicator = c.Theme().Success
	}
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Height(58).Gap(1).Padding(4, 11).AlignItems(ui.End).
		Justify(ui.Center).Radius(6).Border(1, c.Theme().Border).Background(c.Theme().Background).
		Draw(func(p *ui.Painter, r ui.Rect) {
			if measured && !partial && limit > 0 && unit != "平均 ms" && unit != "P99 ms" {
				width := max(0, r.W-12) * float32(min(1, value/limit))
				p.Line(r.X+6, r.Y+r.H-4, r.X+6+width, r.Y+r.H-4, 2, indicator)
			}
		}).Children(func() {
		color, size := c.Theme().Text, float32(25)
		if !measured {
			color, size = c.Theme().TextMuted, 15
		}
		ui.Text(c, valueText).FontSize(size).Bold().SingleLine().TextColor(color)
		if partial {
			note(c, "部分结果")
		}
	})
}

func scoreValue(scores []domain.Score, workload domain.Workload, write bool, unit string, peak bool) (float64, bool, bool) {
	var seconds, count, best float64
	var latency *float64
	var partial *domain.Score
	for _, score := range scores {
		if !scoreMatches(score, workload, write) || score.Seconds <= 0 {
			continue
		}
		if !score.Complete {
			s := score
			partial = &s
			continue
		}
		seconds += score.Seconds
		amount := scoreCount(score, unit)
		count += amount
		best = max(best, amount/score.Seconds)
		latency = scoreLatency(score, unit)
	}
	incomplete := seconds == 0 && partial != nil
	if incomplete {
		seconds, count, latency = partial.Seconds, scoreCount(*partial, unit), scoreLatency(*partial, unit)
	}
	if unit == "平均 ms" || unit == "P99 ms" {
		if latency != nil {
			return *latency, true, incomplete
		}
		return 0, false, false
	}
	if seconds <= 0 {
		return 0, false, false
	}
	if peak && !incomplete {
		return best, true, false
	}
	return count / seconds, true, incomplete
}

func scoreMatches(score domain.Score, workload domain.Workload, write bool) bool {
	if score.Workload != "" {
		return score.Workload == workload.Key() && score.Write == write
	}
	// v0.1.0 stored the direction in the label and queue/thread in config.
	direction := "读"
	if write {
		direction = "写"
	}
	return strings.HasPrefix(score.Profile, workload.Name+" "+direction+" ·")
}

func scoreCount(s domain.Score, unit string) float64 {
	switch unit {
	case "IOPS":
		return float64(s.Operations)
	case "MiB/s":
		return float64(s.Bytes) / (1 << 20)
	default:
		return float64(s.Bytes) / 1e6
	}
}

func scoreLatency(s domain.Score, unit string) *float64 {
	switch unit {
	case "平均 ms":
		return s.AverageMS
	case "P99 ms":
		return s.P99MS
	default:
		return nil
	}
}

func benchmarkLimit(scores []domain.Score, unit string) float64 {
	limit := 0.0
	for _, score := range scores {
		if score.Complete && score.Seconds > 0 {
			limit = math.Max(limit, scoreCount(score, unit)/score.Seconds)
		}
	}
	return limit
}
