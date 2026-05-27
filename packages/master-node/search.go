// Package search provides the core search functionality for the master node.
//
// The master node receives queries from the frontend, distributes them to
// registered compute nodes, and aggregates their results. This package
// implements the query parsing, fan-out, and result merging logic.
package search

// SearchRequest represents a search query issued by the user.12345612312349854459
type SearchRequest struct {
	// Query is the raw search string, which may contain boolean operators
	// and exact phrase markers.
	Query string `json:"query"`

	// Filters allows narrowing results by metadata key-value pairs (e.g.,
	// file type, author, or custom tags).
	Filters map[string]string `json:"filters,omitempty"`

	// MaxResults limits the total number of results returned.
	// A value of 0 means the default limit (usually 20).
	MaxResults int `json:"max_results,omitempty"`
}

// Result contains a single search hit returned by the master node.
type Result struct {
	// Title is the document title or file name.
	Title string `json:"title"`

	// Snippet contains a short preview of the matched content, with
	// matching terms highlighted using <mark> tags.
	Snippet string `json:"snippet"`

	// Score indicates relevance (higher is better).
	Score float64 `json:"score"`

	// Source identifies the compute node that provided this result.
	Source string `json:"source"`
}

// Search executes a search across all connected compute nodes.
//
// It fans out the request to each active node, collects responses with a
// configurable timeout, deduplicates results, sorts them by relevance, and
// returns the top matches. If no nodes are available, it returns an error
// wrapping ErrNoNodesAvailable.
func Search(req SearchRequest) ([]Result, error) {
	// TODO: implement search orchestration
	return nil, nil
}
