package service

import "time"

// Clock provides time operations. This interface enables deterministic testing.
type Clock interface {
	Now() time.Time
}

// RealClock implements Clock using the actual system time.
type RealClock struct{}

// Now returns the current time.
func (RealClock) Now() time.Time {
	return time.Now()
}

// TestClock implements Clock with a fixed time for testing.
type TestClock struct {
	FixedTime time.Time
}

// Now returns the fixed time.
func (t TestClock) Now() time.Time {
	return t.FixedTime
}
