package mdast_test

import (
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/mdast"
)

func TestToken_Text(t *testing.T) {
	t.Parallel()

	content := []byte("hello world")

	tests := []struct {
		name     string
		token    mdast.Token
		expected string
	}{
		{
			name:     "full content",
			token:    mdast.Token{Kind: mdast.TokText, StartOffset: 0, EndOffset: 11},
			expected: "hello world",
		},
		{
			name:     "first word",
			token:    mdast.Token{Kind: mdast.TokText, StartOffset: 0, EndOffset: 5},
			expected: "hello",
		},
		{
			name:     "second word",
			token:    mdast.Token{Kind: mdast.TokText, StartOffset: 6, EndOffset: 11},
			expected: "world",
		},
		{
			name:     "space",
			token:    mdast.Token{Kind: mdast.TokWhitespace, StartOffset: 5, EndOffset: 6},
			expected: " ",
		},
		{
			name:     "empty token",
			token:    mdast.Token{Kind: mdast.TokText, StartOffset: 5, EndOffset: 5},
			expected: "",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := string(testCase.token.Text(content))
			if got != testCase.expected {
				t.Errorf("expected %q, got %q", testCase.expected, got)
			}
		})
	}
}

func TestToken_TextInvalidRange(t *testing.T) {
	t.Parallel()

	content := []byte("hello")

	tests := []struct {
		name  string
		token mdast.Token
	}{
		{
			name:  "negative start",
			token: mdast.Token{StartOffset: -1, EndOffset: 3},
		},
		{
			name:  "end past content",
			token: mdast.Token{StartOffset: 0, EndOffset: 100},
		},
		{
			name:  "start after end",
			token: mdast.Token{StartOffset: 5, EndOffset: 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.token.Text(content)
			if got != nil {
				t.Errorf("expected nil for invalid range, got %q", got)
			}
		})
	}
}

func TestToken_Len(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		token    mdast.Token
		expected int
	}{
		{"non-empty", mdast.Token{StartOffset: 0, EndOffset: 5}, 5},
		{"empty", mdast.Token{StartOffset: 3, EndOffset: 3}, 0},
		{"single byte", mdast.Token{StartOffset: 0, EndOffset: 1}, 1},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if testCase.token.Len() != testCase.expected {
				t.Errorf("expected %d, got %d", testCase.expected, testCase.token.Len())
			}
		})
	}
}

func TestToken_IsEmpty(t *testing.T) {
	t.Parallel()

	emptyToken := mdast.Token{StartOffset: 5, EndOffset: 5}
	nonEmptyToken := mdast.Token{StartOffset: 0, EndOffset: 5}

	if !emptyToken.IsEmpty() {
		t.Error("expected empty token to be empty")
	}

	if nonEmptyToken.IsEmpty() {
		t.Error("expected non-empty token to not be empty")
	}
}

func TestTokenKind_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind     mdast.TokenKind
		expected string
	}{
		{mdast.TokText, "Text"},
		{mdast.TokWhitespace, "Whitespace"},
		{mdast.TokNewline, "Newline"},
		{mdast.TokHeadingMarker, "HeadingMarker"},
		{mdast.TokCodeFence, "CodeFence"},
		{mdast.TokOther, "Other"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()

			if tt.kind.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.kind.String())
			}
		})
	}
}

func TestValidateTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tokens     []mdast.Token
		contentLen int
		expected   bool
	}{
		{
			name:       "empty tokens empty content",
			tokens:     []mdast.Token{},
			contentLen: 0,
			expected:   true,
		},
		{
			name:       "empty tokens non-empty content",
			tokens:     []mdast.Token{},
			contentLen: 5,
			expected:   false,
		},
		{
			name: "valid single token",
			tokens: []mdast.Token{
				{StartOffset: 0, EndOffset: 5},
			},
			contentLen: 5,
			expected:   true,
		},
		{
			name: "valid multiple tokens",
			tokens: []mdast.Token{
				{StartOffset: 0, EndOffset: 3},
				{StartOffset: 3, EndOffset: 5},
				{StartOffset: 5, EndOffset: 10},
			},
			contentLen: 10,
			expected:   true,
		},
		{
			name: "gap between tokens",
			tokens: []mdast.Token{
				{StartOffset: 0, EndOffset: 3},
				{StartOffset: 5, EndOffset: 10},
			},
			contentLen: 10,
			expected:   false,
		},
		{
			name: "doesn't start at 0",
			tokens: []mdast.Token{
				{StartOffset: 1, EndOffset: 5},
			},
			contentLen: 5,
			expected:   false,
		},
		{
			name: "doesn't end at contentLen",
			tokens: []mdast.Token{
				{StartOffset: 0, EndOffset: 3},
			},
			contentLen: 5,
			expected:   false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := mdast.ValidateTokens(testCase.tokens, testCase.contentLen)
			if got != testCase.expected {
				t.Errorf("expected %v, got %v", testCase.expected, got)
			}
		})
	}
}
