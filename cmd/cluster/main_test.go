package main

import (
	"context"
	"go.uber.org/goleak"
	"os"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// TestLeaks tests that there are no goroutine leaks after starting and stopping the server.
/* The following goroutines are expected to be running:
- http server for pprof
- ants cleanup goroutine

*/
func TestLeaks(t *testing.T) {

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		os.Args = []string{"binary", "-node-name", "testnode", "-members", "localhost:7946",
			"-raft-bootstrap", "true", "-storage-way", "memory"}
		err := realMain(ctx)
		if err != nil {
			panic("realMain error" + err.Error())
		}
	}()

	time.Sleep(time.Millisecond * 100)
	cancel()
	wg.Wait()
	// goleak.IgnoreTopFunction()
	goleak.VerifyNone(t,
		// ignore the ants cleanup goroutines
		goleak.IgnoreTopFunction("github.com/panjf2000/ants/v2.(*Pool).purgeStaleWorkers"),
		goleak.IgnoreTopFunction("github.com/panjf2000/ants/v2.(*Pool).ticktock"),
		// ignore the pprof http server goroutine
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"))

}
