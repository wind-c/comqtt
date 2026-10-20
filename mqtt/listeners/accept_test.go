// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2023 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package listeners

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var (
	errTransient = errors.New("simulated EMFILE")
	errClosed    = errors.New("use of closed network connection")
)

func discardLog() *slog.Logger { return slog.New(slog.DiscardHandler) }

// scriptedAccept fails with errs[i] on the ith call and then succeeds.
type scriptedAccept struct {
	errs []error
	val  string
	n    int
}

func (s *scriptedAccept) fn() (string, error) {
	i := s.n
	s.n++
	if i < len(s.errs) {
		return "", s.errs[i]
	}
	return s.val, nil
}

func TestAcceptRetryReturnsOnFirstSuccess(t *testing.T) {
	s := &scriptedAccept{val: "conn"}
	accept := s.fn

	v, err := acceptRetry(accept, nil, make(chan struct{}), discardLog())
	require.NoError(t, err)
	require.Equal(t, "conn", v)
	require.Equal(t, 1, s.n)
}

// The core guarantee: a transient Accept failure must not be fatal.
func TestAcceptRetryRetriesTransientFailures(t *testing.T) {
	s := &scriptedAccept{errs: []error{errTransient, errTransient, errTransient}, val: "conn"}
	accept := s.fn

	v, err := acceptRetry(accept, nil, make(chan struct{}), discardLog())
	require.NoError(t, err)
	require.Equal(t, "conn", v)
	require.Equal(t, 4, s.n, "should have retried through all three failures")
}

// Asserting a floor is deterministic; a ceiling would be flaky on a loaded
// machine. This is what proves the retry actually backs off instead of
// spinning: three failures sleep 5+10+20ms before the fourth attempt.
func TestAcceptRetryBacksOffBetweenAttempts(t *testing.T) {
	s := &scriptedAccept{errs: []error{errTransient, errTransient, errTransient}}
	accept := s.fn

	start := time.Now()
	_, err := acceptRetry(accept, nil, make(chan struct{}), discardLog())
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.GreaterOrEqual(t, elapsed, 35*time.Millisecond,
		"retries must back off, not busy-spin")
}

// Close() is the only reliable stop signal: the error a closed listener
// produces differs by listener type, so done is checked instead.

// repeatErr builds a script of n identical failures. make([]error, n) would
// produce n nil errors, which the scripted accept treats as success.
func repeatErr(err error, n int) []error {
	s := make([]error, n)
	for i := range s {
		s[i] = err
	}
	return s
}

func TestAcceptRetryStopsWhenDoneIsClosed(t *testing.T) {
	s := &scriptedAccept{errs: repeatErr(errTransient, 10)}
	accept := s.fn
	done := make(chan struct{})
	close(done)

	start := time.Now()
	_, err := acceptRetry(accept, nil, done, discardLog())
	require.ErrorIs(t, err, errListenerStopped)
	require.Equal(t, 1, s.n, "must give up on the first failure")
	require.Less(t, time.Since(start), 35*time.Millisecond, "must not sleep once stopped")
}

// Safety net: a listener closed without going through Close must not be
// retried forever.
func TestAcceptRetryStopsWhenListenerClosedOutOfBand(t *testing.T) {
	s := &scriptedAccept{errs: repeatErr(errClosed, 10)}
	accept := s.fn
	done := make(chan struct{})

	stopped := func(e error) bool { return errors.Is(e, errClosed) }
	_, err := acceptRetry(accept, stopped, done, discardLog())
	require.ErrorIs(t, err, errClosed)
	require.Equal(t, 1, s.n, "must not retry an out-of-band close")
}

// The backoff state must be per call, not per listener: one bad patch must not
// permanently slow down every later connection attempt.
func TestAcceptRetryBackoffDoesNotLeakBetweenCalls(t *testing.T) {
	done := make(chan struct{})
	log := discardLog()
	failures := []error{errTransient, errTransient, errTransient}

	first := &scriptedAccept{errs: failures}
	firstAccept := first.fn
	start := time.Now()
	_, err := acceptRetry(firstAccept, nil, done, log)
	require.NoError(t, err)
	require.GreaterOrEqual(t, time.Since(start), 35*time.Millisecond)

	// A second call with the same number of failures must cost the same, not
	// continue from where the first left off (which would be 40+80+160ms).
	second := &scriptedAccept{errs: failures}
	secondAccept := second.fn
	start = time.Now()
	_, err = acceptRetry(secondAccept, nil, done, log)
	require.NoError(t, err)
	elapsed := time.Since(start)
	require.GreaterOrEqual(t, elapsed, 35*time.Millisecond)
	require.Less(t, elapsed, 150*time.Millisecond, "backoff leaked from the previous call")
}

// A connection that did arrive must be handed back even if the listener is
// closing, so the caller can close it instead of leaking the fd.
func TestAcceptRetryReturnsValueEvenWhenDoneIsClosed(t *testing.T) {
	s := &scriptedAccept{val: "conn"}
	accept := s.fn
	done := make(chan struct{})
	close(done)

	v, err := acceptRetry(accept, nil, done, discardLog())
	require.NoError(t, err)
	require.Equal(t, "conn", v)
}

// The happy path must stay allocation free; this runs on every connection.
func TestAcceptRetryHappyPathDoesNotAllocate(t *testing.T) {
	log := discardLog()
	s := &scriptedAccept{val: "conn"}
	accept := s.fn
	done := make(chan struct{})

	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = acceptRetry(accept, nil, done, log)
	})
	require.Zero(t, allocs, "accept runs once per connection and must not allocate")
}