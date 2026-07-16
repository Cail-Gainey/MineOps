// Package httpclient provides the bounded, retry-aware HTTP baseline shared by external catalogs and downloads.
package httpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/constants"
)

// Config contains safe HTTP transport, retry, and response limits.
type Config struct {
	UserAgent                    string
	RequestTimeout               time.Duration
	OverallTimeout               time.Duration
	DialTimeout                  time.Duration
	TLSHandshakeTimeout          time.Duration
	IdleConnTimeout              time.Duration
	MaxIdleConnections           int
	Retries                      int
	RetryBackoff                 time.Duration
	MaximumResponseSize          int64
	ProxyMode                    string
	ProxyHost                    string
	ProxyPort                    uint16
	ProxyUsername                string
	ProxyPassword                string
	ProxyBypass                  []string
	Concurrency                  int
	BandwidthLimitBytesPerSecond int64
}

// Client owns one reusable connection pool and normalized request behavior.
type Client struct {
	mu                           sync.RWMutex
	client                       *http.Client
	userAgent                    string
	retries                      int
	retryBackoff                 time.Duration
	maximumResponseSize          int64
	bandwidthLimitBytesPerSecond int64
	semaphore                    chan struct{}
}

// New creates an HTTP client with conservative defaults and a reusable connection pool.
func New(config Config) *Client {
	client := &Client{}
	_ = client.Reload(config)
	return client
}

// Reload atomically replaces the transport so existing requests finish on the old pool and new requests use current settings.
func (c *Client) Reload(config Config) error {
	if config.UserAgent == "" {
		config.UserAgent = constants.ApplicationUserAgent
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 30 * time.Second
	}
	if config.OverallTimeout < config.RequestTimeout {
		config.OverallTimeout = config.RequestTimeout
	}
	if config.RetryBackoff < 0 {
		config.RetryBackoff = 0
	}
	if config.DialTimeout <= 0 {
		config.DialTimeout = 10 * time.Second
	}
	if config.TLSHandshakeTimeout <= 0 {
		config.TLSHandshakeTimeout = 10 * time.Second
	}
	if config.IdleConnTimeout <= 0 {
		config.IdleConnTimeout = 90 * time.Second
	}
	if config.MaxIdleConnections <= 0 {
		config.MaxIdleConnections = 32
	}
	if config.Retries < 0 {
		config.Retries = 0
	}
	if config.Retries > 3 {
		config.Retries = 3
	}
	if config.MaximumResponseSize <= 0 {
		config.MaximumResponseSize = 8 * 1024 * 1024
	}
	if config.Concurrency <= 0 {
		config.Concurrency = 4
	}
	if config.Concurrency > 32 {
		config.Concurrency = 32
	}
	proxyFunction := http.ProxyFromEnvironment
	dialContext := (&net.Dialer{Timeout: config.DialTimeout, KeepAlive: 30 * time.Second}).DialContext
	switch config.ProxyMode {
	case "", "system":
	case "none":
		proxyFunction = nil
	case "http", "https":
		proxyURL := &url.URL{Scheme: config.ProxyMode, Host: net.JoinHostPort(config.ProxyHost, fmt.Sprint(config.ProxyPort))}
		if config.ProxyUsername != "" {
			proxyURL.User = url.UserPassword(config.ProxyUsername, config.ProxyPassword)
		}
		baseProxy := http.ProxyURL(proxyURL)
		proxyFunction = func(request *http.Request) (*url.URL, error) {
			if bypassProxy(request.URL.Hostname(), config.ProxyBypass) {
				return nil, nil
			}
			return baseProxy(request)
		}
	case "socks5":
		proxyFunction = nil
		dialContext = socks5DialContext(net.JoinHostPort(config.ProxyHost, fmt.Sprint(config.ProxyPort)), config.ProxyUsername, config.ProxyPassword, config.ProxyBypass, config.DialTimeout)
	default:
		return apperror.New(apperror.CodeValidationInvalidArgument, "HTTP Proxy Mode 无效")
	}
	transport := &http.Transport{
		Proxy:             proxyFunction,
		DialContext:       dialContext,
		ForceAttemptHTTP2: true, MaxIdleConns: config.MaxIdleConnections,
		MaxIdleConnsPerHost: 8, IdleConnTimeout: config.IdleConnTimeout,
		TLSHandshakeTimeout: config.TLSHandshakeTimeout, ResponseHeaderTimeout: config.RequestTimeout,
		ExpectContinueTimeout: time.Second,
	}
	next := &http.Client{
		Transport: transport,
		Timeout:   config.OverallTimeout,
		CheckRedirect: func(request *http.Request, previous []*http.Request) error {
			if len(previous) >= 5 {
				return apperror.New(apperror.CodeHTTPStatusFailed, "HTTP 重定向次数超过限制")
			}
			if request.URL.User != nil {
				return apperror.New(apperror.CodeValidationInvalidArgument, "HTTP 重定向目标禁止包含凭据")
			}
			if len(previous) > 0 && previous[0].URL.Scheme == "https" && request.URL.Scheme != "https" {
				return apperror.New(apperror.CodeHTTPStatusFailed, "HTTPS 请求禁止降级重定向")
			}
			return nil
		},
	}
	c.mu.Lock()
	previous := c.client
	c.client = next
	c.userAgent = config.UserAgent
	c.retries = config.Retries
	c.retryBackoff = config.RetryBackoff
	c.maximumResponseSize = config.MaximumResponseSize
	c.bandwidthLimitBytesPerSecond = config.BandwidthLimitBytesPerSecond
	c.semaphore = make(chan struct{}, config.Concurrency)
	c.mu.Unlock()
	if previous != nil {
		previous.CloseIdleConnections()
	}
	return nil
}

// Do performs a request with User-Agent, finite retry, and retryable status handling.
func (c *Client) Do(ctx context.Context, method, endpoint string, headers http.Header) (*http.Response, error) {
	if c == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "HTTP Client 不能为空")
	}
	c.mu.RLock()
	client, userAgent, retries, retryBackoff, bandwidthLimit, semaphore := c.client, c.userAgent, c.retries, c.retryBackoff, c.bandwidthLimitBytesPerSecond, c.semaphore
	c.mu.RUnlock()
	if client == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "HTTP Client 尚未初始化")
	}
	select {
	case semaphore <- struct{}{}:
		defer func() { <-semaphore }()
	case <-ctx.Done():
		return nil, apperror.Wrap(apperror.CodeProcessCancelled, "等待 HTTP 并发配额时已取消", ctx.Err())
	}
	for attempt := 0; ; attempt++ {
		request, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeValidationInvalidArgument, "创建 HTTP 请求失败", err)
		}
		request.Header.Set("User-Agent", userAgent)
		for key, values := range headers {
			for _, value := range values {
				request.Header.Add(key, value)
			}
		}
		response, err := client.Do(request)
		if err == nil && response.StatusCode >= 200 && response.StatusCode < 300 {
			if bandwidthLimit > 0 {
				response.Body = &rateLimitedReadCloser{ReadCloser: response.Body, ctx: ctx, bytesPerSecond: bandwidthLimit, startedAt: time.Now()}
			}
			return response, nil
		}
		retryable := err != nil || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		if response != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
			_ = response.Body.Close()
		}
		if !retryable || attempt >= retries || ctx.Err() != nil {
			if err != nil {
				if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return nil, apperror.Wrap(apperror.CodeProcessCancelled, "HTTP 请求已取消或超时", ctx.Err()).WithRetryable(true)
				}
				return nil, apperror.Wrap(apperror.CodeHTTPConnectionFailed, "HTTP 请求失败", err).WithRetryable(true)
			}
			return nil, apperror.New(apperror.CodeHTTPStatusFailed, fmt.Sprintf("HTTP 服务返回异常状态：%d", response.StatusCode)).WithDetails(map[string]any{
				"status": response.StatusCode, "url": endpoint,
			}).WithRetryable(retryable)
		}
		delay := time.Duration(attempt+1) * retryBackoff
		select {
		case <-ctx.Done():
			return nil, apperror.Wrap(apperror.CodeProcessCancelled, "HTTP 重试已取消", ctx.Err())
		case <-time.After(delay):
		}
	}
}

type rateLimitedReadCloser struct {
	io.ReadCloser
	ctx            context.Context
	bytesPerSecond int64
	startedAt      time.Time
	readBytes      int64
}

func (r *rateLimitedReadCloser) Read(payload []byte) (int, error) {
	read, err := r.ReadCloser.Read(payload)
	r.readBytes += int64(read)
	if read > 0 && r.bytesPerSecond > 0 {
		expected := time.Duration(float64(r.readBytes) / float64(r.bytesPerSecond) * float64(time.Second))
		if delay := expected - time.Since(r.startedAt); delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-r.ctx.Done():
				return read, r.ctx.Err()
			case <-timer.C:
			}
		}
	}
	return read, err
}

// GetJSON reads and decodes a bounded JSON response.
func (c *Client) GetJSON(ctx context.Context, endpoint string, target any) error {
	response, err := c.Do(ctx, http.MethodGet, endpoint, http.Header{"Accept": []string{"application/json"}})
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	c.mu.RLock()
	maximumResponseSize := c.maximumResponseSize
	c.mu.RUnlock()
	reader := io.LimitReader(response.Body, maximumResponseSize+1)
	payload, err := io.ReadAll(reader)
	if err != nil {
		return apperror.Wrap(apperror.CodeIOReadFailed, "读取 HTTP 响应失败", err)
	}
	if int64(len(payload)) > maximumResponseSize {
		return apperror.New(apperror.CodeHTTPResponseTooLarge, "HTTP 响应超过大小限制").WithDetails(map[string]any{
			"maximumBytes": maximumResponseSize,
		})
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return apperror.Wrap(apperror.CodeIOReadFailed, "解析 HTTP JSON 响应失败", err)
	}
	return nil
}

// GetText reads a bounded UTF-8-compatible text response.
func (c *Client) GetText(ctx context.Context, endpoint string) (string, error) {
	response, err := c.Do(ctx, http.MethodGet, endpoint, http.Header{"Accept": []string{"text/plain, application/xml, application/json"}})
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	c.mu.RLock()
	maximumResponseSize := c.maximumResponseSize
	c.mu.RUnlock()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseSize+1))
	if err != nil {
		return "", apperror.Wrap(apperror.CodeIOReadFailed, "读取 HTTP 文本响应失败", err)
	}
	if int64(len(payload)) > maximumResponseSize {
		return "", apperror.New(apperror.CodeHTTPResponseTooLarge, "HTTP 文本响应超过大小限制")
	}
	return string(payload), nil
}

func bypassProxy(host string, patterns []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "*" || pattern == host || strings.HasPrefix(pattern, ".") && strings.HasSuffix(host, pattern) || strings.HasPrefix(pattern, "*.") && strings.HasSuffix(host, pattern[1:]) {
			return true
		}
	}
	return false
}
