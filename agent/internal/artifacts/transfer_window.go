package artifacts

import (
	"fmt"
	"strings"
	"time"
)

// TransferWindow restricts when bulk downloads may start, e.g. "22:00-05:00"
// for an overnight window on a metered or contested link. A nil window is
// always open. Times are interpreted in the supplied clock's location
// (local time on the device).
type TransferWindow struct {
	start, end int // minutes since midnight
}

// ParseTransferWindow parses "HH:MM-HH:MM". Empty input means no restriction.
// start == end is rejected (ambiguous); end < start wraps past midnight.
func ParseTransferWindow(s string) (*TransferWindow, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("transfer window %q: want HH:MM-HH:MM", s)
	}
	a, err := parseClock(parts[0])
	if err != nil {
		return nil, err
	}
	b, err := parseClock(parts[1])
	if err != nil {
		return nil, err
	}
	if a == b {
		return nil, fmt.Errorf("transfer window %q: start equals end", s)
	}
	return &TransferWindow{start: a, end: b}, nil
}

func parseClock(s string) (int, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("transfer window time %q: %w", s, err)
	}
	return t.Hour()*60 + t.Minute(), nil
}

// Open reports whether now falls inside the window.
func (w *TransferWindow) Open(now time.Time) bool {
	if w == nil {
		return true
	}
	m := now.Hour()*60 + now.Minute()
	if w.start < w.end {
		return m >= w.start && m < w.end
	}
	return m >= w.start || m < w.end // wraps midnight
}
