package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"goDisk/internal/domain"
)

func NVMe(page []byte) (domain.Health, error) {
	if len(page) != 512 {
		return domain.Health{}, fmt.Errorf("NVMe SMART/Health 日志应为 512 字节，实际 %d", len(page))
	}
	var empty [512]byte
	if bytes.Equal(page, empty[:]) {
		return domain.Health{}, fmt.Errorf("NVMe SMART/Health 日志缺少健康数据")
	}
	state := "正常"
	switch {
	case page[0] != 0:
		state = "有警告"
	case page[3] < page[4] || page[5] >= 100:
		state = "需关注"
	}
	health := domain.Health{
		Protocol: "NVMe", State: state, Capture: page,
		Metrics: []domain.Metric{
			{Name: "Critical Warning", Value: fmt.Sprintf("0x%02X", page[0]), Raw: fmt.Sprintf("%02X", page[0])},
		},
	}
	health.Temperature = kelvin(page[1:])
	if health.Temperature != nil {
		health.Metrics = append(health.Metrics, domain.Metric{Name: "Composite Temperature", Value: fmt.Sprintf("%d °C", *health.Temperature), Raw: fmt.Sprintf("%04X", binary.LittleEndian.Uint16(page[1:]))})
	}
	for i, name := range []string{"Available Spare", "Available Spare Threshold", "Percentage Used"} {
		value := page[3+i]
		health.Metrics = append(health.Metrics, domain.Metric{Name: name, Value: fmt.Sprintf("%d%%", value), Raw: fmt.Sprintf("%02X", value)})
	}
	used := int(page[5])
	health.PercentageUsed = &used
	for _, field := range []struct {
		name string
		at   int
		unit int64
	}{
		{"Host Data Read", 32, 512000}, {"Host Data Written", 48, 512000},
		{"Host Read Commands", 64, 0}, {"Host Write Commands", 80, 0},
		{"Controller Busy Time", 96, 0},
		{"Power Cycles", 112, 0}, {"Power On Hours", 128, 0},
		{"Unsafe Shutdowns", 144, 0}, {"Media and Data Integrity Errors", 160, 0},
		{"Error Information Log Entries", 176, 0},
	} {
		count := little128(page[field.at:])
		value, raw := count.String(), strings.ToUpper(count.Text(16))
		if field.unit != 0 {
			value = "未报告"
			if count.Sign() != 0 {
				value = count.Mul(count, big.NewInt(field.unit)).String() + " B"
			}
		}
		health.Metrics = append(health.Metrics, domain.Metric{Name: field.name, Value: value, Raw: raw})
	}
	for i, name := range []string{"Warning Temperature Time", "Critical Temperature Time"} {
		appendUint32Metric(&health, name, page[192+i*4:], " 分钟")
	}
	for i := range 8 {
		if t := kelvin(page[200+i*2:]); t != nil {
			health.Metrics = append(health.Metrics, domain.Metric{Name: fmt.Sprintf("Temperature Sensor %d", i+1), Value: fmt.Sprintf("%d °C", *t), Raw: fmt.Sprintf("%04X", binary.LittleEndian.Uint16(page[200+i*2:]))})
		}
	}
	for i, name := range []string{"Thermal Management 1 Transitions", "Thermal Management 2 Transitions", "Thermal Management 1 Time", "Thermal Management 2 Time"} {
		unit := " 次"
		if i >= 2 {
			unit = " 秒"
		}
		appendUint32Metric(&health, name, page[216+i*4:], unit)
	}
	return health, nil
}

func appendUint32Metric(health *domain.Health, name string, source []byte, unit string) {
	value := binary.LittleEndian.Uint32(source)
	health.Metrics = append(health.Metrics, domain.Metric{Name: name, Value: fmt.Sprintf("%d%s", value, unit), Raw: fmt.Sprintf("%X", value)})
}

func kelvin(data []byte) *int {
	value := int(binary.LittleEndian.Uint16(data))
	if value == 0 || value == 65535 {
		return nil
	}
	celsius := value - 273
	return &celsius
}

func little128(src []byte) *big.Int {
	var be [16]byte
	for i := range be {
		be[len(be)-1-i] = src[i]
	}
	return new(big.Int).SetBytes(be[:])
}

func ATA(data, thresholds []byte) ([]domain.Metric, error) {
	if !validATA(data) || binary.LittleEndian.Uint16(data) == 0 {
		return nil, fmt.Errorf("ATA SMART 数据页缺失或校验失败")
	}
	limits := make(map[byte]byte)
	if validATA(thresholds) {
		for at := 2; at < 362; at += 12 {
			limits[thresholds[at]] = thresholds[at+1]
		}
	}
	var metrics []domain.Metric
	for at := 2; at < 362; at += 12 {
		entry := data[at : at+12]
		if entry[0] == 0 {
			continue
		}
		limit := "—"
		if n, ok := limits[entry[0]]; ok {
			limit = strconv.Itoa(int(n))
		}
		raw := [6]byte{entry[10], entry[9], entry[8], entry[7], entry[6], entry[5]}
		metrics = append(metrics, domain.Metric{
			Name:  fmt.Sprintf("SMART 0x%02X", entry[0]),
			Value: fmt.Sprintf("当前 %d · 最差 %d · 阈值 %s", entry[3], entry[4], limit),
			Raw:   fmt.Sprintf("%X", raw),
		})
	}
	if len(metrics) == 0 {
		return nil, fmt.Errorf("ATA SMART 数据页没有属性")
	}
	return metrics, nil
}

// ATARotationRate reads IDENTIFY DEVICE word 217.
func ATARotationRate(identify []byte) uint16 {
	if len(identify) != 512 {
		return 0
	}
	rate := binary.LittleEndian.Uint16(identify[217*2:])
	if rate == 1 || (rate >= 0x0401 && rate < 0xFFFF) {
		return rate
	}
	return 0
}

func validATA(page []byte) bool {
	if len(page) != 512 {
		return false
	}
	var sum byte
	for _, value := range page {
		sum += value
	}
	return sum == 0
}
