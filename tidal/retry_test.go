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
	orig := sleep
	sleep = func(ctx context.Context, d time.Duration) error { slept = append(slept, d); return nil }
	t.Cleanup(func() { sleep = orig })
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
	// backoff 2s, then Retry-After 10s beats 4s, then 8s; then politeDelay on success.
	want := []time.Duration{2 * time.Second, 10 * time.Second, 8 * time.Second, politeDelay}
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
