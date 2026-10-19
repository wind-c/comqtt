// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2023 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package listeners

import (
	"log/slog"
	"net"
	"testing"
)

// fastListener stands in for a real listener so the benchmark measures the
// wrapper, not the accept syscall.
//
// The benchmarks hold it in a net.Listener interface variable, the way the real
// code does. A concrete type would devirtualise and hide the cost of taking a
// method value, which is the thing under test here.
type fastListener struct{ net.Listener }

func (fastListener) Accept() (net.Conn, error) { return nil, nil }

func newIfaceListener() net.Listener { return fastListener{} }

// BenchmarkAcceptInline is what the Serve loop does today.
func BenchmarkAcceptInline(b *testing.B) {
	ln := newIfaceListener()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ln.Accept()
	}
}

// BenchmarkAcceptRetryHoisted hoists the method value out of the loop, which is
// how the call sites are meant to call it.
func BenchmarkAcceptRetryHoisted(b *testing.B) {
	ln := newIfaceListener()
	log := slog.New(slog.DiscardHandler)
	done := make(chan struct{})
	accept := ln.Accept

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = acceptRetry(accept, nil, done, log)
	}
}

// BenchmarkAcceptRetryMethodValue passes l.listen.Accept inline, which may build
// a new closure on every iteration.
func BenchmarkAcceptRetryMethodValue(b *testing.B) {
	ln := newIfaceListener()
	log := slog.New(slog.DiscardHandler)
	done := make(chan struct{})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = acceptRetry(ln.Accept, nil, done, log)
	}
}