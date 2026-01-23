package mermaid

import "github.com/yaklabco/gomdlint/pkg/lint"

// RegisterMermaidRules registers all mermaid validation rules.
func RegisterMermaidRules(registry *lint.Registry) {
	registry.Register(NewSyntaxRule())             // MM001
	registry.Register(NewUndefinedReferenceRule()) // MM002
}
