package model

import "time"

// Clock provides replaceable UTC time for services and repositories.
type Clock interface {
	Now() time.Time
}

// SystemClock returns the current system time normalized to UTC.
type SystemClock struct{}

// Now returns the current UTC time.
func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}
