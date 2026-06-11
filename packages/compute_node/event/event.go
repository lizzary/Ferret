// Package event 提供高性能事件引擎的核心事件类型。
package event

import "fmt"

// Event 表示一个可发布到事件引擎的事件对象。
// name 和 data 均为私有属性，只能通过方法读取。
type Event struct {
	name string
	data any
}

// New 创建一个新的 Event，name 为必填，data 为可选（传 nil 表示无数据）。
func New(name string, data ...any) *Event {
	e := &Event{name: name}
	if len(data) > 0 {
		e.data = data[0]
	}
	return e
}

// GetName 返回事件名称。
func (e *Event) GetName() string {
	return e.name
}

// GetData 返回事件携带的数据（可能为 nil）。
func (e *Event) GetData() any {
	return e.data
}

// Equals 判断两个事件是否相等（仅比较 name）。
// Go 不支持运算符重载，Equals 是惯用替代方案。
func (e *Event) Equals(other *Event) bool {
	if other == nil {
		return false
	}
	return e.name == other.name
}

// String 实现 fmt.Stringer，返回 name 和 data 的可读描述。
func (e *Event) String() string {
	return fmt.Sprintf("Event{name=%q, data=%v}", e.name, e.data)
}
