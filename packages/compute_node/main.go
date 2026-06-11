// Package main 演示高性能事件引擎的完整用法。
package main

import (
	"fmt"
	"sync/atomic"
	"time"

	"compute_node/engine"
	"compute_node/event"
)

func main() {
	fmt.Println("═══════════════════════════════════════")
	fmt.Println("       高性能 Go 事件引擎演示")
	fmt.Println("═══════════════════════════════════════")

	// ─── 1. Event 基础功能 ───────────────────────────────────────
	fmt.Println("\n【1】Event 基础功能")

	e1 := event.New("user.login", map[string]string{"user": "alice", "ip": "127.0.0.1"})
	e2 := event.New("user.login")
	e3 := event.New("user.logout", 42)

	fmt.Println("e1:", e1)
	fmt.Println("e2:", e2)
	fmt.Println("e3:", e3)
	fmt.Printf("e1.GetName() = %q\n", e1.GetName())
	fmt.Printf("e1.GetData() = %v\n", e1.GetData())
	fmt.Printf("e1 == e2 (Equals): %v\n", e1.Equals(e2))   // true：同名
	fmt.Printf("e1 == e3 (Equals): %v\n", e1.Equals(e3))   // false：不同名
	fmt.Printf("e1 == nil (Equals): %v\n", e1.Equals(nil)) // false

	// ─── 2. 引擎启动 & 监听器注册 ────────────────────────────────
	fmt.Println("\n【2】引擎启动（4 workers）& 监听器注册")

	eng := engine.New(4)
	defer eng.Stop()

	loginEvent := event.New("user.login")
	logoutEvent := event.New("user.logout")
	orderEvent := event.New("order.created")

	var loginCount, logoutCount, orderCount atomic.Int64

	eng.AddImmediateListener(loginEvent, func(e *event.Event) {
		loginCount.Add(1)
		data, _ := e.GetData().(map[string]string)
		fmt.Printf("  [login handler-1] user=%s ip=%s\n", data["user"], data["ip"])
	})
	eng.AddImmediateListener(loginEvent, func(e *event.Event) {
		loginCount.Add(1)
		fmt.Printf("  [login handler-2] 审计日志: %s\n", e)
	})
	eng.AddImmediateListener(logoutEvent, func(e *event.Event) {
		logoutCount.Add(1)
		fmt.Printf("  [logout handler] %s\n", e)
	})
	eng.AddImmediateListener(orderEvent, func(e *event.Event) {
		orderCount.Add(1)
		fmt.Printf("  [order handler] 订单 ID=%v\n", e.GetData())
	})

	// ─── 3. Publish + 自动派发（引擎主循环） ─────────────────────
	fmt.Println("\n【3】Publish + 引擎主循环自动派发")

	eng.Publish(event.New("user.login", map[string]string{"user": "alice", "ip": "10.0.0.1"}))
	eng.Publish(event.New("user.login", map[string]string{"user": "bob", "ip": "10.0.0.2"}))
	eng.Publish(event.New("user.logout", nil))
	eng.Publish(event.New("order.created", "ORD-9527"))

	time.Sleep(100 * time.Millisecond) // 等待异步处理完成

	// ─── 4. ProcessOneStep ───────────────────────────────────────
	fmt.Println("\n【4】ProcessOneStep 演示")

	eng2 := engine.New(2)
	defer eng2.Stop()

	stepEvent := event.New("step.tick")
	eng2.AddImmediateListener(stepEvent, func(e *event.Event) {
		fmt.Printf("  [step] 处理: %s\n", e)
	})

	eng2.Publish(event.New("step.tick", "A"))
	eng2.Publish(event.New("step.tick", "B"))

	empty := eng2.ProcessOneStep()
	fmt.Println("  第1次 ProcessOneStep，队列空?", empty) // false
	empty = eng2.ProcessOneStep()
	fmt.Println("  第2次 ProcessOneStep，队列空?", empty) // false
	empty = eng2.ProcessOneStep()
	fmt.Println("  第3次 ProcessOneStep，队列空?", empty) // true

	// ─── 5. Process（并行批量派发） ──────────────────────────────
	fmt.Println("\n【5】Process 并行批量派发")

	eng3 := engine.New(4)
	defer eng3.Stop()

	var processCount atomic.Int64
	batchEvent := event.New("batch.item")
	eng3.AddImmediateListener(batchEvent, func(e *event.Event) {
		processCount.Add(1)
	})

	const batchSize = 100
	for i := 0; i < batchSize; i++ {
		eng3.Publish(event.New("batch.item", i))
	}
	fmt.Printf("  发布 %d 个事件，队列长度: %d\n", batchSize, eng3.QueueLen())
	eng3.Process()
	time.Sleep(50 * time.Millisecond)
	fmt.Printf("  Process 完成，处理计数: %d\n", processCount.Load())

	// ─── 6. 性能压测 ─────────────────────────────────────────────
	fmt.Println("\n【6】性能压测（8 workers，100k 事件）")

	eng4 := engine.New(8, 200000)
	defer eng4.Stop()

	const total = 100_000
	var processed atomic.Int64
	done := make(chan struct{})

	perfEvent := event.New("perf.test")
	eng4.AddImmediateListener(perfEvent, func(e *event.Event) {
		if processed.Add(1) == total {
			close(done)
		}
	})

	start := time.Now()
	for i := 0; i < total; i++ {
		eng4.PublishBlocking(event.New("perf.test", i))
	}
	<-done
	elapsed := time.Since(start)
	fmt.Printf("  处理 %d 事件，耗时 %v，吞吐量 %.0f events/s\n",
		total, elapsed, float64(total)/elapsed.Seconds())

	fmt.Println("\n═══════════════════════════════════════")
	fmt.Println("              演示完成 ✓")
	fmt.Println("═══════════════════════════════════════")
}
