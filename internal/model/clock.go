package model

import "time"

// Clock 为服务与仓储提供可替换的 UTC 时间源。
type Clock interface {
	Now() time.Time
}

// SystemClock 返回归一化为 UTC 的系统当前时间。
type SystemClock struct{}

// Now 返回当前 UTC 时间。
func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}
