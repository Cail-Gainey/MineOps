package service

import (
	"context"
	"net"
	"sort"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

const sshLatencySampleCount = 5

// SSHLatencyResult contains authenticated SSH request round-trip statistics.
type SSHLatencyResult struct {
	Minimum time.Duration
	Median  time.Duration
	Average time.Duration
	Maximum time.Duration
	Samples int
}

// SSHPreflightResult contains route-aware DNS and authenticated SSH latency evidence.
type SSHPreflightResult struct {
	Addresses   []string
	Address     string
	DNSDuration time.Duration
	Latency     SSHLatencyResult
	Config      model.EffectiveSSHConfig
}

// Preflight authenticates through the configured direct or Jump Host route and measures SSH request RTT.
func (f *SSHClientFactory) Preflight(ctx context.Context, session *model.SSHSession, settings model.SSHSettings) (SSHPreflightResult, error) {
	if session == nil {
		return SSHPreflightResult{}, apperror.New(apperror.CodeValidationRequired, "SSH Session 不能为空")
	}
	config := model.ResolveSSHConfig(*session, settings)
	dnsStarted := time.Now()
	addresses, _ := net.DefaultResolver.LookupHost(ctx, session.Host)
	dnsDuration := time.Since(dnsStarted)
	client, err := f.Connect(ctx, session, settings)
	if err != nil {
		return SSHPreflightResult{}, err
	}
	defer func() { _ = client.Close() }()
	latency, err := client.MeasureLatency(ctx, sshLatencySampleCount, time.Duration(config.ConnectTimeoutSec)*time.Second)
	if err != nil {
		return SSHPreflightResult{}, err
	}
	return SSHPreflightResult{
		Addresses: addresses, Address: client.RemoteAddress(), DNSDuration: dnsDuration,
		Latency: latency, Config: config,
	}, nil
}

// MeasureLatency sends authenticated SSH global requests and summarizes their round-trip durations.
func (c *SSHClient) MeasureLatency(ctx context.Context, sampleCount int, timeout time.Duration) (SSHLatencyResult, error) {
	if c == nil || c.client == nil {
		return SSHLatencyResult{}, apperror.New(apperror.CodeSSHConnectionFailed, "SSH Client 不可用")
	}
	if sampleCount < 1 {
		return SSHLatencyResult{}, apperror.New(apperror.CodeValidationInvalidArgument, "SSH 延迟采样次数必须大于零")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if _, err := c.measureLatencySample(ctx, timeout); err != nil {
		return SSHLatencyResult{}, err
	}
	samples := make([]time.Duration, 0, sampleCount)
	for range sampleCount {
		duration, err := c.measureLatencySample(ctx, timeout)
		if err != nil {
			return SSHLatencyResult{}, err
		}
		samples = append(samples, duration)
	}
	return summarizeSSHLatency(samples), nil
}

func (c *SSHClient) measureLatencySample(ctx context.Context, timeout time.Duration) (time.Duration, error) {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type probeResult struct {
		duration time.Duration
		err      error
	}
	completed := make(chan probeResult, 1)
	go func() {
		duration, err := c.sendKeepAliveRequest()
		completed <- probeResult{duration: duration, err: err}
	}()
	select {
	case <-probeCtx.Done():
		_ = c.client.Close()
		return 0, apperror.Wrap(apperror.CodeSSHConnectionFailed, "SSH 延迟探测超时", probeCtx.Err()).WithDetails(map[string]any{
			"stage": "latency",
		})
	case result := <-completed:
		if result.err != nil {
			return 0, apperror.Wrap(apperror.CodeSSHConnectionFailed, "SSH 延迟探测失败", result.err).WithDetails(map[string]any{
				"stage": "latency",
			})
		}
		return result.duration, nil
	}
}

func summarizeSSHLatency(samples []time.Duration) SSHLatencyResult {
	if len(samples) == 0 {
		return SSHLatencyResult{}
	}
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(left, right int) bool { return sorted[left] < sorted[right] })
	var total time.Duration
	for _, sample := range sorted {
		total += sample
	}
	median := sorted[len(sorted)/2]
	if len(sorted)%2 == 0 {
		median = (sorted[len(sorted)/2-1] + sorted[len(sorted)/2]) / 2
	}
	return SSHLatencyResult{
		Minimum: sorted[0], Median: median, Average: total / time.Duration(len(sorted)),
		Maximum: sorted[len(sorted)-1], Samples: len(sorted),
	}
}
