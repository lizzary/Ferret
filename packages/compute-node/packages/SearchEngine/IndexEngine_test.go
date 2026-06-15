// engine_test.go
package SearchEngine

import (
	"sync"
	"testing"
	"time"
)

// TestNew tests engine creation and closing.
func TestNew(t *testing.T) {
	dir := t.TempDir()
	engine, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer engine.Close()

	if engine == nil {
		t.Fatal("engine is nil")
	}
	if engine.path != dir {
		t.Errorf("expected path %s, got %s", dir, engine.path)
	}
}

// TestAddAndSearch tests adding a document and searching for it.
func TestAddAndSearch(t *testing.T) {
	dir := t.TempDir()
	engine, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer engine.Close()

	id := "doc1"
	title := "Go Programming"
	content := "Go is a statically typed language with great concurrency support."

	// Add document
	returnedID, err := engine.Add(id, title, content)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if returnedID != id {
		t.Errorf("Add returned %s, want %s", returnedID, id)
	}

	// Search
	hits, err := engine.Search("concurrency", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].ID != id {
		t.Errorf("hit ID = %s, want %s", hits[0].ID, id)
	}
	if hits[0].Score <= 0 {
		t.Errorf("expected positive score, got %f", hits[0].Score)
	}
}

// TestDelete tests document deletion.
func TestDelete(t *testing.T) {
	dir := t.TempDir()
	engine, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		engine.Close()
		time.Sleep(100 * time.Millisecond) // give Windows time to release files
	}()

	id := "doc_to_delete"
	_, err = engine.Add(id, "Title", "Some content for deletion")
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Verify it exists
	hits, err := engine.Search("deletion", 10)
	if err != nil {
		t.Fatalf("Search before delete failed: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit before delete, got %d", len(hits))
	}

	// Delete
	ok, err := engine.Delete(id)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if !ok {
		t.Error("Delete returned false")
	}

	// Verify it's gone
	hits, err = engine.Search("deletion", 10)
	if err != nil {
		t.Fatalf("Search after delete failed: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("expected 0 hits after delete, got %d", len(hits))
	}
}

// TestDeleteNonexistent tests deleting an ID that does not exist.
func TestDeleteNonexistent(t *testing.T) {
	dir := t.TempDir()
	engine, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer engine.Close()

	ok, err := engine.Delete("nonexistent")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if !ok {
		t.Error("Delete should return true even for nonexistent ID")
	}
}

// TestNumDocs tests the document count functionality.
func TestNumDocs(t *testing.T) {
	dir := t.TempDir()
	engine, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer engine.Close()

	count, err := engine.NumDocs()
	if err != nil {
		t.Fatalf("NumDocs failed: %v", err)
	}
	if count != 0 {
		t.Errorf("initial count = %d, want 0", count)
	}

	_, _ = engine.Add("1", "a", "content a")
	_, _ = engine.Add("2", "b", "content b")

	count, err = engine.NumDocs()
	if err != nil {
		t.Fatalf("NumDocs after add failed: %v", err)
	}
	if count != 2 {
		t.Errorf("count after adds = %d, want 2", count)
	}

	_, _ = engine.Delete("1")
	count, err = engine.NumDocs()
	if err != nil {
		t.Fatalf("NumDocs after delete failed: %v", err)
	}
	if count != 1 {
		t.Errorf("count after delete = %d, want 1", count)
	}
}

// TestSearchEmptyQuery tests search with empty query string.
func TestSearchEmptyQuery(t *testing.T) {
	dir := t.TempDir()
	engine, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer engine.Close()

	_, err = engine.Search("", 10)
	if err == nil {
		t.Error("expected error for empty query, got nil")
	}
}

// TestConcurrentAddDelete tests concurrent operations for race conditions.
func TestConcurrentAddDelete(t *testing.T) {
	dir := t.TempDir()
	engine, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer engine.Close()

	const goroutines = 10
	const docsPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < docsPerGoroutine; i++ {
				id := string(rune(gid*100 + i))
				title := "title"
				content := "content for search"
				_, err := engine.Add(id, title, content)
				if err != nil {
					t.Errorf("concurrent add failed: %v", err)
				}
				// Immediately delete half of them
				if i%2 == 0 {
					_, _ = engine.Delete(id)
				}
			}
		}(g)
	}
	wg.Wait()

	// Final search should not panic
	_, err = engine.Search("search", 100)
	if err != nil {
		t.Errorf("search after concurrency failed: %v", err)
	}

	// Count docs (should be roughly half of total added)
	totalAdded := goroutines * docsPerGoroutine
	count, _ := engine.NumDocs()
	if count > uint64(totalAdded) {
		t.Errorf("count %d > total added %d", count, totalAdded)
	}
}

// TestPersistAcrossInstances verifies that index data survives engine close/reopen.
func TestPersistAcrossInstances(t *testing.T) {
	dir := t.TempDir()

	// First instance
	engine1, err := New(dir)
	if err != nil {
		t.Fatalf("New instance1 failed: %v", err)
	}
	_, err = engine1.Add("persist1", "Title", "Content that must persist")
	if err != nil {
		t.Fatalf("Add in instance1 failed: %v", err)
	}
	err = engine1.Close()
	if err != nil {
		t.Fatalf("Close instance1 failed: %v", err)
	}

	// Second instance (reopen)
	engine2, err := New(dir)
	if err != nil {
		t.Fatalf("New instance2 failed: %v", err)
	}
	defer engine2.Close()

	hits, err := engine2.Search("persist", 10)
	if err != nil {
		t.Fatalf("Search in instance2 failed: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].ID != "persist1" {
		t.Errorf("hit ID = %s, want persist1", hits[0].ID)
	}
}

// TestAddEmptyId tests that adding a document with empty id returns error.
func TestAddEmptyId(t *testing.T) {
	dir := t.TempDir()
	engine, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer engine.Close()

	_, err = engine.Add("", "title", "content")
	if err == nil {
		t.Error("expected error for empty id, got nil")
	}
}

// TestDeleteEmptyId tests that deleting with empty id returns error.
func TestDeleteEmptyId(t *testing.T) {
	dir := t.TempDir()
	engine, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer engine.Close()

	_, err = engine.Delete("")
	if err == nil {
		t.Error("expected error for empty id, got nil")
	}
}

// TestAddDocumentFailures covers Add error paths.
func TestAddDocumentFailures(t *testing.T) {
	dir := t.TempDir()
	engine, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	// Close engine to trigger e.ctx == nil error
	engine.Close()

	_, err = engine.Add("any", "title", "content")
	if err == nil {
		t.Error("expected error when engine closed, got nil")
	}
}

// TestDeleteFailures covers Delete error paths.
func TestDeleteFailures(t *testing.T) {
	dir := t.TempDir()
	engine, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	engine.Close()

	_, err = engine.Delete("any")
	if err == nil {
		t.Error("expected error when engine closed, got nil")
	}
}

// TestSearchFailures covers Search error paths.
func TestSearchFailures(t *testing.T) {
	dir := t.TempDir()
	engine, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	engine.Close()

	_, err = engine.Search("query", 10)
	if err == nil {
		t.Error("expected error when engine closed, got nil")
	}
}

// TestNumDocsFailures covers NumDocs error paths.
func TestNumDocsFailures(t *testing.T) {
	dir := t.TempDir()
	engine, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	engine.Close()

	_, err = engine.NumDocs()
	if err == nil {
		t.Error("expected error when engine closed, got nil")
	}
}

// TestCloseRedundant verifies double close is safe.
func TestCloseRedundant(t *testing.T) {
	dir := t.TempDir()
	engine, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Errorf("second Close should be no-op, got %v", err)
	}
}

// TestParseHitEdgeCases covers helper functions.
func TestParseHitEdgeCases(t *testing.T) {
	tests := []struct {
		input     string
		wantID    string
		wantScore float64
	}{
		{``, "", 0},
		{`{"score":2.5}`, "", 2.5},
		{`{"id":"x"}`, "x", 0},
		{`{"id":"a","score":1.1}`, "a", 1.1},
	}
	for _, tt := range tests {
		hit := parseHit(tt.input)
		if hit.ID != tt.wantID || hit.Score != tt.wantScore {
			t.Errorf("parseHit(%q) = {%q, %v}, want {%q, %v}", tt.input, hit.ID, hit.Score, tt.wantID, tt.wantScore)
		}
	}
}
