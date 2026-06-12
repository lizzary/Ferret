// Package EventEngine 提供高性能并发事件引擎。
//
// # 架构概览
//
// EventEngine 是一个基于发布/订阅模式的事件分发系统，专为高并发场景设计。
// 其核心由以下组件构成：
//
//   - 事件队列（Event Queue）：带缓冲的 channel，发布者直接将事件投递到队列中，
//     天然提供背压控制，避免在高负载下耗尽系统资源。
//
//   - 监听器注册表（Listener Registry）：以事件名称为键的 map[string][]HandlerFunc，
//     使用 sync.RWMutex 保护，支持读多写少的并发访问模式。
//
//   - Worker 池（Worker Pool）：通过带缓冲的 semaphore channel 控制最大并发度，
//     确保同时执行的事件处理 goroutine 数量不超过预设的 workers 值。
//
//   - 引擎主循环（Main Loop）：独立 goroutine 持续从队列消费事件，
//     获取 worker 令牌后分派给对应的监听器执行，支持通过 context 实现优雅关闭。
//
// # 使用示例
//
//	// 创建引擎，4 个 worker，队列大小 8192
//	eng := EventEngine.New(4, 8192)
//	defer eng.Stop()
//
//	// 注册监听器
//	ev := Event.New("task.created")
//	eng.AddImmediateListener(ev, func(e *Event.Event) {
//	    fmt.Println("task created:", e.GetData())
//	})
//
//	// 发布事件
//	eng.Publish(Event.New("task.created", taskID))
package EventEngine

import (
	"compute-node/packages/Event"
	"context"
	"errors"
	"fmt"
	"sync"
)

// 引擎可能返回的错误。
var (
	// ErrQueueFull 表示事件队列已满，新事件被丢弃。
	// 通过 PublishTry 调用时可获取此错误以确认事件是否成功入队。
	ErrQueueFull = errors.New("event queue is full")

	// ErrEngineStopped 表示引擎已停止，无法继续处理事件。
	ErrEngineStopped = errors.New("engine is stopped")
)

// HandlerFunc 是事件监听器的回调函数签名。
//
// 每个注册到引擎的监听器必须符合此签名。
// 监听器在独立的 goroutine 中执行，如果发生 panic 会被引擎自动恢复，
// 不会影响其他监听器或引擎的正常运行。
type HandlerFunc func(e *Event.Event)

// EventEngine 是高性能并发事件引擎的核心类型。
//
// EventEngine 实现了发布/订阅模式，支持以下特性：
//   - 固定大小的 worker 池，限制最大并发度
//   - 有界事件队列，防止内存无限增长
//   - 线程安全的动态监听器注册
//   - 多种发布策略（非阻塞丢弃、阻塞等待、尝试发布）
//   - 优雅关闭，等待所有进行中的事件处理完成
//   - 监听器 panic 自动恢复，保障引擎稳定性
//
// EventEngine 通过 New 函数创建并自动启动，通过 Stop 方法安全关闭。
// 零值不可直接使用。
type EventEngine struct {
	workers   int
	queue     chan *Event.Event
	semaphore chan struct{} // 令牌 channel，容量等于 workers，控制并发度
	listeners map[string][]HandlerFunc
	mu        sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// defaultQueueSize 是队列的默认容量，当 New 未指定 queueSize 时使用。
const defaultQueueSize = 4096

// New 创建并启动一个事件引擎。
//
// workers 指定最大并发 worker 数量，必须为正整数。若传入 ≤0，自动取 1。
// queueSize 为可选参数，指定事件队列的缓冲大小。若不传或传入 ≤0，使用 defaultQueueSize（4096）。
//
// 引擎在创建时即启动主循环 goroutine，调用方应在不再需要引擎时调用 Stop 进行清理。
func New(workers int, queueSize ...int) *EventEngine {
	if workers <= 0 {
		workers = 1
	}
	qs := defaultQueueSize
	if len(queueSize) > 0 && queueSize[0] > 0 {
		qs = queueSize[0]
	}

	ctx, cancel := context.WithCancel(context.Background())
	eng := &EventEngine{
		workers:   workers,
		queue:     make(chan *Event.Event, qs),
		semaphore: make(chan struct{}, workers),
		listeners: make(map[string][]HandlerFunc),
		ctx:       ctx,
		cancel:    cancel,
	}
	for i := 0; i < workers; i++ {
		eng.semaphore <- struct{}{} // 预填充令牌
	}

	eng.wg.Add(1)
	go eng.run()
	return eng
}

// run 是引擎主循环，运行在独立的 goroutine 中。
//
// 主循环的工作流程：
//  1. 从 semaphore 获取一个令牌（阻塞直到有 worker 空闲）。
//  2. 令牌到手后，尝试从队列取出一个事件。
//  3. 启动新的 goroutine 执行 dispatch，执行完毕后归还令牌。
//  4. 收到 ctx.Done() 信号时退出循环，实现优雅关闭。
func (eng *EventEngine) run() {
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
				go func(ev *Event.Event, t struct{}) {
					defer eng.wg.Done()
					defer func() { eng.semaphore <- t }() // 完成后归还令牌
					eng.dispatch(ev)
				}(e, tok)
			case <-eng.ctx.Done():
				eng.semaphore <- tok // ctx 取消时归还令牌再退出
				return
			}
		}
	}
}

// dispatch 将事件分派给所有匹配的监听器。
//
// 该函数在调用方的 goroutine 中执行。它先通过读锁获取监听器列表的副本，
// 然后逐一调用。使用快照而非直接遍历是为了：
//   - 缩短持锁时间，减少锁竞争。
//   - 避免在回调执行期间阻塞 AddImmediateListener。
func (eng *EventEngine) dispatch(e *Event.Event) {
	eng.mu.RLock()
	hs := eng.listeners[e.GetName()]
	handlers := make([]HandlerFunc, len(hs))
	copy(handlers, hs)
	eng.mu.RUnlock()

	for _, h := range handlers {
		safeCall(h, e)
	}
}

// safeCall 在 defer/recover 保护下调用监听器。
//
// 如果监听器发生 panic，safeCall 会捕获并打印警告信息，
// 确保单个监听器的崩溃不会影响其他监听器的执行。
func safeCall(h HandlerFunc, e *Event.Event) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[engine] handler panic for Event %q: %v\n", e.GetName(), r)
		}
	}()
	h(e)
}

// AddImmediateListener 向引擎注册一个事件监听器。
//
// 该方法线程安全，可在引擎运行时动态调用。
// 注册的监听器将在匹配事件被 dispatch 时同步调用
// （在 worker goroutine 中执行，而非发布者 goroutine）。
//
// handler 应避免长时间阻塞，否则会占用 worker 资源。
// 如需执行耗时操作，建议在 handler 内部启动新的 goroutine。
func (eng *EventEngine) AddImmediateListener(e *Event.Event, handler HandlerFunc) {
	eng.mu.Lock()
	defer eng.mu.Unlock()
	name := e.GetName()
	eng.listeners[name] = append(eng.listeners[name], handler)
}

// Publish 以非阻塞方式向事件队列发布一个事件。
//
// 如果队列有空间，事件立即入队并返回；
// 如果队列已满，事件被静默丢弃，同时打印警告日志。
//
// 该方法是性能最优的发布路径，适合可容忍事件丢失的场景。
// 如需确保事件必须被处理，请使用 PublishBlocking。
// 如需获取入队失败的明确反馈，请使用 PublishTry。
func (eng *EventEngine) Publish(e *Event.Event) {
	select {
	case eng.queue <- e:
	default:
		fmt.Printf("[engine] WARNING: queue full, Event %q dropped\n", e.GetName())
	}
}

// PublishBlocking 以阻塞方式向事件队列发布一个事件。
//
// 调用方 goroutine 会阻塞直到事件成功入队，或引擎通过 Stop 关闭。
// 适合发布者需要确保事件不丢失的场景。
//
// 注意：如果引擎已停止，此方法静默返回（事件被丢弃）。
func (eng *EventEngine) PublishBlocking(e *Event.Event) {
	select {
	case eng.queue <- e:
	case <-eng.ctx.Done():
	}
}

// PublishTry 尝试向事件队列发布一个事件，不阻塞。
//
// 如果事件成功入队，返回 nil；
// 如果队列已满无法立即入队，返回 ErrQueueFull。
//
// 这是 Publish 的带错误反馈版本，适合调用方需要根据入队结果
// 执行不同逻辑的场景（如重试、降级处理或上报监控）。
func (eng *EventEngine) PublishTry(e *Event.Event) error {
	select {
	case eng.queue <- e:
		return nil
	default:
		return ErrQueueFull
	}
}

// Stop 优雅停止引擎。
//
// 它会：
//  1. 取消内部 context，通知主循环和所有阻塞的 PublishBlocking 调用退出。
//  2. 等待所有正在执行的监听器完成（通过 sync.WaitGroup 同步）。
//  3. 调用 Stop 后不应再向引擎发布事件，否则行为未定义。
func (eng *EventEngine) Stop() {
	eng.cancel()
	eng.wg.Wait()
}

// QueueLen 返回当前事件队列中待处理的事件数量。
//
// 可用于监控引擎负载。返回值仅是快照，调用后立即过时。
func (eng *EventEngine) QueueLen() int { return len(eng.queue) }

// Workers 返回引擎配置的 worker 数量。
func (eng *EventEngine) Workers() int { return eng.workers }
