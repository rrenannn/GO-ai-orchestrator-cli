// Package clock provides the system clock adapter.
package clock

import "time"

// System reads the wall clock.
type System struct{}

// New returns the system clock.
func New() System { return System{} }

// Now returns the current time.
func (System) Now() time.Time { return time.Now() }
