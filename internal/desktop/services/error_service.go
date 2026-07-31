package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
)

// ClientErrorReport 承载一份脱敏的前端异常报告。
type ClientErrorReport struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	Stack   string `json:"stack"`
	Source  string `json:"source"`
	Route   string `json:"route"`
}

// ErrorService 在桌面日志边界接收前端异常。
type ErrorService struct {
	logger *applog.Logger
}

// NewErrorService 创建前端异常上报门面。
func NewErrorService(logger *applog.Logger) *ErrorService {
	return &ErrorService{logger: logger}
}

// ReportClientError 记录一条前端异常,不把堆栈回传给界面。
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
