// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2023 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co, wind

package listeners

import (
	"errors"
	"log/slog"
	"net"
	"time"
)

// errListenerStopped is returned when a listener was closed deliberately, as
// opposed to failing in a way that is worth retrying.
var errListenerStopped = errors.New("listener stopped")

const (
	// maxAcceptBackoff caps the delay between attempts to accept a connection.
	maxAcceptBackoff = time.Second
	// acceptLogEvery is how many failed attempts pass between log lines. A
	// listener that stays broken would otherwise log once a second forever.
	acceptLogEvery = 16
)

// isNetClosed reports whether err is the error a net.Listener returns once it
// has been closed. net.Error.Temporary is deliberately not used: it is
// deprecated and only covers EINTR, EMFILE and timeouts on Windows.
func isNetClosed(err error) bool {
	return errors.Is(err, net.ErrClosed)
}

// acceptRetry calls accept until it yields a value, retrying failures with a
// capped backoff. It returns an error only when the listener has been closed,
// so a caller can treat any return as fatal. The policy lives here so it can
// be adopted or removed in one place, and is generic so that TCP and Unix can
// share it.
func acceptRetry[T any](accept func() (T, error), stopped func(error) bool, done <-chan struct{}, log *slog.Logger) (T, error) {
	var zero T
	var wait time.Duration
	var failures int

	for {
		v, err := accept()
		if err == nil {
			if failures > 0 {
				log.Info("accept recovered", "after_failures", failures)
			}
			// the backoff is per call, so a new attempt starts optimistic
			return v, nil
		}

		// done is closed by Close and is the only reliable stop signal
		select {
		case <-done:
			return zero, errListenerStopped
		default:
		}

		// safety net for a listener closed without going through Close
		if stopped != nil && stopped(err) {
			return zero, err
		}

		if wait == 0 {
			wait = 5 * time.Millisecond
		} else if wait < maxAcceptBackoff {
			wait *= 2
		}
		failures++

		if failures == 1 || failures%acceptLogEvery == 0 {
			log.Warn("accept failed, retrying", "error", err,
				"attempt", failures, "retry_in", wait)
		}

		select {
		case <-done:
			return zero, errListenerStopped
		case <-time.After(wait):
		}
	}
}