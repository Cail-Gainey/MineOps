package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
)

// ClientErrorReport contains a redacted frontend exception report.
type ClientErrorReport struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	Stack   string `json:"stack"`
	Source  string `json:"source"`
	Route   string `json:"route"`
}

// ErrorService receives frontend exceptions at the desktop logging boundary.
type ErrorService struct {
	logger *applog.Logger
}

// NewErrorService creates the frontend exception reporting facade.
func NewErrorService(logger *applog.Logger) *ErrorService {
	return &ErrorService{logger: logger}
}

// ReportClientError records one frontend exception without returning its stack to the UI.
func (s *ErrorService) ReportClientError(ctx context.Context, report ClientErrorReport) (result ActionResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "ErrorService.ReportClientError", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	s.logger.Error(ctx, "前端未处理错误", apperror.New(apperror.CodeInternal, report.Message), applog.Fields{
		"name": report.Name, "source": report.Source, "route": report.Route, "stack": report.Stack,
	})
	return ActionResult{}
}
