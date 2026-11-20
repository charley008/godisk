package benchmark

import (
	"encoding/xml"
	"errors"
	"math"
	"math/bits"

	"goDisk/internal/domain"
)

type target struct {
	Bytes   *uint64 `xml:"BytesCount"`
	Read    uint64  `xml:"ReadBytes"`
	Written uint64  `xml:"WriteBytes"`
	IOCount *uint64 `xml:"IOCount"`
}

type diskSpdResult struct {
	XMLName xml.Name `xml:"Results"`
	Version string   `xml:"System>Tool>Version"`
	Spans   []struct {
		Seconds float64 `xml:"TestTimeSeconds"`
		Threads []struct {
			Targets []target `xml:"Target"`
		} `xml:"Thread"`
		Average *float64 `xml:"Latency>AverageTotalMilliseconds"`
		Buckets []struct {
			Percentile float64  `xml:"Percentile"`
			Total      *float64 `xml:"TotalMilliseconds"`
		} `xml:"Latency>Bucket"`
	} `xml:"TimeSpan"`
}

func parseReport(data []byte, score *domain.Score) (string, error) {
	var report diskSpdResult
	if err := xml.Unmarshal(data, &report); err != nil {
		return "", err
	}
	if report.Version != "2.3.0" || len(report.Spans) != 1 {
		return report.Version, errors.New("DiskSpd 版本或 TimeSpan 数量不符合预期")
	}
	span := report.Spans[0]
	if span.Seconds <= 0 || math.IsNaN(span.Seconds) || math.IsInf(span.Seconds, 0) {
		return report.Version, errors.New("DiskSpd 未产生有效测量")
	}
	parsed := *score
	parsed.Seconds = span.Seconds
	for _, thread := range span.Threads {
		for _, t := range thread.Targets {
			if t.Bytes == nil || t.IOCount == nil || !add(&parsed.Bytes, *t.Bytes) ||
				!add(&parsed.Operations, *t.IOCount) || !add(&parsed.ReadBytes, t.Read) || !add(&parsed.WriteBytes, t.Written) {
				return report.Version, errors.New("DiskSpd I/O 计数缺失或溢出")
			}
		}
	}
	if parsed.Operations == 0 {
		return report.Version, errors.New("DiskSpd 未报告已完成的 I/O")
	}
	parsed.AverageMS = finiteLatency(span.Average)
	for _, b := range span.Buckets {
		if b.Percentile == 99 {
			parsed.P99MS = finiteLatency(b.Total)
			break
		}
	}
	*score = parsed
	return report.Version, nil
}

func add(dest *uint64, value uint64) bool {
	sum, carry := bits.Add64(*dest, value, 0)
	*dest = sum
	return carry == 0
}

func finiteLatency(value *float64) *float64 {
	if value == nil || *value < 0 || math.IsInf(*value, 0) || math.IsNaN(*value) {
		return nil
	}
	return value
}
