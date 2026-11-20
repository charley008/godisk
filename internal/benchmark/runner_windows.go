package benchmark

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"goDisk/internal/domain"
	platform "goDisk/internal/platform/windows"
	win "golang.org/x/sys/windows"
)

// Release builds set this to the SHA-256 of the DiskSpd built with the package.
var engineHash = "dd4e57e1e8ccaf5d6437938f8aab7f17e9a1e6d8fba8a093006b7cadf16faea2"

const engineSetupHint = "请重新解压完整的 goDisk 压缩包，保留 diskspd 文件夹"

type profile struct {
	domain.Workload
	write bool
}

func profiles(config domain.BenchmarkConfig) []profile {
	var list []profile
	for _, workload := range domain.Workloads(config) {
		list = append(list, profile{Workload: workload})
	}
	if config.Writes {
		for _, read := range list {
			read.write = true
			list = append(list, read)
		}
	}
	return list
}

func Validate(config domain.BenchmarkConfig) error {
	if config.Mode != domain.Both && config.Mode != domain.Random && config.Mode != domain.Sequential {
		return errors.New("请选择测速项目")
	}
	if !slices.Contains(domain.BenchmarkFileSizes(), config.SizeMiB) || config.Seconds < 1 || config.Seconds > 30 ||
		config.WarmupSeconds < 0 || config.WarmupSeconds > 30 || config.IntervalSeconds < 0 || config.IntervalSeconds > 30 ||
		config.Threads < 1 || config.Threads > 8 || config.Queue < 1 || config.Queue > 32 ||
		config.Repetitions < 1 || config.Repetitions > 5 {
		return errors.New("测速参数超出允许范围")
	}
	if len(config.Volume.Disks) != 1 || len(config.Volume.Identities) != 1 {
		return errors.New("请选择能映射到单块物理盘的卷")
	}
	if !filepath.IsAbs(config.Tool) || !strings.EqualFold(filepath.Base(config.Tool), "diskspd.exe") {
		return errors.New("测速组件路径不可用 · " + engineSetupHint)
	}
	return nil
}

func pinTool(path string) (*os.File, error) {
	p, err := win.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := win.CreateFile(p, win.GENERIC_READ, win.FILE_SHARE_READ, nil, win.OPEN_EXISTING, win.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if errors.Is(err, win.ERROR_FILE_NOT_FOUND) || errors.Is(err, win.ERROR_PATH_NOT_FOUND) {
		return nil, errors.New(engineSetupHint)
	}
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(h), path)
	var info win.ByHandleFileInformation
	if err = win.GetFileInformationByHandle(h, &info); err != nil ||
		info.FileAttributes&(win.FILE_ATTRIBUTE_REPARSE_POINT|win.FILE_ATTRIBUTE_DIRECTORY) != 0 ||
		info.FileSizeHigh != 0 || info.FileSizeLow > 32<<20 {
		file.Close()
		return nil, errors.New("测速组件路径或文件类型异常")
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		file.Close()
		return nil, err
	}
	hash := fmt.Sprintf("%x", digest.Sum(nil))
	if hash != engineHash {
		file.Close()
		return nil, errors.New("测速组件校验失败 · " + engineSetupHint)
	}
	return file, nil
}

func Run(ctx context.Context, config domain.BenchmarkConfig, progress func(domain.Progress), sample func(context.Context, string) domain.TempSample) (result domain.BenchmarkResult) {
	result = domain.BenchmarkResult{Config: config, Started: time.Now(), State: "failed"}
	defer func() {
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			result.State = "timed_out"
		case errors.Is(ctx.Err(), context.Canceled):
			result.State = "cancelled"
		}
	}()
	if err := Validate(config); err != nil {
		result.Issue = err.Error()
		return
	}
	tool, err := pinTool(config.Tool)
	if err != nil {
		result.Issue = "测速组件不可用 · " + err.Error()
		return
	}
	defer tool.Close()
	volume, err := platform.CheckVolume(ctx, config.Volume)
	if err != nil {
		result.Issue = err.Error()
		return
	}
	size := int64(config.SizeMiB) << 20
	if volume.Free < uint64(size)+(64<<20) {
		result.Issue = "卷空间不足，需容纳测试文件并保留至少 64 MiB"
		return
	}
	progress(domain.Progress{Phase: "准备测试文件"})
	dir, err := os.MkdirTemp(volume.GUID, "goDisk-test-")
	if err != nil {
		result.Issue = fmt.Sprintf("无法在 %s 创建测试目录: %v", volume.Root, err)
		return
	}
	dirPin, err := platform.PinBenchmarkPath(dir, volume.GUID, true)
	if err != nil {
		result.Issue = fmt.Sprintf("无法锁定测试目录 %s: %v", dir, err)
		_ = os.Remove(dir)
		return
	}
	path := filepath.Join(dir, "goDisk-test.dat")
	var filePin win.Handle
	var owned os.FileInfo
	defer func() {
		progress(domain.Progress{Phase: "清理测试文件", Fraction: 1})
		if filePin != 0 {
			win.CloseHandle(filePin)
		}
		if owned != nil {
			now, statErr := os.Lstat(path)
			if statErr == nil && now.Mode().IsRegular() && os.SameFile(owned, now) {
				statErr = os.Remove(path)
			} else if statErr == nil {
				statErr = errors.New("测试文件身份已改变")
			}
			if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
				appendIssue(&result, fmt.Sprintf("清理失败，请检查 %s (%v)", path, statErr))
			}
		}
		win.CloseHandle(dirPin)
		if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
			appendIssue(&result, fmt.Sprintf("测试目录遗留 %s (%v)", dir, err))
		}
	}()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		result.Issue = err.Error()
		return
	}
	owned, err = f.Stat()
	if err == nil {
		err = fill(ctx, f, size)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		result.Issue = fmt.Sprintf("准备测试文件: %v", err)
		return
	}
	filePin, err = platform.PinBenchmarkPath(path, volume.GUID, false)
	if err != nil {
		result.Issue = err.Error()
		return
	}
	if current, err := os.Lstat(path); err != nil || !os.SameFile(owned, current) || current.Size() != size {
		result.Issue = "测试文件身份或大小已改变"
		return
	}
	list := profiles(config)
	total := len(list) * config.Repetitions
	for i, item := range list {
		if i != 0 && config.IntervalSeconds != 0 {
			progress(domain.Progress{Phase: "测试间隔", Fraction: float64(i*config.Repetitions) / float64(total)})
			timer := time.NewTimer(time.Duration(config.IntervalSeconds) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		for repetition := range config.Repetitions {
			if ctx.Err() != nil {
				return
			}
			step := i*config.Repetitions + repetition
			direction := "读取"
			if item.write {
				direction = "写入"
			}
			label := fmt.Sprintf("%s · %s · %d/%d", item.Key(), direction, repetition+1, config.Repetitions)
			warmup := 0
			if repetition == 0 {
				warmup = config.WarmupSeconds
			}
			score, version, samples, err := runProfile(ctx, config, warmup, path, item, label, func(phase string, fraction float64) {
				progress(domain.Progress{Phase: label + " · " + phase, Fraction: (float64(step) + fraction) / float64(total)})
			}, sample)
			result.Version = version
			result.Scores = append(result.Scores, score)
			result.Temperatures = append(result.Temperatures, samples...)
			progress(domain.Progress{Phase: label, Fraction: float64(step+1) / float64(total), Score: &score})
			if err != nil {
				result.Issue = err.Error()
				if errors.Is(err, context.DeadlineExceeded) {
					result.State = "timed_out"
				}
				return
			}
		}
	}
	result.State = "completed"
	return
}

func appendIssue(result *domain.BenchmarkResult, issue string) {
	if result.Issue != "" {
		result.Issue += "\n"
	}
	result.Issue += issue
}

func fill(ctx context.Context, file *os.File, size int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := file.Truncate(size); err != nil {
		return err
	}
	buffer := make([]byte, min(size, 1<<20))
	rand.Read(buffer)
	for remaining := size; remaining > 0; {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk := buffer[:min(remaining, int64(len(buffer)))]
		n, err := file.Write(chunk)
		if err != nil {
			return err
		}
		remaining -= int64(n)
	}
	return file.Sync()
}

type namedEvent struct {
	handle win.Handle
	name   string
}

func newEvent() (namedEvent, error) {
	name := "Local\\goDisk-" + rand.Text()
	p, _ := win.UTF16PtrFromString(name)
	h, err := win.CreateEvent(nil, 1, 0, p)
	if err != nil {
		return namedEvent{}, err
	}
	return namedEvent{h, name}, nil
}

func runProfile(ctx context.Context, config domain.BenchmarkConfig, warmup int, path string, item profile, label string,
	progress func(string, float64), sample func(context.Context, string) domain.TempSample) (domain.Score, string, []domain.TempSample, error) {
	score := domain.Score{Profile: label, Workload: item.Key(), Write: item.write, ExitCode: -1}
	var events [2]namedEvent
	for i := range events {
		event, err := newEvent()
		if err != nil {
			return score, "", nil, err
		}
		events[i] = event
		defer win.CloseHandle(event.handle)
	}
	gate, stop := events[0], events[1]
	job, err := win.CreateJobObject(nil, nil)
	if err != nil {
		return score, "", nil, err
	}
	defer win.CloseHandle(job)
	limit := win.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limit.BasicLimitInformation.LimitFlags = win.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = win.SetInformationJobObject(job, win.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limit)), uint32(unsafe.Sizeof(limit))); err != nil {
		return score, "", nil, err
	}
	args := []string{
		"-Rxml", "-L", "-Su", "-ag", "-W" + strconv.Itoa(warmup), "-C0", "-b" + item.Block,
		"-d" + strconv.Itoa(config.Seconds), "-o" + strconv.Itoa(item.Queue), "-t" + strconv.Itoa(item.Threads),
		"-yr" + gate.name, "-yp" + stop.name,
	}
	if item.Random {
		args = append(args, "-r4K")
	}
	if item.write {
		args = append(args, "-w100", "-Z"+item.Block)
	} else {
		args = append(args, "-w0")
	}
	cmd := exec.Command(config.Tool, append(args, path)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: win.CREATE_NO_WINDOW}
	var stdout, stderr limitedBuffer
	stdout.max, stderr.max = 8<<20, 64<<10
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = 3 * time.Second
	if err = cmd.Start(); err != nil {
		return score, "", nil, err
	}
	process, err := win.OpenProcess(win.PROCESS_SET_QUOTA|win.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = win.AssignProcessToJobObject(job, process)
		win.CloseHandle(process)
	}
	if err == nil {
		err = win.SetEvent(gate.handle)
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return score, "", nil, fmt.Errorf("启动受控 DiskSpd 任务: %w", err)
	}
	gateOpened := time.Now()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	taskCtx, cancel := context.WithTimeout(ctx, time.Duration(warmup+config.Seconds+30)*time.Second)
	defer cancel()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	sampleCtx, stopSample := context.WithCancel(taskCtx)
	defer stopSample()
	sampleCh := make(chan domain.TempSample, 24)
	if sample != nil {
		go collectSamples(sampleCtx, sampleCh, sample, label)
	}
	if warmup > 0 {
		progress("预热", 0)
	}
	var terminal error
loop:
	for {
		select {
		case terminal = <-done:
			break loop
		case <-taskCtx.Done():
			_ = win.SetEvent(stop.handle)
			timer := time.NewTimer(3 * time.Second)
			select {
			case <-done:
			case <-timer.C:
				_ = win.TerminateJobObject(job, 1)
				_ = cmd.Process.Kill()
				<-done
			}
			timer.Stop()
			terminal = taskCtx.Err()
			break loop
		case now := <-tick.C:
			// The progress estimate excludes this profile's warmup.
			// The subprocess XML determines completion.
			estimated := now.Sub(gateOpened).Seconds() - float64(warmup)
			if estimated >= 0 {
				progress("测量中（估计）", math.Min(.98, estimated/float64(config.Seconds)))
			}
		}
	}
	stopSample()
	var samples []domain.TempSample
drain:
	for {
		select {
		case value := <-sampleCh:
			samples = append(samples, value)
		default:
			break drain
		}
	}
	score.ExitCode = cmd.ProcessState.ExitCode()
	version, parseErr := parseReport(stdout.buffer.Bytes(), &score)
	if terminal == nil && taskCtx.Err() != nil {
		terminal = taskCtx.Err()
	}
	if terminal != nil {
		if stderr.buffer.Len() != 0 && !errors.Is(terminal, context.Canceled) {
			terminal = fmt.Errorf("%w · %s", terminal, strings.TrimSpace(stderr.buffer.String()))
		}
		return score, version, samples, terminal
	}
	if stdout.overflow {
		parseErr = errors.New("DiskSpd XML 超出大小限制")
	}
	if parseErr != nil {
		return score, version, samples, fmt.Errorf("解析 DiskSpd XML: %w", parseErr)
	}
	if score.Seconds < float64(config.Seconds)*.95 {
		return score, version, samples, errors.New("DiskSpd 未完成预定测量")
	}
	score.Complete = true
	return score, version, samples, nil
}

func collectSamples(ctx context.Context, out chan<- domain.TempSample, sample func(context.Context, string) domain.TempSample, label string) {
	timer := time.NewTicker(2 * time.Second)
	defer timer.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		result := sample(ctx, label)
		select {
		case out <- result:
		case <-ctx.Done():
			return
		}
		select {
		case <-timer.C:
		case <-ctx.Done():
			return
		}
	}
}

type limitedBuffer struct {
	buffer   bytes.Buffer
	max      int
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	available := min(len(p), b.max-b.buffer.Len())
	if available < len(p) {
		b.overflow = true
	}
	_, _ = b.buffer.Write(p[:available])
	return len(p), nil
}
