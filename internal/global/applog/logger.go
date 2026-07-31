package applog

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"sync/atomic"
)

// Fields 承载附加到日志记录上的结构化取值。
type Fields map[string]any

// ContextFields 承载 MineOps 标准的关联标识。
type ContextFields struct {
	RequestID   string
	OperationID string
	ServerID    string
	AgentID     string
	Component   string
}

type contextFieldsKey struct{}

// Logger 是 MineOps 各服务唯一的结构化日志边界。
type Logger struct {
	logger *slog.Logger
	level  *slog.LevelVar
}

// New 创建带敏感字段脱敏与运行期等级控制的 JSON 日志器。
func New(output io.Writer, level slog.Level) *Logger {
	levelVariable := &slog.LevelVar{}
	levelVariable.Set(level)
	handler := &redactingHandler{next: slog.NewJSONHandler(output, &slog.HandlerOptions{Level: levelVariable})}
	return &Logger{logger: slog.New(handler), level: levelVariable}
}

// WithContextFields 返回携带标准关联标识的上下文。
func WithContextFields(ctx context.Context, fields ContextFields) context.Context {
	return context.WithValue(ctx, contextFieldsKey{}, fields)
}

// SetLevel 修改运行期的最低日志等级。
func (l *Logger) SetLevel(level slog.Level) {
	if l != nil {
		l.level.Set(level)
	}
}

// Debug 写入一条结构化 debug 记录。
func (l *Logger) Debug(ctx context.Context, message string, fields Fields) {
	l.log(ctx, slog.LevelDebug, message, fields)
}

// Info 写入一条结构化 info 记录。
func (l *Logger) Info(ctx context.Context, message string, fields Fields) {
	l.log(ctx, slog.LevelInfo, message, fields)
}

// Warn 写入一条结构化 warn 记录。
func (l *Logger) Warn(ctx context.Context, message string, fields Fields) {
	l.log(ctx, slog.LevelWarn, message, fields)
}

// Error 写入一条结构化 error 记录,并以脱敏形式保留诊断根因。
func (l *Logger) Error(ctx context.Context, message string, err error, fields Fields) {
	cloned := make(Fields, len(fields)+1)
	for key, value := range fields {
		cloned[key] = value
	}
	if err != nil {
		cloned["error"] = err.Error()
	}
	l.log(ctx, slog.LevelError, message, cloned)
}

func (l *Logger) log(ctx context.Context, level slog.Level, message string, fields Fields) {
	if l == nil || l.logger == nil {
		return
	}
	attributes := make([]any, 0, len(fields)*2+10)
	if correlation, ok := ctx.Value(contextFieldsKey{}).(ContextFields); ok {
		attributes = appendCorrelation(attributes, correlation)
	}
	for key, value := range fields {
		attributes = append(attributes, key, value)
	}
	l.logger.Log(ctx, level, message, attributes...)
}

func appendCorrelation(attributes []any, fields ContextFields) []any {
	values := []struct {
		key   string
		value string
	}{
		{"request_id", fields.RequestID}, {"operation_id", fields.OperationID}, {"server_id", fields.ServerID},
		{"agent_id", fields.AgentID}, {"component", fields.Component},
	}
	for _, value := range values {
		if value.value != "" {
			attributes = append(attributes, value.key, value.value)
		}
	}
	return attributes
}

type redactingHandler struct {
	next slog.Handler
}

// Enabled 判断该等级的日志是否需要处理。
func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle 对日志记录执行敏感信息脱敏后交给下游 Handler。
func (h *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	redacted := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attribute slog.Attr) bool {
		redacted.AddAttrs(redactAttribute(attribute))
		return true
	})
	return h.next.Handle(ctx, redacted)
}

// WithAttrs 返回附带固定属性且保持脱敏行为的 Handler。
func (h *redactingHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attributes))
	for index, attribute := range attributes {
		redacted[index] = redactAttribute(attribute)
	}
	return &redactingHandler{next: h.next.WithAttrs(redacted)}
}

// WithGroup 返回带分组前缀且保持脱敏行为的 Handler。
func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{next: h.next.WithGroup(name)}
}

func redactAttribute(attribute slog.Attr) slog.Attr {
	attribute.Value = attribute.Value.Resolve()
	if sensitiveKey(attribute.Key) {
		return slog.String(attribute.Key, "[REDACTED]")
	}
	if attribute.Value.Kind() == slog.KindGroup {
		children := attribute.Value.Group()
		for index, child := range children {
			children[index] = redactAttribute(child)
		}
		return slog.Group(attribute.Key, attrsToAny(children)...)
	}
	if attribute.Value.Kind() == slog.KindAny {
		return slog.Any(attribute.Key, redactAny(attribute.Value.Any(), 0))
	}
	return attribute
}

func redactAny(value any, depth int) any {
	if value == nil || depth >= 8 {
		return value
	}
	reflected := reflect.ValueOf(value)
	for reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface {
		if reflected.IsNil() {
			return nil
		}
		reflected = reflected.Elem()
	}
	switch reflected.Kind() {
	case reflect.Map:
		if reflected.Type().Key().Kind() != reflect.String {
			return value
		}
		redacted := make(map[string]any, reflected.Len())
		iterator := reflected.MapRange()
		for iterator.Next() {
			key := iterator.Key().String()
			if sensitiveKey(key) {
				redacted[key] = "[REDACTED]"
			} else {
				redacted[key] = redactAny(iterator.Value().Interface(), depth+1)
			}
		}
		return redacted
	case reflect.Slice, reflect.Array:
		redacted := make([]any, reflected.Len())
		for index := range reflected.Len() {
			redacted[index] = redactAny(reflected.Index(index).Interface(), depth+1)
		}
		return redacted
	case reflect.Struct:
		if reflected.Type().PkgPath() == "time" {
			return value
		}
		redacted := make(map[string]any, reflected.NumField())
		for index := range reflected.NumField() {
			fieldType := reflected.Type().Field(index)
			fieldValue := reflected.Field(index)
			if !fieldType.IsExported() || !fieldValue.CanInterface() {
				continue
			}
			key := fieldType.Name
			if jsonName := strings.Split(fieldType.Tag.Get("json"), ",")[0]; jsonName != "" && jsonName != "-" {
				key = jsonName
			}
			if sensitiveKey(key) {
				redacted[key] = "[REDACTED]"
			} else {
				redacted[key] = redactAny(fieldValue.Interface(), depth+1)
			}
		}
		return redacted
	default:
		return value
	}
}

func sensitiveKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "", ".", "").Replace(strings.ToLower(key))
	for _, fragment := range [...]string{"password", "privatekey", "passphrase", "token", "secret", "authorization", "credential"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func attrsToAny(attributes []slog.Attr) []any {
	values := make([]any, len(attributes))
	for index, attribute := range attributes {
		values[index] = attribute
	}
	return values
}

var defaultLogger atomic.Pointer[Logger]

// SetDefault 在 bootstrap 选定受控输出后安装进程级日志器。
func SetDefault(logger *Logger) {
	defaultLogger.Store(logger)
}

// Default 返回已安装的进程日志器;bootstrap 完成前返回丢弃型日志器。
func Default() *Logger {
	if logger := defaultLogger.Load(); logger != nil {
		return logger
	}
	return New(io.Discard, slog.LevelInfo)
}
