package docgentest

import (
	"errors"
	"strings"
)

// Separator 定义了分割字符串时使用的分隔符类型。
// 它本质上是 string 的别名，但可以附加方法。
type Separator string

// Split 使用分隔符 sep 对 s 进行分割，返回分割后的子串切片。
// 如果 s 为空字符串，则返回空切片。
// 该方法永远不会返回 nil。
func (sep Separator) Split(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, string(sep))
}

// Splitter 是一个可定制的字符串分割器。
// 它的零值是一个以空白字符作为分隔符、最多进行 0 次分割（即全部分割）的分割器。
type Splitter struct {
	// Sep 是用于分割的分隔符。
	Sep string
	// N 是最大分割次数。
	// 如果 N < 0，则进行全部分割（等同于 N 为 0）。
	// 如果 N == 0，使用 strings.Split 的行为。
	// 如果 N > 0，最多返回 N 个子串。
	N int
}

// Split 使用接收者的配置对 s 进行分割。
func (s *Splitter) Split(str string) []string {
	sep := s.Sep
	if sep == "" {
		sep = " "
	}
	n := s.N
	if n < 0 {
		n = 0
	}
	return strings.SplitN(str, sep, n)
}

// Reverse 返回将 s 按 Unicode 码点反转后的字符串。
//
// 示例：
//
//	Reverse("hello") // "olleh"
//	Reverse("世界")  // "界世"
func Reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// WordCount 统计 s 中的单词数量。
// 单词是由空白字符分隔的非空序列。
// 多个连续空白字符视为单个分隔。
func WordCount(s string) int {
	words := strings.Fields(s)
	return len(words)
}

// Deprecated: Concat 使用原生的 + 连接字符串，
// 在大量连接时性能不佳，请使用 strings.Builder 代替。
func Concat(strs ...string) string {
	var result string
	for _, s := range strs {
		result += s
	}
	return result
}

var (
	// ErrDivideByZero 是除数为零时返回的错误。
	ErrDivideByZero = errors.New("division by zero")
)
