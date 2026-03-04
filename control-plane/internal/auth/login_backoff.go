package auth

import (
	"strings"
	"sync"
	"time"
)

type LoginBackoffConfig struct {
	Enabled   bool
	Threshold int
	BaseDelay time.Duration
	MaxDelay  time.Duration
	Window    time.Duration
}

type LoginBackoff struct {
	mu        sync.Mutex
	threshold int
	baseDelay time.Duration
	maxDelay  time.Duration
	window    time.Duration
	entries   map[string]*loginBackoffEntry
}

type loginBackoffEntry struct {
	failures     int
	lastFailure  time.Time
	blockedUntil time.Time
}

func NewLoginBackoff(cfg LoginBackoffConfig) *LoginBackoff {
	if !cfg.Enabled {
		return nil
	}
	threshold := cfg.Threshold
	if threshold < 1 {
		threshold = 1
	}
	base := cfg.BaseDelay
	if base <= 0 {
		base = 2 * time.Second
	}
	maxDelay := cfg.MaxDelay
	if maxDelay <= 0 {
		maxDelay = 5 * time.Minute
	}
	if maxDelay < base {
		maxDelay = base
	}
	window := cfg.Window
	if window <= 0 {
		window = 15 * time.Minute
	}

	return &LoginBackoff{
		threshold: threshold,
		baseDelay: base,
		maxDelay:  maxDelay,
		window:    window,
		entries:   map[string]*loginBackoffEntry{},
	}
}

func (b *LoginBackoff) Check(identity string, now time.Time) (bool, time.Duration) {
	if b == nil {
		return false, 0
	}
	key := normalizeIdentity(identity)
	b.mu.Lock()
	defer b.mu.Unlock()

	entry := b.entries[key]
	if entry == nil {
		return false, 0
	}
	if b.isExpired(entry, now) {
		delete(b.entries, key)
		return false, 0
	}
	if now.Before(entry.blockedUntil) {
		return true, entry.blockedUntil.Sub(now)
	}
	return false, 0
}

func (b *LoginBackoff) RegisterFailure(identity string, now time.Time) time.Duration {
	if b == nil {
		return 0
	}
	key := normalizeIdentity(identity)
	b.mu.Lock()
	defer b.mu.Unlock()

	entry := b.entries[key]
	if entry == nil || b.isExpired(entry, now) {
		entry = &loginBackoffEntry{}
		b.entries[key] = entry
	}
	entry.failures++
	entry.lastFailure = now

	if entry.failures < b.threshold {
		b.prune(now)
		return 0
	}
	steps := entry.failures - b.threshold
	delay := b.baseDelay
	for i := 0; i < steps; i++ {
		if delay >= b.maxDelay/2 {
			delay = b.maxDelay
			break
		}
		delay *= 2
	}
	if delay > b.maxDelay {
		delay = b.maxDelay
	}
	entry.blockedUntil = now.Add(delay)
	b.prune(now)
	return delay
}

func (b *LoginBackoff) RegisterSuccess(identity string) {
	if b == nil {
		return
	}
	key := normalizeIdentity(identity)
	b.mu.Lock()
	delete(b.entries, key)
	b.mu.Unlock()
}

func (b *LoginBackoff) isExpired(entry *loginBackoffEntry, now time.Time) bool {
	if entry == nil {
		return true
	}
	if entry.lastFailure.IsZero() {
		return true
	}
	return now.After(entry.lastFailure.Add(b.window)) && now.After(entry.blockedUntil)
}

func (b *LoginBackoff) prune(now time.Time) {
	if len(b.entries) < 1024 {
		return
	}
	for key, entry := range b.entries {
		if b.isExpired(entry, now) {
			delete(b.entries, key)
		}
	}
}

func normalizeIdentity(identity string) string {
	identity = strings.TrimSpace(strings.ToLower(identity))
	if identity == "" {
		return "_"
	}
	return identity
}
