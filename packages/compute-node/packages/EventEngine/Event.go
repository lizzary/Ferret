// Package EventEngine 提供高性能事件引擎中使用的核心事件类型。
//
// Event 是一个不可变的值对象，封装了事件名称和可选载荷数据。
// 事件一旦通过 NewEvent 创建，其 name 和 data 便不可修改，只能通过
// GetName 和 GetData 方法读取。这种设计确保了在并发场景下
// 多个 goroutine 可以安全地共享同一个 Event 实例，无需额外同步。
//
// 典型用法：
//
//	ev := EventEngine.NewEvent("user.created", user)
//	engine.Publish(ev)
package EventEngine

import "fmt"

// Event 表示一个可发布到事件引擎的事件对象。
//
// 每个 Event 由两部分组成：
//   - name：事件的唯一标识字符串，用于将事件路由到对应的监听器。
//   - data：事件携带的可选载荷，可以是任意 Go 值。传 nil 表示无数据。
//
// Event 的零值不可直接使用，请始终通过 NewEvent 函数创建实例。
type Event struct {
	name string
	data any
}

// NewEvent 创建一个新的 Event 实例。
//
// name 为必填参数，用于标识事件类型，在事件路由和 Equals 比较中使用。
// data 为可选参数，传入一个值作为事件载荷；不传或传 nil 表示该事件不携带数据。
//
// 示例：
//
//	click := EventEngine.NewEvent("ui.button.click")
//	login := EventEngine.NewEvent("user.login", userID)
func NewEvent(name string, data ...any) *Event {
	e := &Event{name: name}
	if len(data) > 0 {
		e.data = data[0]
	}
	return e
}

// GetName 返回事件的名称字符串。
//
// 名称由 NewEvent 创建时指定，在 Event 的整个生命周期内保持不变。
func (e *Event) GetName() string {
	return e.name
}

// GetData 返回事件携带的数据。
//
// 如果事件创建时未提供 data 参数或传入了 nil，则返回 nil。
// 调用者应自行对返回值进行类型断言以获取原始类型。
func (e *Event) GetData() any {
	return e.data
}

// Equals 判断当前事件与另一个事件是否相等。
//
// 相等性仅基于事件名称比较，不比较 data 字段。
// 若 other 为 nil，始终返回 false。
//
// 这是 Go 惯用的相等性判断方法（Go 不支持运算符重载）。
func (e *Event) Equals(other *Event) bool {
	if other == nil {
		return false
	}
	return e.name == other.name
}

// String 返回事件的字符串表示，格式为 Event{name="...", data=...}。
//
// 该方法实现了 fmt.Stringer 接口，便于调试和日志输出。
func (e *Event) String() string {
	return fmt.Sprintf("Event{name=%q, data=%v}", e.name, e.data)
}
