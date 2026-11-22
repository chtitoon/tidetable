package tidal

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func stubSleep(t *testing.T) *[]time.Duration {
	t.Helper()
	var slept []time.Duration
	orig, origInterval := sleep, requestInterval
	sleep = func(ctx context.Context, d time.Duration) error { slept = append(slept, d); return nil }
	requestInterval = 0 // pacing has its own test
	t.Cleanup(func() { sleep, requestInterval = orig, origInterval })
	return &slept
}

func TestWithRetryBacksOffOn429And5xx(t *testing.T) {
	slept := stubSleep(t)
	errs := []error{
		&APIError{StatusCode: http.StatusTooManyRequests},
		&APIError{StatusCode: http.StatusTooManyRequests, RetryAfter: 10 * time.Second},
		&APIError{StatusCode: http.StatusBadGateway},
	}
	calls := 0
	got, err := withRetry(context.Background(), &Service{}, func() (int, error) {
		calls++
		if calls <= len(errs) {
			return 0, errs[calls-1]
		}
		return 42, nil
	})
	if err != nil || got != 42 || calls != 4 {
		t.Fatalf("got %d, %v after %d calls", got, err, calls)
	}
	// backoff 2s, then Retry-After 10s beats 4s, then 8s.
	want := []time.Duration{2 * time.Second, 10 * time.Second, 8 * time.Second}
	if len(*slept) != len(want) {
		t.Fatalf("slept %v, want %v", *slept, want)
	}
	for i := range want {
		if (*slept)[i] != want[i] {
			t.Errorf("sleep[%d] = %v, want %v", i, (*slept)[i], want[i])
		}
	}
}

func TestWithRetryGivesUp(t *testing.T) {
	stubSleep(t)
	calls := 0
	_, err := withRetry(context.Background(), &Service{}, func() (int, error) {
		calls++
		return 0, &APIError{StatusCode: http.StatusServiceUnavailable}
	})
	if err == nil || calls != retryMaxAttempts {
		t.Fatalf("err=%v calls=%d, want error after %d calls", err, calls, retryMaxAttempts)
	}
}

func TestWithRetryDoesNotRetryClientErrors(t *testing.T) {
	stubSleep(t)
	for _, err := range []error{&APIError{StatusCode: http.StatusNotFound}, errors.New("network down")} {
		calls := 0
		_, got := withRetry(context.Background(), &Service{}, func() (int, error) { calls++; return 0, err })
		if got == nil || calls != 1 {
			t.Errorf("%v: calls=%d err=%v, want one call and the error", err, calls, got)
		}
	}
}

func TestPaceSpacesConcurrentCallers(t *testing.T) {
	orig := requestInterval
	requestInterval = 20 * time.Millisecond
	t.Cleanup(func() { requestInterval = orig })

	s := &Service{}
	start := time.Now()
	done := make(chan struct{})
	for range 5 {
		go func() { _ = s.pace(context.Background()); done <- struct{}{} }()
	}
	for range 5 {
		<-done
	}
	// 5 callers: the first starts at once, the last after 4 intervals.
	if got := time.Since(start); got < 4*requestInterval {
		t.Fatalf("5 paced calls took %v, want >= %v", got, 4*requestInterval)
	}
}

func TestWithRetryCountsThrottling(t *testing.T) {
	stubSleep(t)
	s := &Service{}
	calls := 0
	_, err := withRetry(context.Background(), s, func() (int, error) {
		if calls++; calls <= 2 {
			return 0, &APIError{StatusCode: http.StatusTooManyRequests}
		}
		return 1, nil
	})
	if err != nil || s.Throttled() != 2 {
		t.Fatalf("err=%v throttled=%d, want nil and 2", err, s.Throttled())
	}
}
