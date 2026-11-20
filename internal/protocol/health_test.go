package protocol

import (
	"encoding/binary"
	"testing"
)

func TestATARotationRate(t *testing.T) {
	for _, test := range []struct{ identifyWord, expected uint16 }{
		{0, 0}, {1, 1}, {0x0400, 0}, {5400, 5400}, {7200, 7200}, {0xffff, 0},
	} {
		page := make([]byte, 512)
		binary.LittleEndian.PutUint16(page[217*2:], test.identifyWord)
		if got := ATARotationRate(page); got != test.expected {
			t.Errorf("word %04X: got %d, want %d", test.identifyWord, got, test.expected)
		}
	}
	if got := ATARotationRate(make([]byte, 511)); got != 0 {
		t.Fatalf("unexpected rate from a short page: %d", got)
	}
}

func TestNVMePreservesWarningsAndWideCounters(t *testing.T) {
	page := make([]byte, 512)
	page[0], page[3], page[4], page[5] = 1, 95, 10, 125
	binary.LittleEndian.PutUint16(page[1:], 318)
	page[128+8] = 1
	got, err := NVMe(page)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "有警告" || got.Temperature == nil || *got.Temperature != 45 {
		t.Fatalf("unexpected health report: %+v", got)
	}
	for _, m := range got.Metrics {
		if m.Name == "Power On Hours" {
			if m.Value != "18446744073709551616" {
				t.Fatalf("power-on hours truncated: %s", m.Value)
			}
			return
		}
	}
	t.Fatal("no power-on hours")
}

func TestNVMeRejectsEmptyAndTruncatedPages(t *testing.T) {
	for _, data := range [][]byte{nil, make([]byte, 511), make([]byte, 512)} {
		if _, err := NVMe(data); err == nil {
			t.Fatalf("accepted invalid page length=%d", len(data))
		}
	}
}
