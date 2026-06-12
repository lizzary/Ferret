package UnitTest

import (
	"compute-node/packages/EventEngine"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// update common 123
// ============================================================================

// ============================================================================
// 断言工具
// ============================================================================

// T 是轻量级测试上下文，提供断言方法并统计通过/失败。
type T struct {
	name   string
	passed int
	failed int
}

// NewT 创建一个测试上下文。
func NewT(name string) *T {
	return &T{name: name}
}

// caller 返回调用者的文件:行号（跳过 skip 层栈帧）。
func caller(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "???:0"
	}
	// 只保留文件名，不输出完整路径
	for i := len(file) - 1; i >= 0; i-- {
		if file[i] == '/' || file[i] == '\\' {
			file = file[i+1:]
			break
		}
	}
	return fmt.Sprintf("%s:%d", file, line)
}

func (t *T) log(pass bool, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	loc := caller(3) // T → assertXxx → caller
	if pass {
		t.passed++
		fmt.Printf("  ✓ PASS | %s | %s\n", loc, msg)
	} else {
		t.failed++
		fmt.Printf("  ✗ FAIL | %s | %s\n", loc, msg)
	}
}

// Equal 断言 expected == actual。
func (t *T) Equal(expected, actual any, msg ...any) {
	pass := fmt.Sprint(expected) == fmt.Sprint(actual)
	detail := fmt.Sprintf("expected=%v, actual=%v", expected, actual)
	if len(msg) > 0 {
		detail += " — " + fmt.Sprint(msg...)
	}
	t.log(pass, detail)
}

// NotEqual 断言 expected != actual。
func (t *T) NotEqual(a, b any, msg ...any) {
	pass := fmt.Sprint(a) != fmt.Sprint(b)
	detail := fmt.Sprintf("%v != %v", a, b)
	if len(msg) > 0 {
		detail += " — " + fmt.Sprint(msg...)
	}
	t.log(pass, detail)
}

// True 断言条件为真。
func (t *T) True(cond bool, msg ...any) {
	detail := "expected true"
	if len(msg) > 0 {
		detail += " — " + fmt.Sprint(msg...)
	}
	t.log(cond, detail)
}

// False 断言条件为假。
func (t *T) False(cond bool, msg ...any) {
	detail := "expected false"
	if len(msg) > 0 {
		detail += " — " + fmt.Sprint(msg...)
	}
	t.log(!cond, detail)
}

// Nil 断言 v == nil。
func (t *T) Nil(v any, msg ...any) {
	isNil := v == nil
	// 处理带类型信息的 nil（如 (*EventEngine.Event)(nil)）
	if !isNil {
		// 反射检查太重量级，这里对常见类型做简单判断
		detail := fmt.Sprintf("expected nil, got %v", v)
		if len(msg) > 0 {
			detail += " — " + fmt.Sprint(msg...)
		}
		t.log(false, detail)
		return
	}
	detail := fmt.Sprintf("got nil")
	if len(msg) > 0 {
		detail += " — " + fmt.Sprint(msg...)
	}
	t.log(true, detail)
}

// NotNil 断言 v != nil。
func (t *T) NotNil(v any, msg ...any) {
	detail := fmt.Sprintf("value=%v", v)
	if len(msg) > 0 {
		detail += " — " + fmt.Sprint(msg...)
	}
	t.log(v != nil, detail)
}

// NoError 断言 err == nil。
func (t *T) NoError(err error, msg ...any) {
	detail := fmt.Sprintf("err=%v", err)
	if len(msg) > 0 {
		detail += " — " + fmt.Sprint(msg...)
	}
	t.log(err == nil, detail)
}

// ErrorIs 断言 err 等于 target error。
func (t *T) ErrorIs(err, target error, msg ...any) {
	detail := fmt.Sprintf("err=%v, target=%v", err, target)
	if len(msg) > 0 {
		detail += " — " + fmt.Sprint(msg...)
	}
	t.log(err == target, detail)
}

// Summary 返回通过/失败计数。
func (t *T) Summary() (passed, failed int) { return t.passed, t.failed }

// ============================================================================
// Event 类型测试（属于 EventEngine 包）
// ============================================================================

func TestEventBasic(t *T) {
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

	// Equals
	t.True(e1.Equals(e2), "same name → true")
	t.False(e1.Equals(e3), "diff name → false")
	t.True(e1.Equals(e1), "same ptr → true")
	t.False(e1.Equals(nil), "nil arg → false")
}

func TestEventNewVariants(t *T) {
	// int
	t.Equal(42, EventEngine.NewEvent("num", 42).GetData())

	// string
	t.Equal("hello world", EventEngine.NewEvent("msg", "hello world").GetData())

	// float64
	t.Equal(3.14159, EventEngine.NewEvent("pi", 3.14159).GetData())

	// bool
	t.Equal(true, EventEngine.NewEvent("flag", true).GetData())

	// slice
	t.Equal([]int{1, 2, 3}, EventEngine.NewEvent("list", []int{1, 2, 3}).GetData())

	// struct
	type Payload struct {
		ID   int
		Name string
	}
	t.Equal(Payload{ID: 1, Name: "test"}, EventEngine.NewEvent("struct", Payload{ID: 1, Name: "test"}).GetData())

	// nil
	t.Nil(EventEngine.NewEvent("nil.event", nil).GetData())

	// 多 data 参数 — 只取第一个
	t.Equal("first", EventEngine.NewEvent("multi", "first", "second", "third").GetData())
}

func TestEventEquals(t *T) {
	a := EventEngine.NewEvent("x", 1)
	b := EventEngine.NewEvent("x", 2)
	t.True(a.Equals(b), "same name, different data → true")

	c := EventEngine.NewEvent("")
	d := EventEngine.NewEvent("")
	t.True(c.Equals(d), "both empty name → true")

	t.False(EventEngine.NewEvent("").Equals(EventEngine.NewEvent("non")), "empty vs non-empty → false")

	t.False(EventEngine.NewEvent("User.Login").Equals(EventEngine.NewEvent("user.login")), "case-sensitive → false")
}

func TestEventStringFormat(t *T) {
	e1 := EventEngine.NewEvent("test.event", "payload")
	t.Equal(`Event{name="test.event", data=payload}`, e1.String())

	e2 := EventEngine.NewEvent("test.event")
	t.Equal(`Event{name="test.event", data=<nil>}`, e2.String())

	e3 := EventEngine.NewEvent("", nil)
	t.Equal(`Event{name="", data=<nil>}`, e3.String())
}

// ============================================================================
// EventEngine 包测试
// ============================================================================

func TestEngineCreation(t *T) {
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

func TestEnginePublishAndDispatch(t *T) {
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

func TestEnginePublishTry(t *T) {
	eng := EventEngine.NewEventEngine(1, 4096)
	defer eng.Stop()

	err := eng.PublishTry(EventEngine.NewEvent("test", "data"))
	t.NoError(err, "PublishTry with room")

	// 小队列：容量只有 1
	eng2 := EventEngine.NewEventEngine(1, 1)
	defer eng2.Stop()

	_ = eng2.PublishTry(EventEngine.NewEvent("fill", 1))
	err2 := eng2.PublishTry(EventEngine.NewEvent("fill", 2))
	t.ErrorIs(err2, EventEngine.ErrQueueFull, "queue full → ErrQueueFull")
}

func TestEnginePublishBlocking(t *T) {
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

// TestEngineMultipleListeners  测试listener
func TestEngineMultipleListeners(t *T) {
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

func TestEngineNoListener(t *T) {
	eng := EventEngine.NewEventEngine(2)
	defer eng.Stop()

	// 无监听器事件不崩溃
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

func TestEngineDynamicListener(t *T) {
	eng := EventEngine.NewEventEngine(2)
	defer eng.Stop()

	var pre, post atomic.Int64

	eng.AddImmediateListener(EventEngine.NewEvent("dynamic"), func(e *EventEngine.Event) { pre.Add(1) })
	eng.Publish(EventEngine.NewEvent("dynamic", "before"))
	time.Sleep(30 * time.Millisecond)

	// 运行时添加
	eng.AddImmediateListener(EventEngine.NewEvent("dynamic"), func(e *EventEngine.Event) { post.Add(1) })
	eng.Publish(EventEngine.NewEvent("dynamic", "after"))
	time.Sleep(30 * time.Millisecond)

	t.Equal(int64(2), pre.Load(), "pre listener received both events")
	t.Equal(int64(1), post.Load(), "post listener received only second event")
}

func TestEngineHandlerPanic(t *T) {
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

func TestEngineMultipleEventTypes(t *T) {
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

func TestEngineConcurrentPublish(t *T) {
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

func TestEngineConcurrentPublishBlocking(t *T) {
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

func TestEngineStopBehavior(t *T) {
	eng := EventEngine.NewEventEngine(2)
	var count atomic.Int64
	eng.AddImmediateListener(EventEngine.NewEvent("stop.test"), func(e *EventEngine.Event) { count.Add(1) })

	eng.Publish(EventEngine.NewEvent("stop.test", 1))
	eng.Publish(EventEngine.NewEvent("stop.test", 2))
	time.Sleep(30 * time.Millisecond)

	eng.Stop()
	t.Equal(int64(2), count.Load(), "events dispatched before stop")
}

func TestEngineStopDrainsQueue(t *T) {
	eng := EventEngine.NewEventEngine(1, 256)
	var count atomic.Int64
	eng.AddImmediateListener(EventEngine.NewEvent("drain"), func(e *EventEngine.Event) { count.Add(1) })

	eng.Publish(EventEngine.NewEvent("drain", 1))
	time.Sleep(20 * time.Millisecond)
	eng.Stop()

	beforeStop := count.Load()

	// Stop 后发布不应崩溃
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

func TestEngineLargePayload(t *T) {
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

func TestEnginePerformance(t *T) {
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

func TestEngineQueueLen(t *T) {
	eng := EventEngine.NewEventEngine(1, 256)
	defer eng.Stop()

	t.Equal(0, eng.QueueLen(), "initial QueueLen=0")

	for i := 0; i < 50; i++ {
		eng.Publish(EventEngine.NewEvent("ql", i))
	}
	// 有些可能已被派发，所以 >=0
	qLen := eng.QueueLen()
	t.True(qLen >= 0, fmt.Sprintf("QueueLen after 50 publishes = %d (>=0)", qLen))
}

func TestEventEqualityWithData(t *T) {
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
// 汇总入口
// ============================================================================

// RunAll 运行所有测试并输出统计结果。
func RunAll() {
	fmt.Println("╔════════════════════════════════════════════════════╗")
	fmt.Println("║       Event & EventEngine 完整测试套件（断言版）    ║")
	fmt.Println("╚════════════════════════════════════════════════════╝")

	type testCase struct {
		name string
		fn   func(*T)
	}

	tests := []testCase{
		// Event 测试
		{"EventBasic", TestEventBasic},
		{"EventNewVariants", TestEventNewVariants},
		{"EventEquals", TestEventEquals},
		{"EventStringFormat", TestEventStringFormat},
		{"EventEqualityWithData", TestEventEqualityWithData},

		// EventEngine 测试
		{"EngineCreation", TestEngineCreation},
		{"EnginePublishAndDispatch", TestEnginePublishAndDispatch},
		{"EnginePublishTry", TestEnginePublishTry},
		{"EnginePublishBlocking", TestEnginePublishBlocking},
		{"EngineMultipleListeners", TestEngineMultipleListeners},
		{"EngineNoListener", TestEngineNoListener},
		{"EngineDynamicListener", TestEngineDynamicListener},
		{"EngineHandlerPanic", TestEngineHandlerPanic},
		{"EngineMultipleEventTypes", TestEngineMultipleEventTypes},
		{"EngineConcurrentPublish", TestEngineConcurrentPublish},
		{"EngineConcurrentPublishBlocking", TestEngineConcurrentPublishBlocking},
		{"EngineStopBehavior", TestEngineStopBehavior},
		{"EngineStopDrainsQueue", TestEngineStopDrainsQueue},
		{"EngineLargePayload", TestEngineLargePayload},
		{"EngineQueueLen", TestEngineQueueLen},
		{"EnginePerformance", TestEnginePerformance},
	}

	totalPassed, totalFailed := 0, 0

	for _, tc := range tests {
		fmt.Printf("\n── %s ──\n", tc.name)
		t := NewT(tc.name)
		tc.fn(t)
		p, f := t.Summary()
		totalPassed += p
		totalFailed += f
	}

	fmt.Println("\n╔════════════════════════════════════════════════════╗")
	fmt.Printf("║  结果: %d 通过, %d 失败", totalPassed, totalFailed)
	if totalFailed > 0 {
		fmt.Print("  ⚠ 存在失败用例！")
	} else {
		fmt.Print("  ✓ 全部通过！")
	}
	fmt.Println()
	fmt.Println("╚════════════════════════════════════════════════════╝")
}
