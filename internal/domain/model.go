package domain

import (
	"fmt"
	"time"
)

const (
	Sequential = "顺序 1 MiB"
	Random     = "随机 4 KiB"
	Both       = "顺序 + 随机"

	DefaultBenchmarkSizeMiB = 1024
)

func BenchmarkFileSizes() []int {
	return []int{16, 32, 64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384, 32768, 65536}
}

func BenchmarkSizeLabel(sizeMiB int) string {
	if sizeMiB >= 1024 && sizeMiB%1024 == 0 {
		return fmt.Sprintf("%d GiB", sizeMiB/1024)
	}
	return fmt.Sprintf("%d MiB", sizeMiB)
}

type Disk struct {
	ID, Path, Model, Firmware, Bus string
	Number                         int
	Capacity                       uint64
	RotationRate                   uint16 // 0 unknown, 1 non-rotating, otherwise RPM.
	Details                        DeviceDetails
	Issue                          string
}

type DeviceDetails struct {
	Serial                          string `json:"-"`
	Standard, CurrentLink, MaxLink  string
	SMART, TRIM, VolatileWriteCache bool
}

type Volume struct {
	Root, GUID, Label string
	Disks             []int
	Identities        []string
	Free              uint64
	System            bool
}

type Metric struct {
	Name, Value string
	Raw         string
}

type Health struct {
	DiskID         string
	Protocol       string
	At             time.Time
	State          string
	Temperature    *int
	PercentageUsed *int
	RotationRate   uint16
	Details        *DeviceDetails `json:",omitempty"`
	Metrics        []Metric
	Issue          string
	Capture        []byte
}

type BenchmarkConfig struct {
	Volume          Volume
	Tool            string
	Mode            string
	SizeMiB         int
	Seconds         int
	WarmupSeconds   int
	IntervalSeconds int
	Threads         int
	Queue           int
	Repetitions     int
	Writes          bool
	Standard        bool
	PeakRounds      bool
}

type Score struct {
	Profile    string
	Seconds    float64
	Bytes      uint64
	Operations uint64
	AverageMS  *float64
	P99MS      *float64
	ReadBytes  uint64
	WriteBytes uint64
	ExitCode   int
	Complete   bool
	Workload   string
	Write      bool
}

type TempSample struct {
	At      time.Time
	Disk    int
	Profile string
	Celsius *int
}

type BenchmarkResult struct {
	Config       BenchmarkConfig
	Started      time.Time
	State        string
	Version      string
	Scores       []Score
	Temperatures []TempSample
	Issue        string
}

type Progress struct {
	Phase    string
	Fraction float64
	Score    *Score
}

type Workload struct {
	Name           string
	Block          string
	Random         bool
	Queue, Threads int
}

func (w Workload) Key() string {
	return fmt.Sprintf("%s Q%dT%d", w.Name, w.Queue, w.Threads)
}

func Workloads(config BenchmarkConfig) []Workload {
	var list []Workload
	if config.Mode != Random {
		if config.Standard {
			list = append(list, Workload{Sequential, "1M", false, 8, 1}, Workload{Sequential, "1M", false, 1, 1})
		} else {
			list = append(list, Workload{Sequential, "1M", false, config.Queue, config.Threads})
		}
	}
	if config.Mode != Sequential {
		if config.Standard {
			list = append(list, Workload{Random, "4K", true, 32, 1}, Workload{Random, "4K", true, 1, 1})
		} else {
			list = append(list, Workload{Random, "4K", true, config.Queue, config.Threads})
		}
	}
	return list
}
