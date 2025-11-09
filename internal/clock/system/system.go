// Package system provides a real clock implementation.
package system

import "time"

// Clock implements crawler.Clock using time.Now.
type Clock struct{}

// New creates a new Clock.
func New() *Clock {
	return &Clock{}
}

// Now returns the current time.
func (Clock) Now() time.Time {
	return time.Now().UTC()
}
