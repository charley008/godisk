package protocol

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
)

func TestNVMeIdentifyDetails(t *testing.T) {
	page := make([]byte, 4096)
	binary.LittleEndian.PutUint16(page, 0x144d)
	copy(page[4:24], "EXAMPLEDEVICE0123   ")
	binary.LittleEndian.PutUint32(page[80:], 0x00010300)
	page[520], page[525] = 4, 1
	details := NVMeIdentifyController(page)
	if details.Serial != "EXAMPLEDEVICE0123" || details.Standard != "NVMe 1.3" ||
		!details.SMART || !details.TRIM || !details.VolatileWriteCache {
		t.Fatalf("invalid identify details: %+v", details)
	}
	encoded, err := json.Marshal(details)
	if err != nil || strings.Contains(string(encoded), details.Serial) {
		t.Fatal("serial should not be serialized", err)
	}
}

func TestATAIdentifyDetails(t *testing.T) {
	page := make([]byte, 512)
	copy(page[20:40], "NS21                ")
	for _, v := range []struct{ word, value uint16 }{{76, 0x0e}, {77, 4}, {80, 1 << 8}, {82, 0x21}, {169, 1}} {
		binary.LittleEndian.PutUint16(page[v.word*2:], v.value)
	}
	details := ATAIdentifyDevice(page)
	if details.Serial != "SN12" || details.Standard != "ATA8-ACS" || details.CurrentLink != "SATA/300" || details.MaxLink != "SATA/600" ||
		!details.SMART || !details.TRIM || !details.VolatileWriteCache {
		t.Fatalf("invalid ATA identify details: %+v", details)
	}
}
