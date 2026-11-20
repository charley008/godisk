package protocol

import (
	"encoding/binary"
	"fmt"
	"strings"

	"goDisk/internal/domain"
)

func NVMeIdentifyController(page []byte) domain.DeviceDetails {
	if len(page) != 4096 || binary.LittleEndian.Uint16(page) == 0 {
		return domain.DeviceDetails{}
	}
	version := binary.LittleEndian.Uint32(page[80:84])
	details := domain.DeviceDetails{
		Serial:             identifyText(page[4:24]),
		SMART:              true,
		TRIM:               page[520]&0x04 != 0,
		VolatileWriteCache: page[525]&0x01 != 0,
	}
	if version != 0 && version != 0xffffffff && version>>16 != 0 {
		details.Standard = fmt.Sprintf("NVMe %d.%d", version>>16, (version>>8)&0xff)
		if patch := version & 0xff; patch != 0 {
			details.Standard += fmt.Sprintf(".%d", patch)
		}
	}
	return details
}

func ATAIdentifyDevice(page []byte) domain.DeviceDetails {
	if len(page) != 512 {
		return domain.DeviceDetails{}
	}
	word := func(i int) uint16 { return binary.LittleEndian.Uint16(page[i*2:]) }
	var serial [20]byte
	for i := 0; i < len(serial); i += 2 {
		serial[i], serial[i+1] = page[20+i+1], page[20+i]
	}
	details := domain.DeviceDetails{Serial: identifyText(serial[:])}
	if supported := word(82); supported != 0xffff {
		details.SMART = supported&1 != 0
		details.VolatileWriteCache = supported&(1<<5) != 0
	}
	if trim := word(169); trim != 0xffff {
		details.TRIM = trim&1 != 0
	}
	if revisions := word(80); revisions != 0xffff {
		for revision := 14; revision >= 1; revision-- {
			if revisions&(1<<revision) != 0 {
				switch {
				case revision >= 9:
					details.Standard = fmt.Sprintf("ACS-%d", revision-7)
				case revision == 8:
					details.Standard = "ATA8-ACS"
				case revision >= 4:
					details.Standard = fmt.Sprintf("ATA/ATAPI-%d", revision)
				default:
					details.Standard = fmt.Sprintf("ATA-%d", revision)
				}
				break
			}
		}
	}
	support, status := word(76), word(77)
	if support != 0 && support != 0xffff {
		for rate := 3; rate >= 1; rate-- {
			if support&(1<<rate) != 0 {
				details.MaxLink = sataLink(rate)
				break
			}
		}
	}
	if status != 0xffff {
		details.CurrentLink = sataLink(int(status>>1) & 7)
	}
	return details
}

func sataLink(generation int) string {
	switch generation {
	case 1:
		return "SATA/150"
	case 2:
		return "SATA/300"
	case 3:
		return "SATA/600"
	default:
		return ""
	}
}

func identifyText(raw []byte) string {
	for _, b := range raw {
		if (b < 0x20 && b != 0) || b > 0x7e {
			return ""
		}
	}
	return strings.Trim(string(raw), "\x00 ")
}
