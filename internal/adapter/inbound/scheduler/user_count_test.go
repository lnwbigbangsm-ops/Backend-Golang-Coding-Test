package scheduler

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeCounter struct {
	calls atomic.Int32
	err   error
}

func (f *fakeCounter) Count(context.Context) (int64, error) {
	f.calls.Add(1)
	return 7, f.err
}

// syncBuffer makes bytes.Buffer safe to share between the reporter goroutine
// and the test.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func runReporter(t *testing.T, counter Counter, ticks int32, calls func() int32) string {
	t.Helper()
	var logs syncBuffer
	r := NewUserCountReporter(counter, 5*time.Millisecond, slog.New(slog.NewTextHandler(&logs, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()

	deadline := time.After(2 * time.Second)
	for calls() < ticks {
		select {
		case <-deadline:
			t.Fatalf("reporter ran %d times, want >= %d", calls(), ticks)
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reporter did not stop after context cancellation")
	}
	return logs.String()
}

func TestUserCountReporter_LogsCountAndStops(t *testing.T) {
	c := &fakeCounter{}
	out := runReporter(t, c, 3, func() int32 { return c.calls.Load() })

	if !strings.Contains(out, "total users") || !strings.Contains(out, "count=7") {
		t.Errorf("unexpected log output:\n%s", out)
	}
	if !strings.Contains(out, "user count reporter stopped") {
		t.Errorf("missing stop log:\n%s", out)
	}
}

func TestUserCountReporter_LogsErrors(t *testing.T) {
	c := &fakeCounter{err: errors.New("db down")}
	out := runReporter(t, c, 1, func() int32 { return c.calls.Load() })

	if !strings.Contains(out, "count users failed") || !strings.Contains(out, "db down") {
		t.Errorf("unexpected log output:\n%s", out)
	}
}
