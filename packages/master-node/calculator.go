package docgentest

// Add 返回两个整数的和。
// 不会发生溢出检查，直接返回 a+b 的结果。
func Add(a, b int) int {
	return a + b
}

// Subtract 返回两个整数的差（a - b）。
func Subtract(a, b int) int {
	return a - b
}

// Multiply 返回两个整数的乘积。
func Multiply(a, b int) int {
	return a * b
}

// Divide 安全地计算 a/b 的商。
// 如果 b 为 0，则返回 0 和 ErrDivideByZero 错误。
func Divide(a, b int) (int, error) {
	if b == 0 {
		return 0, ErrDivideByZero
	}
	return a / b, nil
}

// Calculator 是一个简单的计算器类型，可以保持一个累加值。
// 零值可以直接使用，累加值初始为 0。
type Calculator struct {
	value int
}

// NewCalculator 创建一个新的计算器，并可以传入一个初始值。1
func NewCalculator(initial int) *Calculator {
	return &Calculator{value: initial}
}

// Add 将 n 加到计算器的当前值上，并返回新的值。
func (c *Calculator) Add(n int) int {
	c.value += n
	return c.value
}

// Value 返回计算器当前的累加值。
func (c *Calculator) Value() int {
	return c.value
}
