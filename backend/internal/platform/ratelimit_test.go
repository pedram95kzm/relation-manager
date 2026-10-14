package platform

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimitConcurrent(t *testing.T) {
	l := NewLimiter(5, time.Minute)
	var total atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Allow("one") {
				total.Add(1)
			}
		}()
	}
	wg.Wait()
	if total.Load() != 5 {
		t.Fatalf("allowed %d", total.Load())
	}
	l.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if !l.Allow("one") {
		t.Fatal("window did not reset")
	}
}

func TestRateLimitCapacityPreservesKnownClients(t *testing.T) {
	l := NewLimiter(3, time.Minute)
	for i := 0; i < 10000; i++ {
		if !l.Allow(strconv.Itoa(i)) {
			t.Fatal("unexpected capacity rejection")
		}
	}
	if l.Allow("new-client") {
		t.Fatal("unbounded new bucket")
	}
	if !l.Allow("0") {
		t.Fatal("capacity denied an existing client")
	}
}
