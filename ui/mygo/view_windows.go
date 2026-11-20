package desktop

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"goDisk/internal/domain"
)

func (d *desktop) view(c *ui.Context) {
	theme := c.Theme()
	if theme.Dark {
		theme.Background, theme.Surface, theme.Border = ui.Hex("#14171b"), ui.Hex("#20252b"), ui.Hex("#333c46")
		theme.Accent, theme.AccentHover, theme.AccentPressed = ui.Hex("#8bb6ff"), ui.Hex("#a9c8ff"), ui.Hex("#70a4f6")
		theme.AccentText = ui.Hex("#12233a")
	} else {
		theme.Background, theme.Surface, theme.Border = ui.Hex("#f4f6fa"), ui.Hex("#fcfdff"), ui.Hex("#d6dee9")
		theme.Accent, theme.AccentHover, theme.AccentPressed = ui.Hex("#265fba"), ui.Hex("#3474cc"), ui.Hex("#1d4a97")
		theme.AccentText = ui.Hex("#ffffff")
	}
	theme.InactiveAccent = theme.Accent
	c.SetTheme(theme)

	ui.Column(c).Fill().Background(theme.Background).Children(func() {
		ui.Column(c).Padding(0, 18).Shrink(0).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Tabs(c, &d.page, "磁盘健康", "性能测试", "历史记录")
				ui.Box(c).Grow(1)
				if d.page == 1 {
					label := "高级设置"
					if d.advanced {
						label = "收起设置"
					}
					if ui.Button(c, label).Clicked() {
						d.advanced = !d.advanced
					}
				}
				ui.Row(c).AlignItems(ui.Center).Gap(6).Shrink(0).Children(func() {
					ui.Text(c, "深色主题").FontSize(12).SingleLine().
						OnClick(func() { d.setAppearance(!d.darkTheme) })
					if ui.Switch(c, &d.darkTheme).Label("深色主题").Changed() {
						d.setAppearance(d.darkTheme)
					}
				})
			})
			ui.Divider(c)
		})
		switch d.page {
		case 0:
			d.healthView(c)
		case 1:
			ui.Scroll(c).Grow(1).MinHeight(0).Padding(10, 18, 14).Children(func() { d.benchmarkView(c) })
		case 2:
			ui.Scroll(c).Grow(1).MinHeight(0).Padding(10, 18, 14).Children(func() { d.historyView(c) })
		}
	})
	if title := d.windowTitle(); d.win != nil && title != d.caption {
		d.caption = title
		d.win.SetTitle(title)
	}
}

func panel(c *ui.Context) ui.Element {
	return ui.Column(c).Radius(8).Border(1, c.Theme().Border).Background(c.Theme().Surface)
}

func temperatureView(c *ui.Context, samples []domain.TempSample) {
	var low, high *int
	for _, sample := range samples {
		if sample.Celsius == nil {
			continue
		}
		n := *sample.Celsius
		if low == nil || n < *low {
			low = &n
		}
		if high == nil || n > *high {
			high = &n
		}
	}
	if low == nil || high == nil {
		note(c, "本次没有可用的温度样本。")
		return
	}
	note(c, fmt.Sprintf("测速期间 NVMe 温度 · %d 至 %d °C · %d 次采样", *low, *high, len(samples)))
	accent := c.Theme().Accent
	ui.Box(c).Height(60).Border(1, c.Theme().Border).Radius(6).Draw(func(p *ui.Painter, r ui.Rect) {
		var x, y float32
		previous := false
		for i, point := range samples {
			if point.Celsius == nil {
				previous = false
				continue
			}
			nextX := r.X + 8 + float32(i)*(r.W-16)/float32(max(1, len(samples)-1))
			nextY := r.Y + r.H - 8 - float32(*point.Celsius-*low+1)*(r.H-16)/float32(*high-*low+2)
			if previous {
				p.Line(x, y, nextX, nextY, 2, accent)
			} else {
				p.Line(nextX, nextY, nextX+3, nextY, 3, accent)
			}
			x, y, previous = nextX, nextY, true
		}
	})
}

func (d *desktop) historyView(c *ui.Context) {
	ui.Column(c).Gap(9).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, "测速历史").FontSize(22).Bold().Grow(1)
			if ui.Button(c, "刷新记录").Disabled(d.app.Store == nil).Clicked() {
				d.loadHistory()
			}
		})
		if d.storageIssue != "" {
			note(c, d.storageIssue)
		}
		if d.historyNotice != "" {
			note(c, d.historyNotice)
		}
		if len(d.history) == 0 && d.storageIssue == "" {
			note(c, "暂无测速记录。")
		}
		for i, entry := range d.history {
			speed := "—"
			if entry.err == nil {
				speed = firstSequentialRead(entry.result)
			}
			text := fmt.Sprintf("%s  ·  %s  ·  顺序读取 %s", entry.at.Local().Format("2006-01-02 15:04:05"), entry.label, speed)
			if ui.Button(c.Key(i), text).Disabled(d.running).Clicked() {
				d.restoreHistory(entry)
			}
		}
	})
}

func firstSequentialRead(result domain.BenchmarkResult) string {
	workloads := domain.Workloads(result.Config)
	if len(workloads) == 0 || workloads[0].Random {
		return "—"
	}
	first := workloads[0]
	value, ok, partial := scoreValue(result.Scores, first, false, "MB/s", result.Config.PeakRounds)
	if !ok || partial {
		return "—"
	}
	return fmt.Sprintf("Q%dT%d %.2f MB/s", first.Queue, first.Threads, value)
}

func numberField(c *ui.Context, label string, value *float64, low, high float64, disabled bool) {
	ui.Column(c).Width(108).Gap(4).Children(func() {
		note(c, label)
		ui.NumberInput(c, value, low, high, 1).Label(label).Disabled(disabled)
	})
}

func note(c *ui.Context, value string) {
	ui.Text(c, value).FontSize(12).TextColor(c.Theme().TextMuted)
}

func blank(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}

func capacity(bytes uint64) string {
	switch {
	case bytes == 0:
		return "—"
	case bytes >= 1e12:
		return fmt.Sprintf("%.2f TB", float64(bytes)/1e12)
	default:
		return fmt.Sprintf("%.1f GB", float64(bytes)/1e9)
	}
}

func celsius(value *int) string {
	if value == nil {
		return "—"
	}
	return fmt.Sprintf("%d °C", *value)
}

func statusLabel(status string) string {
	switch status {
	case "completed":
		return "已完成"
	case "cancelled":
		return "已取消"
	case "timed_out":
		return "超时"
	default:
		return "失败"
	}
}
