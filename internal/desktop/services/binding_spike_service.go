// Package services 对外暴露面向桌面的稳定 Wails 服务。
package services

import (
	"context"
	"errors"
	"time"
)

// BindingSpikeRequest 用于验证 Wails 对标量、切片与映射字段的绑定生成。
type BindingSpikeRequest struct {
	Name     string            `json:"name"`
	Tags     []string          `json:"tags"`
	Metadata map[string]string `json:"metadata"`
}

// BindingSpikeResponse 是绑定基线校验中 BindingSpikeService 的返回值。
type BindingSpikeResponse struct {
	Accepted bool                `json:"accepted"`
	Echo     BindingSpikeRequest `json:"echo"`
}

// BindingSpikeService 校验 Wails DTO 与上下文契约的基线行为。
type BindingSpikeService struct{}

// Echo 原样返回传入的 DTO,除非 Wails 请求上下文已取消。
func (s *BindingSpikeService) Echo(ctx context.Context, request BindingSpikeRequest) (BindingSpikeResponse, error) {
	if err := ctx.Err(); err != nil {
		return BindingSpikeResponse{}, err
	}

	return BindingSpikeResponse{Accepted: true, Echo: request}, nil
}

// Wait 阻塞指定时长,Wails 上下文取消时提前返回。
func (s *BindingSpikeService) Wait(ctx context.Context, milliseconds int) (string, error) {
	if milliseconds < 0 {
		return "", errors.New("milliseconds must not be negative")
	}

	timer := time.NewTimer(time.Duration(milliseconds) * time.Millisecond)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-timer.C:
		return "completed", nil
	}
}
