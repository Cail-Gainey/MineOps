package apperror

import (
	"errors"
	"fmt"
)

// Error contains a stable code, safe message/details, retryability, and a diagnostic cause.
type Error struct {
	Code      Code
	Message   string
	Details   map[string]any
	Retryable bool
	Cause     error
}

// DTO is the desktop-safe representation of an application error.
type DTO struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	Retryable bool           `json:"retryable"`
}

// New creates an application error without an underlying cause.
func New(code Code, message string) *Error {
	if !code.Valid() {
		code = CodeInternal
	}
	return &Error{Code: code, Message: message}
}

// Wrap creates an application error that retains the diagnostic cause.
func Wrap(code Code, message string, cause error) *Error {
	errorValue := New(code, message)
	errorValue.Cause = cause
	return errorValue
}

// Error returns a log-friendly error string while keeping details structured.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap exposes the diagnostic cause for errors.Is and errors.As.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// WithDetails attaches desktop-safe structured details.
func (e *Error) WithDetails(details map[string]any) *Error {
	if e == nil {
		return nil
	}
	e.Details = cloneDetails(details)
	return e
}

// WithRetryable marks whether retrying the same intent may succeed.
func (e *Error) WithRetryable(retryable bool) *Error {
	if e == nil {
		return nil
	}
	e.Retryable = retryable
	return e
}

// ToDTO converts any error into a stable desktop-safe DTO without exposing its cause.
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

// FromPanic converts a recovered panic value into a stable internal error.
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
