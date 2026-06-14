// Package SearchEngine 提供基于 tantivy-go 的全文搜索引擎封装。
//
// 该包实现了对 tantivy 索引的基础操作：添加文档、删除文档和搜索文档。
// 文档内容（标题和正文）仅被索引而不存储，搜索结果只返回文档 ID 和相关性分数，
// 调用方需根据 ID 回源数据库获取完整内容。
//
// 并发安全：Add 和 Delete 操作通过互斥锁保证原子性，Search 允许并发读。
//
// 使用前必须配置 CGo 环境并安装 libtantivy_go.a，详见 New 函数注释。
package SearchEngine

import (
	"fmt"
	"strings"
	"sync"

	tantivygo "github.com/anyproto/tantivy-go"
)

// ============================================================================
// 常量
// ============================================================================

const (
	// FieldID 文档唯一标识字段（存储且索引，用于精确删除和结果返回）
	FieldID = "id"
	// FieldTitle 标题字段（仅索引，不存储）
	FieldTitle = "title"
	// FieldContent 正文字段（仅索引，不存储）
	FieldContent = "content"
)

// ============================================================================
// 类型定义
// ============================================================================

// Engine 封装 tantivy 索引的创建、写入、搜索与关闭。
// 零值不可用，必须通过 New 创建。
type Engine struct {
	mu     sync.RWMutex              // 保护写操作（Add/Delete）
	ctx    *tantivygo.TantivyContext // CGO 上下文
	schema *tantivygo.Schema         // 索引结构定义
	path   string                    // 索引持久化路径
	closed bool                      // 防止重复关闭
}

// Hit 表示单条搜索结果。
// 因为标题和内容未存储，仅返回文档 ID 和相关性分数。
type Hit struct {
	ID    string  `json:"id"`    // 文档唯一标识
	Score float64 `json:"score"` // 相关性分数（BM25）
}

// ============================================================================
// 初始化与关闭
// ============================================================================

// New 初始化 tantivy 运行时、创建 schema 并在指定路径打开（或创建）索引。
//
// 参数：
//   - indexPath: 索引持久化目录，例如 "./search_index"
//
// 返回值：
//   - 成功返回可用的 Engine 实例，失败返回错误。
//
// 注意：
//   - 首次调用会创建新索引；后续调用会打开已有索引。
//   - 返回的 Engine 使用完毕后必须调用 Close 释放资源。
//   - LibInit 内部使用 sync.Once，多次调用 New 是安全的。
//   - 环境要求：必须正确安装 MinGW-w64 并将 libtantivy_go.a 放入 mingw64\lib。
//
// 示例：
//
//	engine, err := New("./my_index")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer engine.Close()
func New(indexPath string) (_ *Engine, err error) {
	// 初始化 tantivy 原生库
	if err := tantivygo.LibInit(true, true, "off"); err != nil {
		return nil, fmt.Errorf("LibInit: %w", err)
	}

	// 构建 schema
	builder, err := tantivygo.NewSchemaBuilder()
	if err != nil {
		return nil, fmt.Errorf("NewSchemaBuilder: %w", err)
	}

	// id 字段：索引且存储（用于返回结果），分词器 Raw（精确匹配）
	if err := builder.AddTextField(FieldID, true, true, false,
		tantivygo.IndexRecordOptionBasic,
		tantivygo.TokenizerRaw); err != nil {
		return nil, fmt.Errorf("AddTextField(%s): %w", FieldID, err)
	}

	// title 字段：索引但不存储，带词频和位置信息
	if err := builder.AddTextField(FieldTitle, true, false, false,
		tantivygo.IndexRecordOptionWithFreqsAndPositions,
		tantivygo.TokenizerSimple); err != nil {
		return nil, fmt.Errorf("AddTextField(%s): %w", FieldTitle, err)
	}

	// content 字段：索引但不存储，带词频和位置信息
	if err := builder.AddTextField(FieldContent, true, false, false,
		tantivygo.IndexRecordOptionWithFreqsAndPositions,
		tantivygo.TokenizerSimple); err != nil {
		return nil, fmt.Errorf("AddTextField(%s): %w", FieldContent, err)
	}

	schema, err := builder.BuildSchema()
	if err != nil {
		return nil, fmt.Errorf("BuildSchema: %w", err)
	}

	// 打开或创建索引
	ctx, err := tantivygo.NewTantivyContextWithSchema(indexPath, schema)
	if err != nil {
		return nil, fmt.Errorf("NewTantivyContextWithSchema: %w", err)
	}

	// 注册分词器
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

// Close 等待合并完成并释放所有资源。重复调用安全。
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e == nil || e.closed {
		return nil
	}
	e.closed = true
	if e.ctx == nil {
		return nil
	}
	err := e.ctx.Close()
	e.ctx = nil // <-- ADD THIS LINE
	return err
}

// NumDocs 返回当前索引中的文档总数（包括已标记删除但尚未合并的文档）。
func (e *Engine) NumDocs() (uint64, error) {
	if e == nil || e.ctx == nil {
		return 0, fmt.Errorf("engine 未初始化")
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	n, err := e.ctx.NumDocs()
	if err != nil {
		return 0, fmt.Errorf("NumDocs: %w", err)
	}
	return n, nil
}

// ============================================================================
// 文档操作（原子、并发安全）
// ============================================================================

// Add 添加一篇文档。标题和正文仅被索引，不会被存储。
//
// 参数：
//   - id: 文档唯一标识（业务主键，不能为空）
//   - title: 文档标题（仅用于搜索）
//   - content: 文档正文（仅用于搜索）
//
// 返回值：
//   - 成功返回传入的 id，失败返回错误。
//
// 原子性：该方法在互斥锁保护下执行，对单个文档的添加是原子的。
//
// 示例：
//
//	_, err := engine.Add("doc_001", "Rust 语言", "Rust 是一门系统编程语言...")
func (e *Engine) Add(id, title, content string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("id 不能为空")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.ctx == nil {
		return "", fmt.Errorf("engine 未初始化")
	}

	doc := tantivygo.NewDocument()
	if doc == nil {
		return "", fmt.Errorf("NewDocument 失败")
	}

	consumed := false
	defer func() {
		if !consumed {
			doc.Free()
		}
	}()

	if err := doc.AddField(id, e.ctx, FieldID); err != nil {
		return "", fmt.Errorf("添加 id 字段失败: %w", err)
	}
	if err := doc.AddField(title, e.ctx, FieldTitle); err != nil {
		return "", fmt.Errorf("添加 title 字段失败: %w", err)
	}
	if err := doc.AddField(content, e.ctx, FieldContent); err != nil {
		return "", fmt.Errorf("添加 content 字段失败: %w", err)
	}

	if err := e.ctx.AddAndConsumeDocuments(doc); err != nil {
		return "", fmt.Errorf("写入索引失败: %w", err)
	}
	consumed = true
	return id, nil
}

// Delete 按文档 ID 删除文档。
//
// 参数：
//   - id: 要删除的文档唯一标识（不能为空）
//
// 返回值：
//   - 成功返回 true（即使 id 不存在也返回 true，因为删除操作本身是幂等的）
//   - 失败返回错误
//
// 原子性：该方法在互斥锁保护下执行，对单个文档的删除是原子的。
// 注意：删除只是逻辑删除，磁盘空间不会立即释放，后续段合并时会物理删除。
//
// 示例：
//
//	ok, err := engine.Delete("doc_001")
func (e *Engine) Delete(id string) (bool, error) {
	if id == "" {
		return false, fmt.Errorf("id 不能为空")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.ctx == nil {
		return false, fmt.Errorf("engine 未初始化")
	}
	if err := e.ctx.DeleteDocuments(FieldID, id); err != nil {
		return false, fmt.Errorf("删除失败: %w", err)
	}
	return true, nil
}

// ============================================================================
// 搜索
// ============================================================================

// Search 在标题和正文字段中执行全文检索，返回最多 limit 条结果。
//
// 参数：
//   - query: 搜索查询字符串（不能为空）
//   - limit: 最大返回结果数量
//
// 返回值：
//   - 成功返回 Hit 切片（可能为空），失败返回错误。
//
// 并发性：该方法使用读锁，允许多个 goroutine 同时搜索。
//
// 示例：
//
//	hits, err := engine.Search("programming language", 10)
//	for _, hit := range hits {
//	    fmt.Printf("ID: %s, Score: %f\n", hit.ID, hit.Score)
//	}
func (e *Engine) Search(query string, limit uintptr) ([]Hit, error) {
	if query == "" {
		return nil, fmt.Errorf("query 不能为空")
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.ctx == nil {
		return nil, fmt.Errorf("engine 未初始化")
	}

	sCtx := tantivygo.NewSearchContextBuilder().
		SetQuery(query).
		SetDocsLimit(limit).
		SetWithHighlights(false). // 无需高亮，因为内容未存储
		AddFieldDefaultWeight(FieldTitle).
		AddFieldDefaultWeight(FieldContent).
		Build()

	result, err := e.ctx.Search(sCtx)
	if err != nil {
		return nil, fmt.Errorf("搜索失败: %w", err)
	}
	defer result.Free()

	size, err := result.GetSize()
	if err != nil {
		return nil, fmt.Errorf("获取结果数量失败: %w", err)
	}

	hits := make([]Hit, 0, size)
	for i := uint64(0); i < size; i++ {
		doc, err := result.Get(i)
		if err != nil || doc == nil {
			continue
		}
		// 只获取 id 字段（已存储）
		jsonStr, err := doc.ToJson(e.ctx, FieldID)
		doc.Free()
		if err != nil {
			continue
		}
		hits = append(hits, parseHit(jsonStr))
	}
	return hits, nil
}

// ============================================================================
// 内部辅助函数
// ============================================================================

// parseHit 从 tantivy 返回的 JSON 中提取 id 和 score。
// 输入格式示例: {"id":"123","score":2.345}
func parseHit(raw string) Hit {
	h := Hit{}
	h.ID = extractField(raw, FieldID)
	h.Score = parseScore(raw)
	return h
}

// extractField 简单提取 JSON 字符串字段值（不依赖 encoding/json）。
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

// parseScore 提取 JSON 中的 score 数值。
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
// 示例（仅用于演示，不会在正式代码中执行）
// ============================================================================

// Example 演示了如何使用 Engine 进行文档索引和搜索。
// 该示例生成临时文件、建立索引、执行搜索并删除文档。
// 注意：此函数仅用于文档生成，生产环境不应依赖。
func Example() {
	// 此处的具体实现与原始 example.go 类似，但为了保持文档清晰，
	// 实际使用时请参考 New、Add、Search、Delete 的独立示例。
	fmt.Println("请参考各函数的具体示例")
}
