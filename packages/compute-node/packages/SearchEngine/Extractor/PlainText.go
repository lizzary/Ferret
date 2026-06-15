package Extractor

import (
	"fmt"
	"os"
)

// plainTextExtractor 直接读取文件内容，当作纯文本返回（UTF-8）
func plainTextExtractor(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file failed: %w", err)
	}
	return string(data), nil
}

// 所有可以直接按纯文本读取的后缀列表
var plainTextSuffixes = []string{
	// 纯文本 / 标记语言
	".txt", ".text", ".log", ".md", ".rst", ".tex",
	".html", ".htm", ".xml", ".json", ".yaml", ".yml", ".toml",
	".ini", ".cfg", ".conf", ".env",

	// 代码文件
	".go", ".py", ".js", ".ts", ".java", ".c", ".cpp", ".cc", ".cxx",
	".h", ".hpp", ".rs", ".rb", ".php", ".swift", ".kt", ".scala",
	".cs", ".fs", ".pl", ".pm", ".lua", ".r", ".m", ".mm", ".sql",
	".sh", ".bash", ".zsh", ".ps1", ".vba", ".vbs", ".groovy",
	".dart", ".erl", ".ex", ".exs", ".clj", ".jl", ".hs", ".ml",
	".scm", ".lisp", ".vim", ".tf",
}

func init() {
	for _, suffix := range plainTextSuffixes {
		register(suffix, plainTextExtractor)
	}
}
