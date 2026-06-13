// Package SearchEngine 基于 tantivy-go 的全文搜索引擎封装。
//
// 使用前准备：
//  1. 从 https://winlibs.com/ 下载 GCC 16.1.0 (with POSIX threads) + MinGW-w64 14.0.0 (UCRT)
//  2. 将上面的压缩包解压到一个任意目录，并将mingw64/bin添加到path环境变量
//  3. 在项目目录中运行 go get github.com/anyproto/tantivy-go@v1.0.6
//  4. 从 https://github.com/anyproto/tantivy-go/releases 下载 windows-amd64.tar.gz并解压
//  5. 将 libtantivy_go.a 放入 mingw64\lib 目录
//  6. 编译: CGO_ENABLED=1 go build .
//
// 快速开始见本文件底部的 Example() 函数。
package SearchEngine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tantivygo "github.com/anyproto/tantivy-go"
)

// ============================================================================
// 常量 / 默认值
// ============================================================================

// 索引字段名。
const (
	FieldID      = "id"
	FieldTitle   = "title"
	FieldContent = "content"
)

// ============================================================================
// Engine —— 搜索引擎封装
// ============================================================================

// Engine 封装 tantivy 索引的创建、写入、搜索与关闭。
type Engine struct {
	ctx    *tantivygo.TantivyContext
	schema *tantivygo.Schema
	path   string
}

// New 初始化 tantivy 运行时、创建 schema 并在指定路径打开（或创建）索引。
//
//   - indexPath: 索引持久化目录，如 "./search_index"
//   - 首次调用会创建新索引；后续调用会打开已有索引
//   - 返回的 Engine 使用完毕后必须调用 Close()
func New(indexPath string) (*Engine, error) {
	if err := tantivygo.LibInit(true, true, "off"); err != nil {
		return nil, fmt.Errorf("LibInit: %w", err)
	}

	builder, err := tantivygo.NewSchemaBuilder()
	if err != nil {
		return nil, fmt.Errorf("NewSchemaBuilder: %w", err)
	}

	// title: 短文本，全文索引，存储原文，按位置记录
	if err := builder.AddTextField(FieldTitle, true, true, false,
		tantivygo.IndexRecordOptionWithFreqsAndPositions,
		tantivygo.TokenizerSimple); err != nil {
		return nil, fmt.Errorf("AddTextField(title): %w", err)
	}

	// content: 正文，全文索引，存储原文，按位置记录
	if err := builder.AddTextField(FieldContent, true, true, false,
		tantivygo.IndexRecordOptionWithFreqsAndPositions,
		tantivygo.TokenizerSimple); err != nil {
		return nil, fmt.Errorf("AddTextField(content): %w", err)
	}

	// id: 唯一标识，不索引，原样存储（raw tokenizer）
	if err := builder.AddTextField(FieldID, true, false, false,
		tantivygo.IndexRecordOptionBasic,
		tantivygo.TokenizerRaw); err != nil {
		return nil, fmt.Errorf("AddTextField(id): %w", err)
	}

	schema, err := builder.BuildSchema()
	if err != nil {
		return nil, fmt.Errorf("BuildSchema: %w", err)
	}

	ctx, err := tantivygo.NewTantivyContextWithSchema(indexPath, schema)
	if err != nil {
		return nil, fmt.Errorf("NewTantivyContextWithSchema: %w", err)
	}

	// 注册 tokenizer — 必须使用 schema 中引用的 tokenizer 名
	if err := ctx.RegisterTextAnalyzerSimple(tantivygo.TokenizerSimple, 500, tantivygo.English); err != nil {
		ctx.Close()
		return nil, fmt.Errorf("RegisterTextAnalyzerSimple: %w", err)
	}
	if err := ctx.RegisterTextAnalyzerRaw(tantivygo.TokenizerRaw); err != nil {
		ctx.Close()
		return nil, fmt.Errorf("RegisterTextAnalyzerRaw: %w", err)
	}

	return &Engine{ctx: ctx, schema: schema, path: indexPath}, nil
}

// Close 等待合并完成并释放所有资源。
func (e *Engine) Close() error {
	return e.ctx.Close()
}

// NumDocs 返回当前已索引的文档数。
func (e *Engine) NumDocs() (uint64, error) {
	return e.ctx.NumDocs()
}

// ============================================================================
// 文档操作
// ============================================================================

// Add 添加一篇文档。id/标题/正文会写入三个字段。
func (e *Engine) Add(id, title, content string) error {
	doc := tantivygo.NewDocument()
	if err := doc.AddField(id, e.ctx, FieldID); err != nil {
		return fmt.Errorf("AddField(id): %w", err)
	}
	if err := doc.AddField(title, e.ctx, FieldTitle); err != nil {
		return fmt.Errorf("AddField(title): %w", err)
	}
	if err := doc.AddField(content, e.ctx, FieldContent); err != nil {
		return fmt.Errorf("AddField(content): %w", err)
	}
	return e.ctx.AddAndConsumeDocuments(doc)
}

// Delete 按 ID 删除文档。
func (e *Engine) Delete(ids ...string) error {
	return e.ctx.DeleteDocuments(FieldID, ids...)
}

// ============================================================================
// 搜索
// ============================================================================

// Hit 是一条搜索结果。
type Hit struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// Search 在 title 和 content 字段中搜索，返回最多 limit 条结果。
func (e *Engine) Search(query string, limit uintptr) ([]Hit, error) {
	sCtx := tantivygo.NewSearchContextBuilder().
		SetQuery(query).
		SetDocsLimit(limit).
		SetWithHighlights(true).
		AddFieldDefaultWeight(FieldTitle).
		AddFieldDefaultWeight(FieldContent).
		Build()

	result, err := e.ctx.Search(sCtx)
	if err != nil {
		return nil, fmt.Errorf("Search: %w", err)
	}
	defer result.Free()

	size, err := result.GetSize()
	if err != nil {
		return nil, fmt.Errorf("GetSize: %w", err)
	}

	hits := make([]Hit, 0, size)
	for i := uint64(0); i < size; i++ {
		doc, err := result.Get(i)
		if err != nil {
			continue
		}
		jsonStr, err := doc.ToJson(e.ctx, FieldID, FieldTitle, FieldContent)
		doc.Free()
		if err != nil {
			continue
		}
		hits = append(hits, parseHit(jsonStr))
	}
	return hits, nil
}

// ============================================================================
// 文件索引工具
// ============================================================================

// IndexFiles 读取 dir 下所有 .txt 文件，以文件名为标题、文件内容为正文建立索引。
func (e *Engine) IndexFiles(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("ReadDir: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return count, fmt.Errorf("ReadFile(%s): %w", entry.Name(), err)
		}

		id := strings.TrimSuffix(entry.Name(), ".txt")
		title := id
		content := string(data)

		if err := e.Add(id, title, content); err != nil {
			return count, fmt.Errorf("Add(%s): %w", id, err)
		}
		count++
	}
	return count, nil
}

// ============================================================================
// 辅助
// ============================================================================

// parseHit 从 tantivy 返回的 JSON 中提取字段（简易解析，避免引入 encoding/json 依赖）。
// tantivy 返回格式示例：
//
//	{"id":"1","title":"Rust","content":"...","score":2.4,"highlights":[...]}
func parseHit(raw string) Hit {
	h := Hit{}
	h.ID = extractField(raw, "id")
	h.Title = extractField(raw, "title")
	h.Content = extractField(raw, "content")
	h.Score = parseScore(raw)
	return h
}

func extractField(raw, field string) string {
	key := `"` + field + `":"`
	start := strings.Index(raw, key)
	if start < 0 {
		return ""
	}
	start += len(key)
	end := strings.Index(raw[start:], `"`)
	if end < 0 {
		return ""
	}
	return raw[start : start+end]
}

func parseScore(raw string) float64 {
	key := `"score":`
	start := strings.Index(raw, key)
	if start < 0 {
		return 0
	}
	start += len(key)
	end := start
	for end < len(raw) && (raw[end] >= '0' && raw[end] <= '9' || raw[end] == '.' || raw[end] == '-' || raw[end] == 'e' || raw[end] == 'E') {
		end++
	}
	var f float64
	fmt.Sscanf(raw[start:end], "%f", &f)
	return f
}

// ============================================================================
// 完整示例
// ============================================================================

/*
Example 演示端到端流程:

 1. 初始化引擎
 2. 生成 5 个示例 .txt 文件
 3. 将文件内容建立索引
 4. 执行搜索并打印结果
 5. 清理
*/
func Example() {
	// ——— 1. 准备目录 ———
	dataDir := filepath.Join(".", "example-docs")
	indexDir := filepath.Join(".", "example-index")
	os.RemoveAll(dataDir)
	os.RemoveAll(indexDir)
	os.MkdirAll(dataDir, 0755)
	// ——— 2. 生成 5 个 .txt 文件 ———
	files := []struct{ name, content string }{
		{
			"Rust Programming Language.txt",
			"Rust is a systems programming language focused on safety, speed, and concurrency. " +
				"It achieves memory safety without a garbage collector using its ownership model. " +
				"The language is widely used for building high-performance distributed systems and web servers.",
		},
		{
			"Go Programming Language.txt",
			"Go is an open source programming language designed for simplicity. " +
				"It features goroutines for lightweight concurrency and a garbage collector for memory management. " +
				"Go is popular for cloud services, CLI tools, and networked applications.",
		},
		{
			"Full-Text Search with Tantivy.txt",
			"Tantivy is a full-text search engine library inspired by Apache Lucene and written in Rust. " +
				"It supports inverted indexes, BM25 relevance scoring, tokenizers for multiple languages, " +
				"and fast field queries. Tantivy is ideal for embedding search capabilities into applications.",
		},
		{
			"Building a Search Engine.txt",
			"Building a search engine involves tokenizing text into terms, creating an inverted index, " +
				"and applying relevance models like TF-IDF or BM25. Modern search engines also support " +
				"phrase queries, fuzzy matching, and faceted search. The inverted index maps each term " +
				"to the documents containing it, enabling sub-millisecond retrieval.",
		},
		{
			"Introduction to Distributed Systems.txt",
			"Distributed systems involve multiple computers working together as a single system. " +
				"Key challenges include network latency, fault tolerance, data consistency, and concurrency. " +
				"Technologies like Rust and Go are increasingly used to build reliable distributed infrastructure. " +
				"Search engines themselves are often implemented as distributed systems for scalability.",
		},
	}

	for _, f := range files {
		path := filepath.Join(dataDir, f.name)
		if err := os.WriteFile(path, []byte(f.content), 0644); err != nil {
			fmt.Printf("创建文件失败 %s: %v\n", f.name, err)
			return
		}
		fmt.Printf("  ✓ 创建文件: %s\n", f.name)
	}
	fmt.Printf("\n已生成 %d 个 .txt 文件: %s\n\n", len(files), dataDir)

	// ——— 3. 初始化搜索引擎 ———
	engine, err := New(indexDir)
	if err != nil {
		fmt.Printf("初始化引擎失败: %v\n", err)
		return
	}
	defer engine.Close()
	fmt.Println("✓ 搜索引擎初始化完成")

	// ——— 4. 将所有 .txt 文件索引进引擎 ———
	n, err := engine.IndexFiles(dataDir)
	if err != nil {
		fmt.Printf("索引文件失败: %v\n", err)
		return
	}
	fmt.Printf("✓ 已索引 %d 篇文档\n", n)

	docCount, _ := engine.NumDocs()
	fmt.Printf("✓ 索引中文档总数: %d\n\n", docCount)

	// ——— 5. 执行搜索 ———
	queries := []string{
		"programming language",
		"search engine",
		"concurrency",
		"BM25",
	}

	fmt.Println("═══════════════════════════════════════════════")
	fmt.Println("              搜索测试")
	fmt.Println("═══════════════════════════════════════════════")

	for _, q := range queries {
		fmt.Printf("\n🔍 搜索: \"%s\"\n", q)
		fmt.Println("───────────────────────────────────────────────")

		hits, err := engine.Search(q, 5)
		if err != nil {
			fmt.Printf("  搜索失败: %v\n", err)
			continue
		}

		if len(hits) == 0 {
			fmt.Println("  没有找到匹配的文档")
			continue
		}

		for i, h := range hits {
			fmt.Printf("  #%d  [score: %.4f]  %s\n", i+1, h.Score, h.Title)
			// 截取正文前 120 个字作为摘要
			snippet := h.Content
			if len(snippet) > 120 {
				snippet = snippet[:120] + "..."
			}
			fmt.Printf("      %s\n", snippet)
		}
	}

	// ——— 6. 删除操作示例 ———
	fmt.Println("\n═══════════════════════════════════════════════")
	fmt.Println("              删除操作")
	fmt.Println("═══════════════════════════════════════════════")
	fmt.Println()

	idToDelete := "Go Programming Language"
	fmt.Printf("删除前文档数: ")
	if c, err := engine.NumDocs(); err == nil {
		fmt.Printf("%d\n", c)
	}
	if err := engine.Delete(idToDelete); err != nil {
		fmt.Printf("删除失败: %v\n", err)
	} else {
		fmt.Printf("✓ 已删除: %s\n", idToDelete)
	}
	fmt.Printf("删除后文档数: ")
	if c, err := engine.NumDocs(); err == nil {
		fmt.Printf("%d\n", c)
	}

	fmt.Println("\n═══════════════════════════════════════════════")
	fmt.Println("              示例完成")
	fmt.Println("═══════════════════════════════════════════════")
}
