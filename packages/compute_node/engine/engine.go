// Package engine 提供高性能并发事件引擎。
//
// 架构概览：
//   - 事件队列：带缓冲 channel，发布者直接投递，天然背压。
//   - 监听器注册表：读多写少，sync.RWMutex 保护。
//   - Worker 池：固定大小，通过 semaphore channel 控制并发度。
//   - 引擎主循环：独立 goroutine，持续消费队列，优雅关闭。
package engine

import (
	"context"
	"fmt"
	"sync"

	"compute_node/event"
)

// HandlerFunc 是事件监听器的回调函数类型。
type HandlerFunc func(e *event.Event)

// Engine 是高性能事件引擎。
type Engine struct {
	workers   int
	queue     chan *event.Event
	semaphore chan struct{} // 容量 == workers
	listeners map[string][]HandlerFunc
	mu        sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

const defaultQueueSize = 4096

// New 创建并启动引擎。workers 指定并发度，queueSize 可选（默认 4096）。
func New(workers int, queueSize ...int) *Engine {
	if workers <= 0 {
		workers = 1
	}
	qs := defaultQueueSize
	if len(queueSize) > 0 && queueSize[0] > 0 {
		qs = queueSize[0]
	}

	ctx, cancel := context.WithCancel(context.Background())
	eng := &Engine{
		workers:   workers,
		queue:     make(chan *event.Event, qs),
		semaphore: make(chan struct{}, workers),
		listeners: make(map[string][]HandlerFunc),
		ctx:       ctx,
		cancel:    cancel,
	}
	for i := 0; i < workers; i++ {
		eng.semaphore <- struct{}{}
	}

	eng.wg.Add(1)
	go eng.run()
	return eng
}

// run 是引擎主循环，在独立 goroutine 中运行。
// 每次从 semaphore 取令牌（确保不超过 worker 上限），再取事件派发。
func (eng *Engine) run() {
	defer eng.wg.Done()
	for {
		select {
		case <-eng.ctx.Done():
			return
		case tok := <-eng.semaphore:
			// 拿到令牌，尝试取事件
			select {
			case e := <-eng.queue:
				eng.wg.Add(1)
				go func(ev *event.Event, t struct{}) {
					defer eng.wg.Done()
					defer func() { eng.semaphore <- t }()
					eng.dispatch(ev)
				}(e, tok)
			case <-eng.ctx.Done():
				eng.semaphore <- tok // 归还令牌再退出
				return
			}
		}
	}
}

// dispatch 执行某事件对应的所有监听器（在调用方 goroutine 中执行）。
func (eng *Engine) dispatch(e *event.Event) {
	eng.mu.RLock()
	hs := eng.listeners[e.GetName()]
	handlers := make([]HandlerFunc, len(hs))
	copy(handlers, hs)
	eng.mu.RUnlock()

	for _, h := range handlers {
		safeCall(h, e)
	}
}

func safeCall(h HandlerFunc, e *event.Event) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[engine] handler panic for event %q: %v\n", e.GetName(), r)
		}
	}()
	h(e)
}

// ProcessOneStep 从队列头部同步取出一个事件并执行所有监听器。
// 队列为空返回 true，否则返回 false。
// 此方法绕过 worker 池，在调用方 goroutine 同步执行，适合测试或确定性场景。
func (eng *Engine) ProcessOneStep() bool {
	select {
	case e := <-eng.queue:
		eng.dispatch(e)
		return false
	default:
		return true
	}
}

// Process 并行批量派发当前队列中所有已入队的事件。
// 它直接管理独立的 goroutine 池，不与主循环争抢 semaphore。
func (eng *Engine) Process() {
	n := len(eng.queue)
	if n == 0 {
		return
	}

	// 用独立 semaphore 控制 Process 自己的并发度
	sem := make(chan struct{}, eng.workers)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		select {
		case e := <-eng.queue:
			sem <- struct{}{} // 获取并发令牌
			wg.Add(1)
			go func(ev *event.Event) {
				defer wg.Done()
				defer func() { <-sem }()
				eng.dispatch(ev)
			}(e)
		default:
			// 队列已空，提前退出
			goto done
		}
	}
done:
	wg.Wait()
}

// AddImmediateListener 向引擎注册事件监听器，线程安全，可在运行时动态调用。
func (eng *Engine) AddImmediateListener(e *event.Event, handler HandlerFunc) {
	eng.mu.Lock()
	defer eng.mu.Unlock()
	name := e.GetName()
	eng.listeners[name] = append(eng.listeners[name], handler)
}

// Publish 向事件队列发布事件（非阻塞）。队列满时丢弃并打印警告。
func (eng *Engine) Publish(e *event.Event) {
	select {
	case eng.queue <- e:
	default:
		fmt.Printf("[engine] WARNING: queue full, event %q dropped\n", e.GetName())
	}
}

// PublishBlocking 向事件队列发布事件（阻塞直到入队或引擎关闭）。
func (eng *Engine) PublishBlocking(e *event.Event) {
	select {
	case eng.queue <- e:
	case <-eng.ctx.Done():
	}
}

// Stop 优雅停止引擎，等待所有正在执行的事件处理完成。
func (eng *Engine) Stop() {
	eng.cancel()
	eng.wg.Wait()
}

// QueueLen 返回当前队列待处理事件数。
func (eng *Engine) QueueLen() int { return len(eng.queue) }

// Workers 返回引擎 worker 数。
func (eng *Engine) Workers() int { return eng.workers }
