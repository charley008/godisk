package desktop

import (
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"goDisk/internal/domain"
)

func (d *desktop) healthView(c *ui.Context) {
	ui.Column(c).Grow(1).MinHeight(0).Padding(6, 14, 10).Gap(6).Children(func() {
		if d.scanMessage != "" {
			note(c, d.scanMessage)
		}
		if len(d.disks) == 0 {
			if d.scanning {
				ui.Progress(c, -1)
			} else {
				note(c, "未发现磁盘")
			}
			if !d.offline {
				return
			}
		}
		if len(d.disks) != 0 {
			d.diskRail(c)
		}
		d.healthDetails(c)
	})
}

func (d *desktop) diskRail(c *ui.Context) {
	ui.ScrollHorizontal(c).Height(47).Shrink(0).KeepScrollbars(true).Children(func() {
		ui.Row(c).Gap(7).Children(func() {
			for _, disk := range d.disks {
				key := c.Key(disk.ID)
				selected := disk.ID == d.diskID && !d.offline
				report := d.summaries[disk.ID]
				volumes := d.diskVolumes(disk)
				secondary := volumes + "  ·  " + blank(report.State)
				if report.Temperature != nil {
					secondary += "  ·  " + celsius(report.Temperature)
				}
				border, face := c.Theme().Border, c.Theme().Surface
				if selected {
					border = c.Theme().Accent
					face = c.Theme().SurfaceHover
				}
				button := ui.ButtonBase(key).Width(207).Height(40).Shrink(0).Padding(3, 8).Radius(5).
					Border(1, border).Background(face).AlignItems(ui.Start).Label(disk.Model + " " + secondary)
				button.Children(func() {
					ui.Column(key).MinWidth(0).Gap(2).Children(func() {
						ui.Text(key, disk.Model).Bold().SingleLine().Tooltip(disk.Model)
						ui.Text(key, secondary).FontSize(11).SingleLine().TextColor(healthColor(key, report.State))
					})
				})
				if button.Clicked() && !selected && !d.running && !d.reading {
					d.selectDisk(disk)
				}
			}
		})
	})
}

func (d *desktop) diskVolumes(disk domain.Disk) string {
	var roots []string
	for _, volume := range d.volumes {
		if slices.Contains(volume.Disks, disk.Number) {
			roots = append(roots, strings.TrimSuffix(volume.Root, "\\"))
		}
	}
	if len(roots) == 0 {
		if disk.Number < 0 {
			return "盘符未知"
		}
		return fmt.Sprintf("磁盘 %d", disk.Number)
	}
	return strings.Join(roots, " ")
}

func (d *desktop) healthDetails(c *ui.Context) {
	disk, report := d.currentDisk(), d.report
	d.healthSummary(c, disk, report)
	if d.reading {
		ui.Progress(c, -1)
	}
	for _, message := range []string{d.healthNotice, report.Issue, disk.Issue} {
		if message != "" {
			note(c, message)
		}
	}
	if len(report.Metrics) == 0 {
		return
	}
	d.table.RowHeight = 19
	columns := []ui.TableColumn{
		{Title: "ID", Width: 54, Align: ui.Center},
		{Title: "SMART 属性", RowHeader: true, MinWidth: 235},
		{Title: "当前读数", Width: 154, Align: ui.End},
		{Title: "原始值 (HEX)", Width: 246, Align: ui.End},
	}
	theme := c.Theme()
	pad := theme.Space(2.5)
	ui.Table(c, &d.table, columns, len(report.Metrics), func(row, col int) {
		m := report.Metrics[row]
		id, label := metricPresentation(m.Name)
		values := [4]string{id, label, friendly(m.Name, m.Value), displayRaw(report.Protocol, m.Raw)}
		tooltips := [4]string{id, m.Name, m.Value, m.Raw}
		ui.Row(c).Fill().AlignItems(ui.Center).Justify(columns[col].Align).Draw(func(p *ui.Painter, r ui.Rect) {
			edge := r.X + r.W + pad
			p.Line(r.X-pad, r.Y+r.H-.5, edge, r.Y+r.H-.5, 1, theme.Border)
			if col+1 < len(columns) {
				p.Line(edge-.5, r.Y, edge-.5, r.Y+r.H, 1, theme.Border)
			}
		}).Children(func() {
			cell := ui.Text(c, values[col]).FontSize(12).SingleLine().Tooltip(tooltips[col])
			if col == 0 || col == 3 {
				cell.Font("Consolas")
			}
		})
	}).Grow(1).MinHeight(0).Border(1, c.Theme().Border).Background(c.Theme().Surface).Radius(4)
}

type summaryField struct {
	label, value string
	span         float32
	action       func()
}

func (d *desktop) healthSummary(c *ui.Context, disk domain.Disk, report domain.Health) {
	status := []summaryField{
		{label: "健康状态", value: report.State},
		{label: "温度", value: celsius(report.Temperature)},
		{label: "预计剩余寿命", value: wear(report)},
	}
	if d.offline {
		disk = domain.Disk{}
	}
	details := disk.Details
	if report.Details != nil {
		details = *report.Details
		if details.Serial == "" {
			details.Serial = disk.Details.Serial
		}
	}
	panel(c).Shrink(0).Role(ui.RoleTable).Label("磁盘摘要").Children(func() {
		if !d.offline {
			summaryRow(c.Key("device"), []summaryField{
				{label: "接口 / 容量", value: disk.Bus + " / " + diskCapacity(disk.Capacity)},
				{label: "驱动器号", value: d.diskVolumes(disk)},
				{label: "固件", value: disk.Firmware},
			}, c.Theme().Text)
			ui.Divider(c)
			serial := summaryField{label: "序列号", value: maskedSerial(details.Serial, d.revealSerial)}
			if details.Serial != "" {
				serial.action = func() { d.revealSerial = !d.revealSerial }
			}
			summaryRow(c.Key("connection"), []summaryField{
				serial,
				{label: "传输模式", value: transferText(details), span: 2},
			}, c.Theme().Text)
			ui.Divider(c)
			summaryRow(c.Key("capabilities"), []summaryField{
				{label: "标准", value: details.Standard},
				{label: "支持的功能", value: featuresText(details, len(report.Metrics) != 0), span: 2},
			}, c.Theme().Text)
			ui.Divider(c)
		}
		summaryRow(c.Key("status"), status, healthColor(c, report.State))
		ui.Divider(c)
		summaryRow(c.Key("runtime"), []summaryField{
			{label: "转速", value: rotationText(report, disk)},
			{label: "通电次数", value: friendly("Power Cycles", metricValue(report, "Power Cycles"))},
			{label: "通电时间", value: friendly("Power On Hours", metricValue(report, "Power On Hours"))},
		}, c.Theme().Text)
		ui.Divider(c)
		summaryRow(c.Key("counts"), []summaryField{
			{label: "主机累计读取", value: friendly("Host Data Read", metricValue(report, "Host Data Read")), span: 1.5},
			{label: "主机累计写入", value: friendly("Host Data Written", metricValue(report, "Host Data Written")), span: 1.5},
		}, c.Theme().Text)
	})
}

func maskedSerial(serial string, reveal bool) string {
	if reveal || serial == "" {
		return serial
	}
	chars := []rune(serial)
	if len(chars) > 4 {
		return "•••• " + string(chars[len(chars)-4:])
	}
	return "••••"
}

func transferText(details domain.DeviceDetails) string {
	if details.CurrentLink == "" && details.MaxLink == "" {
		return "未知"
	}
	return blank(details.CurrentLink) + " | " + blank(details.MaxLink)
}

func featuresText(details domain.DeviceDetails, smartRead bool) string {
	var features []string
	if smartRead || details.SMART {
		features = append(features, "S.M.A.R.T.")
	}
	if details.TRIM {
		features = append(features, "TRIM")
	}
	if details.VolatileWriteCache {
		features = append(features, "VolatileWriteCache")
	}
	if len(features) == 0 {
		return "未知"
	}
	return strings.Join(features, "、")
}

func rotationText(report domain.Health, disk domain.Disk) string {
	rate := report.RotationRate
	if rate == 0 {
		rate = disk.RotationRate
	}
	switch {
	case rate == 1 || report.Protocol == "NVMe" || disk.Bus == "NVMe":
		return "固态 (SSD)"
	case rate > 1:
		return fmt.Sprintf("%d RPM", rate)
	default:
		return "未知"
	}
}

func summaryRow(c *ui.Context, fields []summaryField, firstColor ui.Color) {
	border := c.Theme().Border
	ui.Row(c).Height(20).AlignItems(ui.Stretch).Role(ui.RoleRow).Children(func() {
		for i, field := range fields {
			key := c.Key(field.label)
			cells := ui.Row(key).Grow(max(1, field.span)).Basis(0).MinWidth(0).AlignItems(ui.Center).Gap(5).Padding(0, 8).Role(ui.RoleCell)
			if i > 0 {
				cells.Draw(func(p *ui.Painter, r ui.Rect) { p.Line(r.X+.5, r.Y, r.X+.5, r.Y+r.H, 1, border) })
			}
			cells.Children(func() {
				ui.Text(key, field.label).Width(78).Shrink(0).SingleLine().FontSize(11).TextColor(c.Theme().TextMuted)
				color := c.Theme().Text
				if i == 0 {
					color = firstColor
				}
				if field.action != nil {
					button := ui.ButtonBase(key).Height(20).Grow(1).MinWidth(0).Padding(0).Justify(ui.Start).
						Label("切换序列号显示 " + field.value).Tooltip("点击显示或隐藏序列号")
					button.Children(func() {
						ui.Text(key, field.value).FontSize(12).Bold().SingleLine().TextColor(c.Theme().Accent)
					})
					if button.Clicked() {
						field.action()
					}
					return
				}
				ui.Text(key, blank(field.value)).Grow(1).MinWidth(0).Bold().FontSize(12).SingleLine().Tooltip(field.value).TextColor(color)
			})
		}
	})
}

func wear(report domain.Health) string {
	if report.Protocol != "NVMe" {
		return "—"
	}
	used := report.PercentageUsed
	if used == nil {
		if parsed, err := strconv.Atoi(strings.TrimSuffix(metricValue(report, "Percentage Used"), "%")); err == nil {
			used = &parsed
		}
	}
	if used == nil {
		return "—"
	}
	return fmt.Sprintf("%d%%", max(0, 100-*used))
}

func metricValue(report domain.Health, name string) string {
	for _, metric := range report.Metrics {
		if metric.Name == name {
			return metric.Value
		}
	}
	return ""
}

func healthColor(c *ui.Context, state string) ui.Color {
	switch state {
	case "正常", "未报告阈值告警":
		return c.Theme().Success
	case "需关注", "有警告":
		return c.Theme().Warning
	default:
		return c.Theme().TextMuted
	}
}

func friendly(name, value string) string {
	switch name {
	case "Host Data Read", "Host Data Written":
		if digits, ok := strings.CutSuffix(value, " B"); ok {
			if n, ok := new(big.Int).SetString(digits, 10); ok && n.Sign() >= 0 {
				return n.Rsh(n, 30).String() + " GiB"
			}
		}
	case "Power On Hours", "Power Cycles", "Controller Busy Time":
		if _, err := strconv.ParseUint(value, 10, 64); err == nil {
			return value + map[string]string{"Power On Hours": " 小时", "Power Cycles": " 次", "Controller Busy Time": " 分钟"}[name]
		}
	}
	return blank(value)
}

func diskCapacity(bytes uint64) string {
	if bytes == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f GB", float64(bytes)/1e9)
}

func displayRaw(protocol, raw string) string {
	if protocol == "NVMe" && raw != "" && len(raw) < 14 {
		return strings.Repeat("0", 14-len(raw)) + raw
	}
	return blank(raw)
}

// These IDs are display identifiers, so snapshots retain their original field names.
var nvmeFields = map[string]struct{ id, label string }{
	"Critical Warning": {"01", "严重警告标志"}, "Composite Temperature": {"02", "综合温度"},
	"Available Spare": {"03", "可用备用空间"}, "Available Spare Threshold": {"04", "备用空间阈值"}, "Percentage Used": {"05", "已用寿命"},
	"Host Data Read": {"06", "主机累计读取"}, "Host Data Written": {"07", "主机累计写入"},
	"Host Read Commands": {"08", "主机读命令数"}, "Host Write Commands": {"09", "主机写命令数"},
	"Controller Busy Time": {"0A", "控制器忙碌时间"}, "Power Cycles": {"0B", "通电次数"}, "Power On Hours": {"0C", "通电时间"},
	"Unsafe Shutdowns": {"0D", "不安全关机次数"}, "Media and Data Integrity Errors": {"0E", "介质与数据完整性错误"},
	"Error Information Log Entries": {"0F", "错误日志条目"}, "Warning Temperature Time": {"10", "高温警告累计时间"},
	"Critical Temperature Time": {"11", "严重高温累计时间"}, "Thermal Management 1 Transitions": {"1A", "热管理 1 切换次数"},
	"Thermal Management 2 Transitions": {"1B", "热管理 2 切换次数"}, "Thermal Management 1 Time": {"1C", "热管理 1 持续时间"},
	"Thermal Management 2 Time": {"1D", "热管理 2 持续时间"},
}

func metricPresentation(name string) (id, label string) {
	if item, ok := nvmeFields[name]; ok {
		return item.id, item.label
	}
	if sensor, ok := strings.CutPrefix(name, "Temperature Sensor "); ok {
		if n, err := strconv.Atoi(sensor); err == nil && n >= 1 && n <= 8 {
			return fmt.Sprintf("%02X", 0x11+n), "温度传感器 " + sensor
		}
	}
	if ataID, ok := strings.CutPrefix(name, "SMART 0x"); ok {
		return ataID, "ATA SMART 属性"
	}
	return "—", name
}
