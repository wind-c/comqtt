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
// has been closed.
//
// net.Error.Temporary is deliberately not used: it is deprecated ("Temporary
// errors are not well-defined... Do not use this method") and its
// implementation only covers EINTR, EMFILE and timeouts on Windows, missing
// ENFILE and ECONNABORTED.
func isNetClosed(err error) bool {
	return errors.Is(err, net.ErrClosed)
}

// acceptRetry calls accept until it yields a value, retrying failures with a
// capped backoff. It returns an error only when the listener has been closed,
// so a caller can treat any return as fatal.
//
// Accept fails for reasons that have nothing to do with the listener being
// unusable -- most often EMFILE when a burst of connections exhausts the file
// descriptor table, or ECONNABORTED when a peer disappears mid-handshake.
// Listeners.Serve calls a listener's Serve exactly once with nothing to
// restart it, so returning on those takes the listener down for good, and
// nothing is logged. The OS listening socket stays open, so clients keep
// completing the TCP handshake and then hang until their own timeout, each
// occupying an accept queue slot.
//
// The policy lives here rather than inside each Serve so it can be adopted or
// removed in one place. It is generic so that TCP and Unix (net.Conn) can share
// it.
//
// accept should be hoisted out of the caller's loop: taking a method value such
// as l.listen.Accept inside the loop is easy to do and needlessly obscures the
// hot path.
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
			// The backoff is per call rather than per listener, so a new
			// connection attempt always starts optimistic while a sustained
			// failure still backs off within this call.
			return v, nil
		}

		// done is closed by Close, and is the only reliable stop signal: the
		// error a closed listener produces differs by listener type.
		select {
		case <-done:
			return zero, errListenerStopped
		default:
		}

		// Safety net for a listener closed without going through Close, which
		// would otherwise be retried forever.
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