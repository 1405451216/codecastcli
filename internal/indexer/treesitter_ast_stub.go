//go:build !cgo

package indexer

// extractTagsAST is a no-op stub when CGO is not available.
// Returns nil so the caller falls back to regex-based extraction.
func extractTagsAST(path string, content []byte, language string) []Tag {
	return nil
}
