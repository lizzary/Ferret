package UnitTest

import (
	"compute-node/packages/EventEngine"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Event 类型测试
// ============================================================================

func testEventBasic(t *T) {
	e1 := EventEngine.NewEvent("user.login", map[string]string{"user": "alice", "ip": "127.0.0.1"})
	e2 := EventEngine.NewEvent("user.login")
	e3 := EventEngine.NewEvent("user.logout", 42)
	e5 := EventEngine.NewEvent("empty.event")

	t.Equal("user.login", e1.GetName(), "e1 name")
	t.Equal("user.login", e2.GetName(), "e2 name")
	t.Equal("user.logout", e3.GetName(), "e3 name")

	t.NotNil(e1.GetData(), "e1 data not nil")
	t.NotNil(e3.GetData(), "e3 data not nil")
	t.Nil(e5.GetData(), "e5 data nil")

	t.True(e1.Equals(e2), "same name → true")
	t.False(e1.Equals(e3), "diff name → false")
	t.True(e1.Equals(e1), "same ptr → true")
	t.False(e1.Equals(nil), "nil arg → false")
}

func testEventNewVariants(t *T) {
	t.Equal(42, EventEngine.NewEvent("num", 42).GetData())
	t.Equal("hello world", EventEngine.NewEvent("msg", "hello world").GetData())
	t.Equal(3.14159, EventEngine.NewEvent("pi", 3.14159).GetData())
	t.Equal(true, EventEngine.NewEvent("flag", true).GetData())
	t.Equal([]int{1, 2, 3}, EventEngine.NewEvent("list", []int{1, 2, 3}).GetData())

	type Payload struct {
		ID   int
		Name string
	}
	t.Equal(Payload{ID: 1, Name: "test"}, EventEngine.NewEvent("struct", Payload{ID: 1, Name: "test"}).GetData())
	t.Nil(EventEngine.NewEvent("nil.event", nil).GetData())
	t.Equal("first", EventEngine.NewEvent("multi", "first", "second", "third").GetData())
}

func testEventEquals(t *T) {
	a := EventEngine.NewEvent("x", 1)
	b := EventEngine.NewEvent("x", 2)
	t.True(a.Equals(b), "same name, different data → true")

	c := EventEngine.NewEvent("")
	d := EventEngine.NewEvent("")
	t.True(c.Equals(d), "both empty name → true")

	t.False(EventEngine.NewEvent("").Equals(EventEngine.NewEvent("non")), "empty vs non-empty → false")
	t.False(EventEngine.NewEvent("User.Login").Equals(EventEngine.NewEvent("user.login")), "case-sensitive → false")
}

func testEventStringFormat(t *T) {
	e1 := EventEngine.NewEvent("test.event", "payload")
	t.Equal(`Event{name="test.event", data=payload}`, e1.String())

	e2 := EventEngine.NewEvent("test.event")
	t.Equal(`Event{name="test.event", data=<nil>}`, e2.String())

	e3 := EventEngine.NewEvent("", nil)
	t.Equal(`Event{name="", data=<nil>}`, e3.String())
}

func testEventEqualityWithData(t *T) {
	type complexData struct {
		ID     int
		Name   string
		Items  []string
		Nested map[string]int
	}

	e1 := EventEngine.NewEvent("order.updated", complexData{ID: 1, Name: "a"})
	e2 := EventEngine.NewEvent("order.updated", complexData{ID: 2, Name: "b"})
	e3 := EventEngine.NewEvent("order.updated", nil)

	t.True(e1.Equals(e2), "diff data, same name → true")
	t.True(e1.Equals(e3), "struct vs nil data, same name → true")
	t.True(e2.Equals(e3), "struct2 vs nil data, same name → true")
}

// ============================================================================
// EventEngine 包测试
// ============================================================================

func testEngineCreation(t *T) {
	eng0 := EventEngine.NewEventEngine(0)
	t.Equal(1, eng0.Workers(), "workers=0 → default 1")
	eng0.Stop()

	engNeg := EventEngine.NewEventEngine(-5)
	t.Equal(1, engNeg.Workers(), "workers=-5 → default 1")
	engNeg.Stop()

	eng4 := EventEngine.NewEventEngine(4)
	t.Equal(4, eng4.Workers(), "workers=4")
	eng4.Stop()
}

func testEnginePublishAndDispatch(t *T) {
	eng := EventEngine.NewEventEngine(4)
	defer eng.Stop()

	var count atomic.Int64
	eng.AddImmediateListener(EventEngine.NewEvent("user.login"), func(e *EventEngine.Event) {
		count.Add(1)
	})

	eng.Publish(EventEngine.NewEvent("user.login", map[string]string{"user": "alice"}))
	eng.Publish(EventEngine.NewEvent("user.login", map[string]string{"user": "bob"}))
	eng.Publish(EventEngine.NewEvent("user.login", map[string]string{"user": "charlie"}))

	time.Sleep(50 * time.Millisecond)
	t.Equal(int64(3), count.Load(), "3 events dispatched")
}

func testEnginePublishTry(t *T) {
	eng := EventEngine.NewEventEngine(1, 4096)
	defer eng.Stop()

	err := eng.PublishTry(EventEngine.NewEvent("test", "data"))
	t.NoError(err, "PublishTry with room")

	eng2 := EventEngine.NewEventEngine(1, 1)
	defer eng2.Stop()

	_ = eng2.PublishTry(EventEngine.NewEvent("fill", 1))
	err2 := eng2.PublishTry(EventEngine.NewEvent("fill", 2))
	t.ErrorIs(err2, EventEngine.ErrQueueFull, "queue full → ErrQueueFull")
}

func testEnginePublishBlocking(t *T) {
	eng := EventEngine.NewEventEngine(2, 16)
	defer eng.Stop()

	var count atomic.Int64
	eng.AddImmediateListener(EventEngine.NewEvent("block"), func(e *EventEngine.Event) {
		count.Add(1)
		time.Sleep(1 * time.Millisecond)
	})

	const n = 50
	for i := 0; i < n; i++ {
		eng.PublishBlocking(EventEngine.NewEvent("block", i))
	}
	time.Sleep(200 * time.Millisecond)
	t.Equal(int64(n), count.Load(), "all blocking publishes dispatched")
}

func testEngineMultipleListeners(t *T) {
	eng := EventEngine.NewEventEngine(2)
	defer eng.Stop()

	var c1, c2, c3 atomic.Int64
	eng.AddImmediateListener(EventEngine.NewEvent("multi"), func(e *EventEngine.Event) { c1.Add(1) })
	eng.AddImmediateListener(EventEngine.NewEvent("multi"), func(e *EventEngine.Event) { c2.Add(1) })
	eng.AddImmediateListener(EventEngine.NewEvent("multi"), func(e *EventEngine.Event) { c3.Add(1) })

	eng.Publish(EventEngine.NewEvent("multi", nil))
	time.Sleep(30 * time.Millisecond)

	t.Equal(int64(1), c1.Load(), "listener 1 triggered")
	t.Equal(int64(1), c2.Load(), "listener 2 triggered")
	t.Equal(int64(1), c3.Load(), "listener 3 triggered")
}

func testEngineNoListener(t *T) {
	eng := EventEngine.NewEventEngine(2)
	defer eng.Stop()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.log(false, fmt.Sprintf("no-listener event panicked: %v", r))
			}
		}()
		eng.Publish(EventEngine.NewEvent("no.listener", "ignored"))
		time.Sleep(30 * time.Millisecond)
		eng2 := EventEngine.NewEventEngine(1)
		defer eng2.Stop()
		eng2.Publish(EventEngine.NewEvent("ghost", nil))
		time.Sleep(30 * time.Millisecond)
	}()
	t.True(true, "no-listener event: no panic")
}

func testEngineDynamicListener(t *T) {
	eng := EventEngine.NewEventEngine(2)
	defer eng.Stop()

	var pre, post atomic.Int64

	eng.AddImmediateListener(EventEngine.NewEvent("dynamic"), func(e *EventEngine.Event) { pre.Add(1) })
	eng.Publish(EventEngine.NewEvent("dynamic", "before"))
	time.Sleep(30 * time.Millisecond)

	eng.AddImmediateListener(EventEngine.NewEvent("dynamic"), func(e *EventEngine.Event) { post.Add(1) })
	eng.Publish(EventEngine.NewEvent("dynamic", "after"))
	time.Sleep(30 * time.Millisecond)

	t.Equal(int64(2), pre.Load(), "pre listener received both events")
	t.Equal(int64(1), post.Load(), "post listener received only second event")
}

func testEngineHandlerPanic(t *T) {
	eng := EventEngine.NewEventEngine(2)
	defer eng.Stop()

	var normalCount atomic.Int64
	eng.AddImmediateListener(EventEngine.NewEvent("panic.test"), func(e *EventEngine.Event) {
		panic("intentional panic for testing!")
	})
	eng.AddImmediateListener(EventEngine.NewEvent("panic.test"), func(e *EventEngine.Event) {
		normalCount.Add(1)
	})

	eng.Publish(EventEngine.NewEvent("panic.test", nil))
	time.Sleep(50 * time.Millisecond)

	t.Equal(int64(1), normalCount.Load(), "normal handler ran after panic recovery")
}

func testEngineMultipleEventTypes(t *T) {
	eng := EventEngine.NewEventEngine(4)
	defer eng.Stop()

	var loginCount, logoutCount, orderCount, paymentCount atomic.Int64

	eng.AddImmediateListener(EventEngine.NewEvent("user.login"), func(e *EventEngine.Event) { loginCount.Add(1) })
	eng.AddImmediateListener(EventEngine.NewEvent("user.logout"), func(e *EventEngine.Event) { logoutCount.Add(1) })
	eng.AddImmediateListener(EventEngine.NewEvent("order.created"), func(e *EventEngine.Event) { orderCount.Add(1) })
	eng.AddImmediateListener(EventEngine.NewEvent("payment.done"), func(e *EventEngine.Event) { paymentCount.Add(1) })

	for i := 0; i < 10; i++ {
		eng.Publish(EventEngine.NewEvent("user.login", i))
	}
	for i := 0; i < 5; i++ {
		eng.Publish(EventEngine.NewEvent("user.logout", i))
	}
	for i := 0; i < 8; i++ {
		eng.Publish(EventEngine.NewEvent("order.created", i))
	}
	for i := 0; i < 3; i++ {
		eng.Publish(EventEngine.NewEvent("payment.done", i))
	}

	time.Sleep(200 * time.Millisecond)

	t.Equal(int64(10), loginCount.Load(), "user.login x10")
	t.Equal(int64(5), logoutCount.Load(), "user.logout x5")
	t.Equal(int64(8), orderCount.Load(), "order.created x8")
	t.Equal(int64(3), paymentCount.Load(), "payment.done x3")
}

func testEngineConcurrentPublish(t *T) {
	eng := EventEngine.NewEventEngine(8, 8192)
	defer eng.Stop()

	var count atomic.Int64
	eng.AddImmediateListener(EventEngine.NewEvent("concurrent"), func(e *EventEngine.Event) { count.Add(1) })

	const goroutines = 10
	const perGoroutine = 500
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				eng.Publish(EventEngine.NewEvent("concurrent", map[string]int{"g": id, "i": i}))
			}
		}(g)
	}

	wg.Wait()
	time.Sleep(500 * time.Millisecond)

	total := int64(goroutines * perGoroutine)
	t.Equal(total, count.Load(), "all concurrent events dispatched")
}

func testEngineConcurrentPublishBlocking(t *T) {
	eng := EventEngine.NewEventEngine(4, 2048)
	defer eng.Stop()

	var count atomic.Int64
	eng.AddImmediateListener(EventEngine.NewEvent("concurrent.block"), func(e *EventEngine.Event) { count.Add(1) })

	const goroutines = 5
	const perGoroutine = 200
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				eng.PublishBlocking(EventEngine.NewEvent("concurrent.block", i))
			}
		}(g)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	total := int64(goroutines * perGoroutine)
	t.Equal(total, count.Load(), "all concurrent PublishBlocking dispatched")
}

func testEngineStopBehavior(t *T) {
	eng := EventEngine.NewEventEngine(2)
	var count atomic.Int64
	eng.AddImmediateListener(EventEngine.NewEvent("stop.test"), func(e *EventEngine.Event) { count.Add(1) })

	eng.Publish(EventEngine.NewEvent("stop.test", 1))
	eng.Publish(EventEngine.NewEvent("stop.test", 2))
	time.Sleep(30 * time.Millisecond)

	eng.Stop()
	t.Equal(int64(2), count.Load(), "events dispatched before stop")
}

func testEngineStopDrainsQueue(t *T) {
	eng := EventEngine.NewEventEngine(1, 256)
	var count atomic.Int64
	eng.AddImmediateListener(EventEngine.NewEvent("drain"), func(e *EventEngine.Event) { count.Add(1) })

	eng.Publish(EventEngine.NewEvent("drain", 1))
	time.Sleep(20 * time.Millisecond)
	eng.Stop()

	beforeStop := count.Load()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.log(false, fmt.Sprintf("publish after stop panicked: %v", r))
			}
		}()
		eng.Publish(EventEngine.NewEvent("drain", 2))
		eng.Publish(EventEngine.NewEvent("drain", 3))
	}()
	t.NotEqual(int64(0), beforeStop, "events handled before stop")
	t.True(true, "publish after stop: no panic")
}

func testEngineLargePayload(t *T) {
	eng := EventEngine.NewEventEngine(2, 128)
	defer eng.Stop()

	var receivedLen atomic.Int64
	eng.AddImmediateListener(EventEngine.NewEvent("large"), func(e *EventEngine.Event) {
		data, _ := e.GetData().([]byte)
		receivedLen.Store(int64(len(data)))
	})

	largeData := make([]byte, 1024*1024) // 1MB
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}
	eng.Publish(EventEngine.NewEvent("large", largeData))
	time.Sleep(100 * time.Millisecond)

	t.Equal(int64(len(largeData)), receivedLen.Load(), "1MB payload intact")
}

func testEnginePerformance(t *T) {
	eng := EventEngine.NewEventEngine(8, 65536)
	defer eng.Stop()

	const total = 50_000
	var processed atomic.Int64
	done := make(chan struct{})

	eng.AddImmediateListener(EventEngine.NewEvent("perf"), func(e *EventEngine.Event) {
		if processed.Add(1) == total {
			close(done)
		}
	})

	start := time.Now()
	for i := 0; i < total; i++ {
		eng.PublishBlocking(EventEngine.NewEvent("perf", i))
	}
	<-done
	elapsed := time.Since(start)

	t.Equal(int64(total), processed.Load(), "all events processed")
	fmt.Printf("    throughput: %.0f events/s\n", float64(total)/elapsed.Seconds())
}

func testEngineQueueLen(t *T) {
	eng := EventEngine.NewEventEngine(1, 256)
	defer eng.Stop()

	t.Equal(0, eng.QueueLen(), "initial QueueLen=0")

	for i := 0; i < 50; i++ {
		eng.Publish(EventEngine.NewEvent("ql", i))
	}
	qLen := eng.QueueLen()
	t.True(qLen >= 0, fmt.Sprintf("QueueLen after 50 publishes = %d (>=0)", qLen))
}

// RunEventEngineTests 执行所有 Event / EventEngine 测试。
func RunEventEngineTests() (passed, failed int) {
	return RunSuite("Event & EventEngine 完整测试套件", []testCase{
		{"EventBasic", testEventBasic},
		{"EventNewVariants", testEventNewVariants},
		{"EventEquals", testEventEquals},
		{"EventStringFormat", testEventStringFormat},
		{"EventEqualityWithData", testEventEqualityWithData},
		{"EngineCreation", testEngineCreation},
		{"EnginePublishAndDispatch", testEnginePublishAndDispatch},
		{"EnginePublishTry", testEnginePublishTry},
		{"EnginePublishBlocking", testEnginePublishBlocking},
		{"EngineMultipleListeners", testEngineMultipleListeners},
		{"EngineNoListener", testEngineNoListener},
		{"EngineDynamicListener", testEngineDynamicListener},
		{"EngineHandlerPanic", testEngineHandlerPanic},
		{"EngineMultipleEventTypes", testEngineMultipleEventTypes},
		{"EngineConcurrentPublish", testEngineConcurrentPublish},
		{"EngineConcurrentPublishBlocking", testEngineConcurrentPublishBlocking},
		{"EngineStopBehavior", testEngineStopBehavior},
		{"EngineStopDrainsQueue", testEngineStopDrainsQueue},
		{"EngineLargePayload", testEngineLargePayload},
		{"EngineQueueLen", testEngineQueueLen},
		{"EnginePerformance", testEnginePerformance},
	})
}
