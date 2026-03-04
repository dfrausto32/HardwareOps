package auth

import (
	"testing"
	"time"
)

func TestLoginBackoffProgressiveAndReset(t *testing.T) {
	now := time.Now().UTC()
	backoff := NewLoginBackoff(LoginBackoffConfig{
		Enabled:   true,
		Threshold: 2,
		BaseDelay: 100 * time.Millisecond,
		MaxDelay:  800 * time.Millisecond,
		Window:    10 * time.Minute,
	})

	if blocked, _ := backoff.Check("admin@example.com", now); blocked {
		t.Fatalf("unexpected blocked before failures")
	}

	if delay := backoff.RegisterFailure("admin@example.com", now); delay != 0 {
		t.Fatalf("expected no delay on first failure, got %s", delay)
	}
	if blocked, _ := backoff.Check("admin@example.com", now); blocked {
		t.Fatalf("unexpected blocked after first failure")
	}

	delay2 := backoff.RegisterFailure("admin@example.com", now)
	if delay2 != 100*time.Millisecond {
		t.Fatalf("unexpected second failure delay: %s", delay2)
	}
	if blocked, retry := backoff.Check("admin@example.com", now.Add(10*time.Millisecond)); !blocked || retry <= 0 {
		t.Fatalf("expected blocked after second failure")
	}

	delay3 := backoff.RegisterFailure("admin@example.com", now)
	if delay3 != 200*time.Millisecond {
		t.Fatalf("unexpected third failure delay: %s", delay3)
	}

	backoff.RegisterSuccess("admin@example.com")
	if blocked, _ := backoff.Check("admin@example.com", now); blocked {
		t.Fatalf("expected reset after success")
	}
}

func TestLoginBackoffCapsDelay(t *testing.T) {
	now := time.Now().UTC()
	backoff := NewLoginBackoff(LoginBackoffConfig{
		Enabled:   true,
		Threshold: 1,
		BaseDelay: 100 * time.Millisecond,
		MaxDelay:  400 * time.Millisecond,
		Window:    10 * time.Minute,
	})

	delay1 := backoff.RegisterFailure("admin@example.com", now)
	delay2 := backoff.RegisterFailure("admin@example.com", now)
	delay3 := backoff.RegisterFailure("admin@example.com", now)
	delay4 := backoff.RegisterFailure("admin@example.com", now)

	if delay1 != 100*time.Millisecond || delay2 != 200*time.Millisecond || delay3 != 400*time.Millisecond || delay4 != 400*time.Millisecond {
		t.Fatalf("unexpected capped delays: %s %s %s %s", delay1, delay2, delay3, delay4)
	}
}
