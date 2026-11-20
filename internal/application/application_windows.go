package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"goDisk/internal/benchmark"
	"goDisk/internal/domain"
	platform "goDisk/internal/platform/windows"
	"goDisk/internal/protocol"
	"goDisk/internal/storage"
)

type App struct {
	Store *storage.Store
}

func (a App) Disks(ctx context.Context) ([]domain.Disk, []domain.Volume, error) {
	return platform.Scan(ctx)
}

func (a App) ReadHealth(ctx context.Context, disk domain.Disk) domain.Health {
	return platform.ReadHealth(ctx, disk)
}

func (a App) ValidateBenchmark(config domain.BenchmarkConfig) error {
	return benchmark.Validate(config)
}

func (a App) Benchmark(ctx context.Context, config domain.BenchmarkConfig, disks []domain.Disk, progress func(domain.Progress)) domain.BenchmarkResult {
	var sample func(context.Context, string) domain.TempSample
	for _, disk := range disks {
		if len(config.Volume.Identities) == 1 && disk.ID == config.Volume.Identities[0] && disk.Bus == "NVMe" {
			sample = func(ctx context.Context, name string) domain.TempSample {
				report := platform.ReadHealth(ctx, disk)
				return domain.TempSample{At: report.At, Disk: disk.Number, Profile: name, Celsius: report.Temperature}
			}
			break
		}
	}
	result := benchmark.Run(ctx, config, progress, sample)
	if a.Store != nil {
		saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		record := result
		record.Config.Tool = ""
		if err := a.Store.Add(saveCtx, "benchmark", config.Volume.Root, result.Started, record); err != nil {
			result.Issue = errors.Join(errorText(result.Issue), fmt.Errorf("未保存测试记录: %w", err)).Error()
		}
	}
	return result
}

type capture struct {
	Version int       `json:"version"`
	Source  string    `json:"source"`
	At      time.Time `json:"collected_at"`
	Length  int       `json:"length"`
	Digest  string    `json:"sha256"`
	Page    []byte    `json:"page"`
}

func (a App) ExportHealth(path string, report domain.Health) error {
	if report.Protocol != "NVMe" || len(report.Capture) != 512 {
		return errors.New("当前报告没有可导出的 NVMe 健康页")
	}
	item := capture{Version: 1, Source: "NVMe SMART/Health Log 0x02", At: report.At,
		Length: len(report.Capture), Digest: fmt.Sprintf("%x", sha256.Sum256(report.Capture)), Page: report.Capture}
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	return errors.Join(writeErr, f.Close())
}

func (a App) ImportHealth(path string) (domain.Health, error) {
	f, err := os.Open(path)
	if err != nil {
		return domain.Health{}, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return domain.Health{}, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > 8192 {
		return domain.Health{}, errors.New("健康快照文件格式或大小不符合要求")
	}
	data, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil {
		return domain.Health{}, err
	}
	var item capture
	if len(data) > 8192 || json.Unmarshal(data, &item) != nil || item.Version != 1 || item.Source != "NVMe SMART/Health Log 0x02" ||
		item.At.IsZero() || item.Length != 512 || len(item.Page) != item.Length ||
		item.Digest != fmt.Sprintf("%x", sha256.Sum256(item.Page)) {
		return domain.Health{}, errors.New("快照版本、健康页或摘要无效")
	}
	report, err := protocol.NVMe(item.Page)
	report.At = item.At
	return report, err
}

func errorText(s string) error {
	if s == "" {
		return nil
	}
	return errors.New(s)
}
