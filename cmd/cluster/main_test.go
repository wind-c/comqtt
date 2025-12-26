package main

import (
	"context"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
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
	// skip test if there is no redis server:
	if !hasRedis() {
		t.SkipNow()
	}
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
		goleak.IgnoreTopFunction("github.com/panjf2000/ants/v2.(*poolCommon).purgeStaleWorkers"),
		goleak.IgnoreTopFunction("github.com/panjf2000/ants/v2.(*Pool).ticktock"),
		goleak.IgnoreTopFunction("github.com/panjf2000/ants/v2.(*poolCommon).ticktock"),
		// ignore the pprof http server goroutine
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"))

}

// hasRedis does a TCP connect to port 6379 to see if there is a redis server running on localhost.
func hasRedis() bool {
	c, err := net.Dial("tcp", "localhost:6379")
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}
