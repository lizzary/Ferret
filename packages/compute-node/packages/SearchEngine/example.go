// Package SearchEngine 基于 tantivy-go 的全文搜索引擎封装。
//
// 使用前准备：
//  1. 在 ferret/compute-node 目录中运行 go get github.com/anyproto/tantivy-go
//  2. 从 https://winlibs.com/ 下载 GCC 16.1.0 (with POSIX threads) + MinGW-w64 14.0.0 (UCRT)
//  3. 将上面的压缩包解压到一个任意目录，并将 mingw64/bin 添加到 PATH 环境变量
//  4. 在 compute-node 项目根目录中运行 go get github.com/anyproto/tantivy-go@v1.0.6
//  5. 从 https://github.com/anyproto/tantivy-go/releases 下载 windows-amd64.tar.gz 并解压
//  6. 将 libtantivy_go.a 放入 mingw64\lib 目录
//  7. 编译: go build -o main.exe .\main.go
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
// 零值不可用，必须通过 New() 创建。
type Engine struct {
	ctx    *tantivygo.TantivyContext
	schema *tantivygo.Schema
	path   string

	// closed 标记是否已关闭，防止重复 Close 导致 CGo 双重释放（堆损坏 0xC0000374）。
	closed bool
}

// New 初始化 tantivy 运行时、创建 schema 并在指定路径打开（或创建）索引。
//
//   - indexPath: 索引持久化目录，如 "./search_index"
//   - 首次调用会创建新索引；后续调用会打开已有索引
//   - 返回的 Engine 使用完毕后必须调用 Close()
//   - LibInit 内部使用 sync.Once，多次调用 New() 是安全的
func New(indexPath string) (_ *Engine, err error) {
	// 1) 初始化 tantivy 原生库（sync.Once，可重复调用）
	if err := tantivygo.LibInit(true, true, "off"); err != nil {
		return nil, fmt.Errorf("SearchEngine.New: LibInit: %w", err)
	}

	// 2) 构建 schema
	builder, err := tantivygo.NewSchemaBuilder()
	if err != nil {
		return nil, fmt.Errorf("SearchEngine.New: NewSchemaBuilder: %w", err)
	}

	if err := builder.AddTextField(FieldTitle, true, true, false,
		tantivygo.IndexRecordOptionWithFreqsAndPositions,
		tantivygo.TokenizerSimple); err != nil {
		return nil, fmt.Errorf("SearchEngine.New: AddTextField(%s): %w", FieldTitle, err)
	}

	if err := builder.AddTextField(FieldContent, true, true, false,
		tantivygo.IndexRecordOptionWithFreqsAndPositions,
		tantivygo.TokenizerSimple); err != nil {
		return nil, fmt.Errorf("SearchEngine.New: AddTextField(%s): %w", FieldContent, err)
	}

	if err := builder.AddTextField(FieldID, true, false, false,
		tantivygo.IndexRecordOptionBasic,
		tantivygo.TokenizerRaw); err != nil {
		return nil, fmt.Errorf("SearchEngine.New: AddTextField(%s): %w", FieldID, err)
	}

	schema, err := builder.BuildSchema()
	if err != nil {
		return nil, fmt.Errorf("SearchEngine.New: BuildSchema: %w", err)
	}

	// 3) 创建/打开索引
	ctx, err := tantivygo.NewTantivyContextWithSchema(indexPath, schema)
	if err != nil {
		return nil, fmt.Errorf("SearchEngine.New: NewTantivyContextWithSchema(%s): %w", indexPath, err)
	}

	// ——— 从此处开始 ctx 已分配，出错必须 Close() ———
	// 使用 defer 捕获后续错误并安全关闭（避免 double-close 导致 0xC0000374）
	cleanup := true
	defer func() {
		if cleanup {
			if closeErr := ctx.Close(); closeErr != nil {
				// Close 失败时不能重复尝试，仅记录
				err = fmt.Errorf("%w; Close: %v", err, closeErr)
			}
		}
	}()

	// 4) 注册 tokenizer（必须与 schema 中引用的 tokenizer 名一致）
	if err := ctx.RegisterTextAnalyzerSimple(tantivygo.TokenizerSimple, 500, tantivygo.English); err != nil {
		return nil, fmt.Errorf("SearchEngine.New: RegisterTextAnalyzerSimple(%s): %w", tantivygo.TokenizerSimple, err)
	}

	if err := ctx.RegisterTextAnalyzerRaw(tantivygo.TokenizerRaw); err != nil {
		return nil, fmt.Errorf("SearchEngine.New: RegisterTextAnalyzerRaw(%s): %w", tantivygo.TokenizerRaw, err)
	}

	cleanup = false // 所有权转移给 Engine，不再由 defer 关闭
	return &Engine{ctx: ctx, schema: schema, path: indexPath}, nil
}

// Close 等待合并完成并释放所有资源。重复调用安全。
func (e *Engine) Close() error {
	if e == nil || e.closed {
		return nil
	}
	e.closed = true
	if e.ctx == nil {
		return nil
	}
	return e.ctx.Close()
}

// NumDocs 返回当前已索引的文档数。
// 调用前确保 Engine 已通过 New() 成功创建。
func (e *Engine) NumDocs() (uint64, error) {
	if e == nil || e.ctx == nil {
		return 0, fmt.Errorf("SearchEngine.NumDocs: Engine 未初始化")
	}
	n, err := e.ctx.NumDocs()
	if err != nil {
		return 0, fmt.Errorf("SearchEngine.NumDocs: %w", err)
	}
	return n, nil
}

// ============================================================================
// 文档操作
// ============================================================================

// Add 添加一篇文档。id/标题/正文会写入三个字段。
func (e *Engine) Add(id, title, content string) error {
	if e == nil || e.ctx == nil {
		return fmt.Errorf("SearchEngine.Add: Engine 未初始化")
	}
	if id == "" {
		return fmt.Errorf("SearchEngine.Add: id 不能为空")
	}

	doc := tantivygo.NewDocument()
	if doc == nil {
		return fmt.Errorf("SearchEngine.Add: NewDocument 返回 nil")
	}

	// 任一 AddField 失败时释放 doc 防止内存泄漏
	var added bool
	defer func() {
		if !added {
			doc.Free()
		}
	}()

	if err := doc.AddField(id, e.ctx, FieldID); err != nil {
		return fmt.Errorf("SearchEngine.Add: doc.AddField(%s, %s): %w", id, FieldID, err)
	}
	if err := doc.AddField(title, e.ctx, FieldTitle); err != nil {
		return fmt.Errorf("SearchEngine.Add: doc.AddField(%s, %s): %w", title, FieldTitle, err)
	}
	if err := doc.AddField(content, e.ctx, FieldContent); err != nil {
		return fmt.Errorf("SearchEngine.Add: doc.AddField(%s): %w", FieldContent, err)
	}

	if err := e.ctx.AddAndConsumeDocuments(doc); err != nil {
		return fmt.Errorf("SearchEngine.Add: AddAndConsumeDocuments(%s): %w", id, err)
	}
	added = true // doc 已被 tantivy 接管，不要 Free
	return nil
}

// Delete 按 ID 删除文档。
func (e *Engine) Delete(ids ...string) error {
	if e == nil || e.ctx == nil {
		return fmt.Errorf("SearchEngine.Delete: Engine 未初始化")
	}
	if len(ids) == 0 {
		return nil
	}
	if err := e.ctx.DeleteDocuments(FieldID, ids...); err != nil {
		return fmt.Errorf("SearchEngine.Delete: DeleteDocuments(%v): %w", ids, err)
	}
	return nil
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
func (e *Engine) Search(query string, limit uintptr) (hits []Hit, err error) {
	if e == nil || e.ctx == nil {
		return nil, fmt.Errorf("SearchEngine.Search: Engine 未初始化")
	}
	if query == "" {
		return nil, fmt.Errorf("SearchEngine.Search: query 不能为空")
	}

	sCtx := tantivygo.NewSearchContextBuilder().
		SetQuery(query).
		SetDocsLimit(limit).
		SetWithHighlights(true).
		AddFieldDefaultWeight(FieldTitle).
		AddFieldDefaultWeight(FieldContent).
		Build()

	result, err := e.ctx.Search(sCtx)
	if err != nil {
		return nil, fmt.Errorf("SearchEngine.Search: ctx.Search(%q): %w", query, err)
	}
	defer result.Free()

	size, err := result.GetSize()
	if err != nil {
		return nil, fmt.Errorf("SearchEngine.Search: result.GetSize: %w", err)
	}

	hits = make([]Hit, 0, size)
	for i := uint64(0); i < size; i++ {
		doc, err := result.Get(i)
		if err != nil {
			// 单条文档获取失败不中断全部结果，但记录日志
			fmt.Fprintf(os.Stderr, "[WARN] SearchEngine.Search: result.Get(%d): %v\n", i, err)
			continue
		}
		if doc == nil {
			fmt.Fprintf(os.Stderr, "[WARN] SearchEngine.Search: result.Get(%d) 返回 nil 文档\n", i)
			continue
		}

		jsonStr, err := doc.ToJson(e.ctx, FieldID, FieldTitle, FieldContent)
		// doc 使用完毕立即释放（无论 ToJson 成功与否）
		doc.Free()

		if err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] SearchEngine.Search: doc.ToJson(%d): %v\n", i, err)
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
// 返回成功索引的文件数。
func (e *Engine) IndexFiles(dir string) (int, error) {
	if e == nil || e.ctx == nil {
		return 0, fmt.Errorf("SearchEngine.IndexFiles: Engine 未初始化")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("SearchEngine.IndexFiles: ReadDir(%s): %w", dir, err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			return count, fmt.Errorf("SearchEngine.IndexFiles: ReadFile(%s): %w", filePath, err)
		}

		id := strings.TrimSuffix(entry.Name(), ".txt")
		title := id
		content := string(data)

		if err := e.Add(id, title, content); err != nil {
			return count, fmt.Errorf("SearchEngine.IndexFiles: Add(%s): %w", id, err)
		}
		count++
	}
	return count, nil
}

// ============================================================================
// 辅助（简易 JSON 解析，避免引入 encoding/json 依赖）
// ============================================================================

// parseHit 从 tantivy 返回的 JSON 中提取字段。
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
 5. 删除与统计
*/
func Example() {
	// ——— 1. 准备目录 ———
	dataDir := filepath.Join(".", "example-docs")
	indexDir := filepath.Join(".", "example-index")

	// 清理旧数据（忽略错误：目录可能不存在）
	_ = os.RemoveAll(dataDir)
	_ = os.RemoveAll(indexDir)

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] 创建目录 %s 失败: %v\n", dataDir, err)
		return
	}

	// ——— 2. 生成 5 个 .txt 文件 ———
	type docFile struct{ name, content string }
	files := []docFile{
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

	for i := range files {
		path := filepath.Join(dataDir, files[i].name)
		if err := os.WriteFile(path, []byte(files[i].content), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] 创建文件失败 %s: %v\n", files[i].name, err)
			return
		}
		fmt.Printf("  ✓ 创建文件: %s\n", files[i].name)
	}
	fmt.Printf("\n已生成 %d 个 .txt 文件: %s\n\n", len(files), dataDir)

	// ——— 3. 初始化搜索引擎 ———
	engine, err := New(indexDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] 初始化搜索引擎失败: %v\n", err)
		return
	}
	defer func() {
		if closeErr := engine.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "[WARN] 引擎关闭时出错: %v\n", closeErr)
		}
	}()
	fmt.Println("✓ 搜索引擎初始化完成")

	// ——— 4. 将所有 .txt 文件索引进引擎 ———
	n, err := engine.IndexFiles(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] 索引文件失败: %v\n", err)
		return
	}
	fmt.Printf("✓ 已索引 %d 篇文档\n", n)

	docCount, err := engine.NumDocs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] 获取文档数失败: %v\n", err)
		return
	}
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
		fmt.Printf("\n搜索: %q\n", q)
		fmt.Println("───────────────────────────────────────────────")

		hits, err := engine.Search(q, 5)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] 搜索失败 %q: %v\n", q, err)
			continue
		}

		if len(hits) == 0 {
			fmt.Println("  (无匹配结果)")
			continue
		}

		for i, h := range hits {
			fmt.Printf("  #%d  [score: %.4f]  %s\n", i+1, h.Score, h.Title)
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

	idToDelete := "Go Programming Language"
	fmt.Printf("\n删除前文档数: ")
	if c, err := engine.NumDocs(); err != nil {
		fmt.Fprintf(os.Stderr, "\n[ERROR] NumDocs: %v\n", err)
	} else {
		fmt.Printf("%d\n", c)
	}

	if err := engine.Delete(idToDelete); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] 删除失败: %v\n", err)
	} else {
		fmt.Printf("✓ 已删除: %s\n", idToDelete)
	}

	fmt.Printf("删除后文档数: ")
	if c, err := engine.NumDocs(); err != nil {
		fmt.Fprintf(os.Stderr, "\n[ERROR] NumDocs: %v\n", err)
	} else {
		fmt.Printf("%d\n", c)
	}

	fmt.Println("\n═══════════════════════════════════════════════")
	fmt.Println("              示例完成")
	fmt.Println("═══════════════════════════════════════════════")
}
