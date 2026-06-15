package Extractor

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// Extractor 定义提取函数类型
type Extractor func(filePath string) (string, error)

var (
	extractorMap map[string]Extractor
	once         sync.Once
)

// register 注册一个文件后缀对应的提取器（线程安全，可在多个 init 中调用）
func register(suffix string, ext Extractor) {
	once.Do(func() {
		extractorMap = make(map[string]Extractor)
	})
	extractorMap[strings.ToLower(suffix)] = ext
}

// Extract 根据文件路径提取文本内容。
// 若后缀已注册，调用对应的提取函数；否则返回错误。
func Extract(filePath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == "" {
		// 处理无后缀的特殊文件（如 Dockerfile, Makefile）
		// 为了简单，我们返回错误，要求调用方显式处理无后缀文件。
		return "", fmt.Errorf("unsupported file type: no file extension")
	}

	if extractorMap == nil {
		// 防御：如果没有注册任何内容，尝试初始化（理论上不会发生）
		once.Do(func() {
			extractorMap = make(map[string]Extractor)
		})
	}

	extractor, ok := extractorMap[ext]
	if !ok {
		return "", fmt.Errorf("unsupported file type: %s", ext)
	}
	return extractor(filePath)
}

// GetSupportedExtensions 返回所有已注册的后缀（可选，用于调试）
func GetSupportedExtensions() []string {
	if extractorMap == nil {
		return nil
	}
	exts := make([]string, 0, len(extractorMap))
	for k := range extractorMap {
		exts = append(exts, k)
	}
	return exts
}
