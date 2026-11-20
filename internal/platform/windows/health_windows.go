package windows

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"goDisk/internal/domain"
	"goDisk/internal/protocol"
	win "golang.org/x/sys/windows"
)

func ReadHealth(ctx context.Context, disk domain.Disk) domain.Health {
	report := domain.Health{State: "未知"}
	var err error
	switch disk.Bus {
	case "NVMe":
		report, err = readNVMe(disk)
	case "ATA", "SATA":
		report, err = readATA(ctx, disk)
	default:
		err = fmt.Errorf("该总线的 SMART 采集尚未支持 (%s)", disk.Bus)
	}
	report.DiskID, report.At = disk.ID, time.Now()
	if report.RotationRate == 0 {
		report.RotationRate = disk.RotationRate
	}
	if report.Protocol == "" {
		report.Protocol = disk.Bus
	}
	if err != nil {
		report.State = "未知"
		report.Issue = err.Error()
		if errors.Is(err, win.ERROR_ACCESS_DENIED) {
			report.Issue = "权限不足。可在可信环境下手动以管理员身份运行，再单独验证该设备。 " + report.Issue
		}
	}
	return report
}

func readNVMe(disk domain.Disk) (domain.Health, error) {
	h, err := open(disk.Path, 0, shareDisk, 0)
	if err != nil {
		return domain.Health{}, fmt.Errorf("打开 NVMe: %w", err)
	}
	defer win.CloseHandle(h)
	health, err := nvmeHealthPage(h, 50)
	if errors.Is(err, win.ERROR_NOT_SUPPORTED) || errors.Is(err, win.ERROR_INVALID_FUNCTION) || errors.Is(err, win.ERROR_INVALID_PARAMETER) {
		health, err = nvmeHealthPage(h, 49)
	}
	if err != nil {
		return domain.Health{}, fmt.Errorf("读取 NVMe SMART/Health: %w", err)
	}
	return protocol.NVMe(health)
}

func nvmeHealthPage(h win.Handle, property uint32) ([]byte, error) {
	return protocolPage(h, property, 3, 2, 2, 512)
}

func protocolPage(h win.Handle, property, transport, dataType, request, pageSize uint32) ([]byte, error) {
	const header = 48
	buf := make([]byte, header+pageSize)
	for i, value := range []uint32{property, 0, transport, dataType, request, 0, 40, pageSize} {
		binary.LittleEndian.PutUint32(buf[i*4:], value)
	}
	n, err := query(h, queryStorageProperty, buf, buf)
	if err != nil {
		return nil, err
	}
	if n < header || n > len(buf) || binary.LittleEndian.Uint32(buf) != header ||
		binary.LittleEndian.Uint32(buf[4:]) != header || binary.LittleEndian.Uint32(buf[8:]) != transport ||
		binary.LittleEndian.Uint32(buf[12:]) != dataType {
		return nil, fmt.Errorf("存储协议描述符无效 (%d 字节)", n)
	}
	offset := uint64(binary.LittleEndian.Uint32(buf[24:])) + 8
	length := uint64(binary.LittleEndian.Uint32(buf[28:]))
	if offset < header || length < uint64(pageSize) || offset+length > uint64(n) {
		return nil, fmt.Errorf("存储协议数据超出返回缓冲区")
	}
	return buf[offset : offset+uint64(pageSize)], nil
}

func readATA(ctx context.Context, disk domain.Disk) (domain.Health, error) {
	report := domain.Health{Protocol: "ATA", State: "未知"}
	h, err := open(disk.Path, win.GENERIC_READ|win.GENERIC_WRITE, shareDisk, 0)
	if err != nil {
		return report, fmt.Errorf("打开 SATA 透传句柄: %w", err)
	}
	defer win.CloseHandle(h)
	if disk.RotationRate == 0 {
		if identify, _, err := ataTask(h, [7]byte{0, 0, 0, 0, 0, 0xa0, 0xec}, true); err == nil {
			report.RotationRate = protocol.ATARotationRate(identify)
			details := protocol.ATAIdentifyDevice(identify)
			report.Details = &details
		}
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	data, _, err := ataSMART(h, 0xd0, true)
	if err != nil {
		return report, fmt.Errorf("SMART READ DATA: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	thresholds, _, thresholdErr := ataSMART(h, 0xd1, true)
	report.Metrics, err = protocol.ATA(data, thresholds)
	if err != nil {
		return report, err
	}
	if thresholdErr != nil {
		report.Issue = "阈值读取失败 " + thresholdErr.Error()
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	_, status, err := ataSMART(h, 0xda, false)
	switch {
	case err != nil:
		report.Issue = errors.Join(optionalError(report.Issue), fmt.Errorf("SMART RETURN STATUS: %w", err)).Error()
	case status == 0xc24f:
		report.State = "未报告阈值告警"
	case status == 0x2cf4:
		report.State = "有警告"
	default:
		report.Issue = errors.Join(optionalError(report.Issue), fmt.Errorf("未识别的 ATA SMART 状态 0x%04X", status)).Error()
	}
	return report, nil
}

func optionalError(value string) error {
	if value == "" {
		return nil
	}
	return errors.New(value)
}

func ataSMART(h win.Handle, feature byte, dataIn bool) ([]byte, uint16, error) {
	return ataTask(h, [7]byte{feature, 1, 0, 0x4f, 0xc2, 0xa0, 0xb0}, dataIn)
}

func ataTask(h win.Handle, task [7]byte, dataIn bool) ([]byte, uint16, error) {
	const header, ioctl = 48, 0x0004d02c // Windows AMD64.
	length, flags := 0, uint16(1)
	if dataIn {
		length, flags = 512, 3
	}
	buf := make([]byte, header+length)
	binary.LittleEndian.PutUint16(buf, header)
	binary.LittleEndian.PutUint16(buf[2:], flags)
	binary.LittleEndian.PutUint32(buf[8:], uint32(length))
	binary.LittleEndian.PutUint32(buf[12:], 4)
	if dataIn {
		binary.LittleEndian.PutUint64(buf[24:], header)
	}
	copy(buf[40:], task[:])
	n, err := query(h, ioctl, buf[:header], buf)
	if err != nil {
		return nil, 0, err
	}
	if n != len(buf) || int(binary.LittleEndian.Uint32(buf[8:])) != length {
		return nil, 0, fmt.Errorf("ATA 返回长度无效 (%d 字节)", n)
	}
	if buf[46]&0xa9 != 0 {
		return nil, 0, fmt.Errorf("ATA status 0x%02X, error 0x%02X", buf[46], buf[40])
	}
	return buf[header:], binary.LittleEndian.Uint16(buf[43:]), nil
}
