package EventEngine

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------
// Basic functionality
// ---------------------------------------------------------------------

func TestEngineCreation(t *testing.T) {
	t.Parallel()

	eng := NewEventEngine(0)
	defer eng.Stop()
	if eng.Workers() != 1 {
		t.Errorf("workers=0 -> default 1, got %d", eng.Workers())
	}

	eng2 := NewEventEngine(-5)
	defer eng2.Stop()
	if eng2.Workers() != 1 {
		t.Errorf("workers=-5 -> default 1, got %d", eng2.Workers())
	}

	eng3 := NewEventEngine(4, 1024)
	defer eng3.Stop()
	if eng3.Workers() != 4 {
		t.Errorf("workers=4, got %d", eng3.Workers())
	}
}

func TestPublishAndDispatch(t *testing.T) {
	eng := NewEventEngine(2)
	defer eng.Stop()

	var count atomic.Int64
	eng.AddImmediateListener(NewEvent("ping"), func(e *Event) {
		count.Add(1)
	})

	for i := 0; i < 10; i++ {
		eng.Publish(NewEvent("ping", i))
	}
	// Allow some time for async processing
	time.Sleep(100 * time.Millisecond)

	if got := count.Load(); got != 10 {
		t.Errorf("expected 10 events processed, got %d", got)
	}
}

func TestPublishTry(t *testing.T) {
	eng := NewEventEngine(1, 1) // queue size 1
	defer eng.Stop()

	// Fill the queue
	err := eng.PublishTry(NewEvent("fill", 1))
	if err != nil {
		t.Fatalf("first PublishTry failed: %v", err)
	}
	err = eng.PublishTry(NewEvent("fill", 2))
	if err != ErrQueueFull {
		t.Errorf("second PublishTry should return ErrQueueFull, got %v", err)
	}
}

func TestPublishBlocking(t *testing.T) {
	eng := NewEventEngine(2, 16)
	defer eng.Stop()

	var count atomic.Int64
	eng.AddImmediateListener(NewEvent("block"), func(e *Event) {
		count.Add(1)
		time.Sleep(10 * time.Millisecond)
	})

	const n = 50
	for i := 0; i < n; i++ {
		eng.PublishBlocking(NewEvent("block", i))
	}
	time.Sleep(500 * time.Millisecond)

	if got := count.Load(); got != n {
		t.Errorf("expected %d, got %d", n, got)
	}
}

// ---------------------------------------------------------------------
// Listeners
// ---------------------------------------------------------------------

func TestMultipleListeners(t *testing.T) {
	eng := NewEventEngine(2)
	defer eng.Stop()

	var c1, c2, c3 atomic.Int64
	eng.AddImmediateListener(NewEvent("multi"), func(e *Event) { c1.Add(1) })
	eng.AddImmediateListener(NewEvent("multi"), func(e *Event) { c2.Add(1) })
	eng.AddImmediateListener(NewEvent("multi"), func(e *Event) { c3.Add(1) })

	eng.Publish(NewEvent("multi"))
	time.Sleep(50 * time.Millisecond)

	if c1.Load() != 1 || c2.Load() != 1 || c3.Load() != 1 {
		t.Errorf("listeners not all called: %d, %d, %d", c1.Load(), c2.Load(), c3.Load())
	}
}

func TestNoListenerNoPanic(t *testing.T) {
	eng := NewEventEngine(1)
	defer eng.Stop()

	// Should not panic
	eng.Publish(NewEvent("nonexistent"))
	time.Sleep(50 * time.Millisecond)
}

func TestDynamicListenerAddition(t *testing.T) {
	eng := NewEventEngine(2)
	defer eng.Stop()

	var pre, post atomic.Int64

	eng.AddImmediateListener(NewEvent("dynamic"), func(e *Event) { pre.Add(1) })
	eng.Publish(NewEvent("dynamic", "before"))
	time.Sleep(30 * time.Millisecond)

	eng.AddImmediateListener(NewEvent("dynamic"), func(e *Event) { post.Add(1) })
	eng.Publish(NewEvent("dynamic", "after"))
	time.Sleep(30 * time.Millisecond)

	if pre.Load() != 2 {
		t.Errorf("pre listener should receive 2 events, got %d", pre.Load())
	}
	if post.Load() != 1 {
		t.Errorf("post listener should receive 1 event, got %d", post.Load())
	}
}

func TestPanicRecovery(t *testing.T) {
	eng := NewEventEngine(2)
	defer eng.Stop()

	var normal atomic.Int64
	eng.AddImmediateListener(NewEvent("panic"), func(e *Event) {
		panic("intentional panic for test")
	})
	eng.AddImmediateListener(NewEvent("panic"), func(e *Event) {
		normal.Add(1)
	})

	eng.Publish(NewEvent("panic"))
	time.Sleep(50 * time.Millisecond)

	if normal.Load() != 1 {
		t.Errorf("normal handler should still run, got %d", normal.Load())
	}
}

// ---------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------

func TestConcurrentPublish(t *testing.T) {
	eng := NewEventEngine(8, 8192)
	defer eng.Stop()

	var count atomic.Int64
	eng.AddImmediateListener(NewEvent("concurrent"), func(e *Event) { count.Add(1) })

	const goroutines = 10
	const perGoroutine = 500
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				eng.Publish(NewEvent("concurrent", i))
			}
		}()
	}
	wg.Wait()
	// Allow processing
	time.Sleep(500 * time.Millisecond)

	expected := int64(goroutines * perGoroutine)
	if got := count.Load(); got != expected {
		t.Errorf("expected %d, got %d", expected, got)
	}
}

func TestConcurrentPublishBlocking(t *testing.T) {
	eng := NewEventEngine(4, 2048)
	defer eng.Stop()

	var count atomic.Int64
	eng.AddImmediateListener(NewEvent("block.conc"), func(e *Event) { count.Add(1) })

	const goroutines = 5
	const perGoroutine = 200
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				eng.PublishBlocking(NewEvent("block.conc", i))
			}
		}()
	}
	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	expected := int64(goroutines * perGoroutine)
	if got := count.Load(); got != expected {
		t.Errorf("expected %d, got %d", expected, got)
	}
}

// ---------------------------------------------------------------------
// Stop and cleanup
// ---------------------------------------------------------------------

func TestStopBehavior(t *testing.T) {
	eng := NewEventEngine(2)
	var count atomic.Int64
	eng.AddImmediateListener(NewEvent("stop"), func(e *Event) { count.Add(1) })

	eng.Publish(NewEvent("stop", 1))
	eng.Publish(NewEvent("stop", 2))
	time.Sleep(30 * time.Millisecond)

	eng.Stop()
	// After stop, no more events should be processed (but we can't publish safely)
	// We just ensure no panic and count equals pre-stop publishes.
	if count.Load() != 2 {
		t.Errorf("expected 2 events processed before stop, got %d", count.Load())
	}
}

func TestStopDrainsQueue(t *testing.T) {
	eng := NewEventEngine(1, 256)
	var count atomic.Int64
	eng.AddImmediateListener(NewEvent("drain"), func(e *Event) { count.Add(1) })

	for i := 0; i < 100; i++ {
		eng.Publish(NewEvent("drain", i))
	}
	time.Sleep(50 * time.Millisecond)
	eng.Stop()

	// All queued events should be processed before stop completes
	if count.Load() != 100 {
		t.Errorf("expected 100, got %d", count.Load())
	}
}

// ---------------------------------------------------------------------
// Queue monitoring
// ---------------------------------------------------------------------

func TestQueueLen(t *testing.T) {
	eng := NewEventEngine(1, 256)
	defer eng.Stop()

	if ql := eng.QueueLen(); ql != 0 {
		t.Errorf("initial queue length = %d, want 0", ql)
	}

	for i := 0; i < 50; i++ {
		eng.Publish(NewEvent("ql", i))
	}
	// It may already have been consumed; we just check it's non-negative.
	if ql := eng.QueueLen(); ql < 0 {
		t.Errorf("QueueLen negative: %d", ql)
	}
}

// ---------------------------------------------------------------------
// Large payload
// ---------------------------------------------------------------------

func TestLargePayload(t *testing.T) {
	eng := NewEventEngine(2, 128)
	defer eng.Stop()

	var receivedLen int64
	eng.AddImmediateListener(NewEvent("large"), func(e *Event) {
		data, ok := e.GetData().([]byte)
		if ok {
			atomic.StoreInt64(&receivedLen, int64(len(data)))
		}
	})

	largeData := make([]byte, 1024*1024) // 1MB
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}
	eng.Publish(NewEvent("large", largeData))
	time.Sleep(100 * time.Millisecond)

	if got := atomic.LoadInt64(&receivedLen); got != int64(len(largeData)) {
		t.Errorf("expected payload length %d, got %d", len(largeData), got)
	}
}

// ---------------------------------------------------------------------
// Performance (optional, not run by default)
// ---------------------------------------------------------------------

func BenchmarkEventEngine(b *testing.B) {
	eng := NewEventEngine(8, 65536)
	defer eng.Stop()

	var processed atomic.Int64
	done := make(chan struct{})
	eng.AddImmediateListener(NewEvent("bench"), func(e *Event) {
		if processed.Add(1) == int64(b.N) {
			close(done)
		}
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng.PublishBlocking(NewEvent("bench", i))
	}
	<-done
}
