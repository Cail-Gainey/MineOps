package apperror

import (
	"errors"
	"fmt"
)

// Error 承载稳定错误码、安全的信息与细节、可重试标记以及诊断用的根因。
type Error struct {
	Code      Code
	Message   string
	Details   map[string]any
	Retryable bool
	Cause     error
}

// DTO 是应用错误在桌面侧的安全表示。
type DTO struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	Retryable bool           `json:"retryable"`
}

// New 创建一个不带底层根因的应用错误。
func New(code Code, message string) *Error {
	if !code.Valid() {
		code = CodeInternal
	}
	return &Error{Code: code, Message: message}
}

// Wrap 创建一个保留诊断根因的应用错误。
func Wrap(code Code, message string, cause error) *Error {
	errorValue := New(code, message)
	errorValue.Cause = cause
	return errorValue
}

// Error 返回便于记录日志的错误字符串,同时保持细节的结构化。
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap 向 errors.Is 与 errors.As 暴露诊断根因。
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// WithDetails 附加桌面侧安全的结构化细节。
func (e *Error) WithDetails(details map[string]any) *Error {
	if e == nil {
		return nil
	}
	e.Details = cloneDetails(details)
	return e
}

// WithRetryable 标记重试同一意图是否可能成功。
func (e *Error) WithRetryable(retryable bool) *Error {
	if e == nil {
		return nil
	}
	e.Retryable = retryable
	return e
}

// ToDTO 把任意错误转换成稳定的桌面安全 DTO,不暴露其根因。
func ToDTO(err error) DTO {
	var applicationError *Error
	if errors.As(err, &applicationError) {
		return DTO{
			Code:      applicationError.Code.String(),
			Message:   applicationError.Message,
			Details:   cloneDetails(applicationError.Details),
			Retryable: applicationError.Retryable,
		}
	}
	return DTO{Code: CodeInternal.String(), Message: "发生未预期错误", Retryable: false}
}

// FromPanic 把 recover 到的 panic 值转换成稳定的内部错误。
func FromPanic(value any) *Error {
	return Wrap(CodeInternal, "发生未预期错误", fmt.Errorf("panic: %v", value))
}

func cloneDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(details))
	for key, value := range details {
		cloned[key] = value
	}
	return cloned
}
