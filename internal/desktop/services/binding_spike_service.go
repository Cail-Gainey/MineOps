// Package services exposes stable desktop-facing Wails services.
package services

import (
	"context"
	"errors"
	"time"
)

// BindingSpikeRequest exercises Wails generation for scalar, slice, and map fields.
type BindingSpikeRequest struct {
	Name     string            `json:"name"`
	Tags     []string          `json:"tags"`
	Metadata map[string]string `json:"metadata"`
}

// BindingSpikeResponse is returned by BindingSpikeService during the binding baseline check.
type BindingSpikeResponse struct {
	Accepted bool                `json:"accepted"`
	Echo     BindingSpikeRequest `json:"echo"`
}

// BindingSpikeService validates the baseline Wails DTO and context contract.
type BindingSpikeService struct{}

// Echo returns the supplied DTO unless the Wails request context has been cancelled.
func (s *BindingSpikeService) Echo(ctx context.Context, request BindingSpikeRequest) (BindingSpikeResponse, error) {
	if err := ctx.Err(); err != nil {
		return BindingSpikeResponse{}, err
	}

	return BindingSpikeResponse{Accepted: true, Echo: request}, nil
}

// Wait blocks for the requested duration or returns early when the Wails context is cancelled.
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
