package UnitTest

import (
	"fmt"
	"runtime"
)

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

func caller(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "???:0"
	}
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
	loc := caller(3)
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
	if !isNil {
		detail := fmt.Sprintf("expected nil, got %v", v)
		if len(msg) > 0 {
			detail += " — " + fmt.Sprint(msg...)
		}
		t.log(false, detail)
		return
	}
	detail := "got nil"
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

// RunSuite 执行一组测试用例并输出统计。
type testCase struct {
	name string
	fn   func(*T)
}

func RunSuite(title string, tests []testCase) (passed, failed int) {
	fmt.Println("╔════════════════════════════════════════════════════╗")
	fmt.Printf("║  %-48s ║\n", title)
	fmt.Println("╚════════════════════════════════════════════════════╝")

	for _, tc := range tests {
		fmt.Printf("\n── %s ──\n", tc.name)
		t := NewT(tc.name)
		tc.fn(t)
		p, f := t.Summary()
		passed += p
		failed += f
	}

	fmt.Println("\n╔════════════════════════════════════════════════════╗")
	fmt.Printf("║  结果: %d 通过, %d 失败", passed, failed)
	if failed > 0 {
		fmt.Print("  ⚠ 存在失败用例！")
	} else {
		fmt.Print("  ✓ 全部通过！")
	}
	fmt.Println()
	fmt.Println("╚════════════════════════════════════════════════════╝")
	return
}
