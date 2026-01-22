package goldmark

import (
	"testing"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

func TestTokenize_Empty(t *testing.T) {
	tokens := Tokenize(nil)
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens for nil input, got %d", len(tokens))
	}

	tokens = Tokenize([]byte{})
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens for empty input, got %d", len(tokens))
	}
}

func TestTokenize_ValidatesContiguous(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"plain text", "Hello, world!"},
		{"heading", "# Hello"},
		{"heading with text", "# Hello\nWorld"},
		{"list", "- item 1\n- item 2"},
		{"ordered list", "1. first\n2. second"},
		{"blockquote", "> quoted text"},
		{"code fence", "```go\ncode\n```"},
		{"inline code", "Use `code` here"},
		{"emphasis", "*emphasis* and **strong**"},
		{"link", "[text](url)"},
		{"image", "![alt](src)"},
		{"thematic break", "---"},
		{"mixed content", "# Title\n\nParagraph with *emphasis* and `code`.\n\n- list item\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte(tt.content)
			tokens := Tokenize(content)

			if !mdast.ValidateTokens(tokens, len(content)) {
				t.Errorf("tokens are not contiguous or do not cover content")
				for i, tok := range tokens {
					t.Logf("  token[%d]: kind=%v start=%d end=%d text=%q",
						i, tok.Kind, tok.StartOffset, tok.EndOffset, tok.Text(content))
				}
			}
		})
	}
}

func TestTokenize_HeadingMarker(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantKind mdast.TokenKind
	}{
		{"h1", "# Heading", mdast.TokHeadingMarker},
		{"h2", "## Heading", mdast.TokHeadingMarker},
		{"h3", "### Heading", mdast.TokHeadingMarker},
		{"h4", "#### Heading", mdast.TokHeadingMarker},
		{"h5", "##### Heading", mdast.TokHeadingMarker},
		{"h6", "###### Heading", mdast.TokHeadingMarker},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := Tokenize([]byte(tt.content))

			if len(tokens) == 0 {
				t.Fatal("expected at least one token")
			}

			if tokens[0].Kind != tt.wantKind {
				t.Errorf("first token kind = %v, want %v", tokens[0].Kind, tt.wantKind)
			}
		})
	}
}

func TestTokenize_ListBullet(t *testing.T) {
	tests := []struct {
		name   string
		marker string
	}{
		{"dash", "-"},
		{"plus", "+"},
		{"asterisk", "*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte(tt.marker + " item")
			tokens := Tokenize(content)

			if len(tokens) == 0 {
				t.Fatal("expected at least one token")
			}

			if tokens[0].Kind != mdast.TokListBullet {
				t.Errorf("first token kind = %v, want TokListBullet", tokens[0].Kind)
			}
		})
	}
}

func TestTokenize_OrderedList(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"dot delimiter", "1. item"},
		{"paren delimiter", "1) item"},
		{"multi digit", "10. item"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := Tokenize([]byte(tt.content))

			if len(tokens) == 0 {
				t.Fatal("expected at least one token")
			}

			if tokens[0].Kind != mdast.TokListNumber {
				t.Errorf("first token kind = %v, want TokListNumber", tokens[0].Kind)
			}
		})
	}
}

func TestTokenize_Blockquote(t *testing.T) {
	content := []byte("> quoted text")
	tokens := Tokenize(content)

	if len(tokens) == 0 {
		t.Fatal("expected at least one token")
	}

	if tokens[0].Kind != mdast.TokBlockquoteMarker {
		t.Errorf("first token kind = %v, want TokBlockquoteMarker", tokens[0].Kind)
	}
}

func TestTokenize_CodeFence(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"backticks", "```\ncode\n```"},
		{"backticks with info", "```go\ncode\n```"},
		{"tildes", "~~~\ncode\n~~~"},
		{"longer fence", "````\ncode\n````"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte(tt.content)
			tokens := Tokenize(content)

			if !mdast.ValidateTokens(tokens, len(content)) {
				t.Error("tokens are not valid")
			}

			// First token should be code fence.
			if len(tokens) == 0 || tokens[0].Kind != mdast.TokCodeFence {
				t.Errorf("first token kind = %v, want TokCodeFence", tokens[0].Kind)
			}
		})
	}
}

func TestTokenize_CodeFenceWithInfo(t *testing.T) {
	content := []byte("```go\nfunc main() {}\n```")
	tokens := Tokenize(content)

	if !mdast.ValidateTokens(tokens, len(content)) {
		t.Error("tokens are not valid")
	}

	// Should have TokCodeFence followed by TokCodeFenceInfo.
	foundFence := false
	foundInfo := false

	for _, tok := range tokens {
		if tok.Kind == mdast.TokCodeFence {
			foundFence = true
		}
		if tok.Kind == mdast.TokCodeFenceInfo {
			foundInfo = true
		}
	}

	if !foundFence {
		t.Error("expected TokCodeFence token")
	}
	if !foundInfo {
		t.Error("expected TokCodeFenceInfo token")
	}
}

func TestTokenize_ThematicBreak(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"dashes", "---"},
		{"asterisks", "***"},
		{"underscores", "___"},
		{"with spaces", "- - -"},
		{"long", "----------"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte(tt.content)
			tokens := Tokenize(content)

			if len(tokens) == 0 {
				t.Fatal("expected at least one token")
			}

			if tokens[0].Kind != mdast.TokThematicBreak {
				t.Errorf("first token kind = %v, want TokThematicBreak", tokens[0].Kind)
			}
		})
	}
}

func TestTokenize_InlineCode(t *testing.T) {
	content := []byte("Use `code` here")
	tokens := Tokenize(content)

	if !mdast.ValidateTokens(tokens, len(content)) {
		t.Error("tokens are not valid")
	}

	// Should contain TokBacktick tokens.
	backtickCount := 0
	for _, tok := range tokens {
		if tok.Kind == mdast.TokBacktick {
			backtickCount++
		}
	}

	if backtickCount != 2 {
		t.Errorf("expected 2 TokBacktick tokens, got %d", backtickCount)
	}
}

func TestTokenize_Emphasis(t *testing.T) {
	tests := []struct {
		name    string
		content string
		count   int
	}{
		{"single asterisk", "*emphasis*", 2},
		{"double asterisk", "**strong**", 2},
		{"single underscore", "_emphasis_", 2},
		{"double underscore", "__strong__", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte(tt.content)
			tokens := Tokenize(content)

			if !mdast.ValidateTokens(tokens, len(content)) {
				t.Error("tokens are not valid")
			}

			emphasisCount := 0
			for _, tok := range tokens {
				if tok.Kind == mdast.TokEmphasisMarker {
					emphasisCount++
				}
			}

			if emphasisCount != tt.count {
				t.Errorf("expected %d TokEmphasisMarker tokens, got %d", tt.count, emphasisCount)
			}
		})
	}
}

func TestTokenize_Link(t *testing.T) {
	content := []byte("[text](url)")
	tokens := Tokenize(content)

	if !mdast.ValidateTokens(tokens, len(content)) {
		t.Error("tokens are not valid")
	}

	// Should contain link-related tokens.
	var foundOpen, foundClose, foundParenOpen, foundParenClose bool

	for _, tok := range tokens {
		switch tok.Kind {
		case mdast.TokLinkOpen:
			foundOpen = true
		case mdast.TokLinkClose:
			foundClose = true
		case mdast.TokParenOpen:
			foundParenOpen = true
		case mdast.TokParenClose:
			foundParenClose = true
		}
	}

	if !foundOpen {
		t.Error("expected TokLinkOpen")
	}
	if !foundClose {
		t.Error("expected TokLinkClose")
	}
	if !foundParenOpen {
		t.Error("expected TokParenOpen")
	}
	if !foundParenClose {
		t.Error("expected TokParenClose")
	}
}

func TestTokenize_Image(t *testing.T) {
	content := []byte("![alt](src)")
	tokens := Tokenize(content)

	if !mdast.ValidateTokens(tokens, len(content)) {
		t.Error("tokens are not valid")
	}

	// Should contain image marker.
	foundImageMarker := false
	for _, tok := range tokens {
		if tok.Kind == mdast.TokImageMarker {
			foundImageMarker = true
			break
		}
	}

	if !foundImageMarker {
		t.Error("expected TokImageMarker")
	}
}

func TestTokenize_EscapedChar(t *testing.T) {
	content := []byte(`\*not emphasis\*`)
	tokens := Tokenize(content)

	if !mdast.ValidateTokens(tokens, len(content)) {
		t.Error("tokens are not valid")
	}

	// Should contain escaped char tokens.
	escapedCount := 0
	for _, tok := range tokens {
		if tok.Kind == mdast.TokEscapedChar {
			escapedCount++
		}
	}

	if escapedCount != 2 {
		t.Errorf("expected 2 TokEscapedChar tokens, got %d", escapedCount)
	}
}

func TestTokenize_HTML(t *testing.T) {
	content := []byte("<div>content</div>")
	tokens := Tokenize(content)

	if !mdast.ValidateTokens(tokens, len(content)) {
		t.Error("tokens are not valid")
	}

	// Should contain HTML tokens.
	htmlCount := 0
	for _, tok := range tokens {
		if tok.Kind == mdast.TokHTML {
			htmlCount++
		}
	}

	if htmlCount == 0 {
		t.Error("expected at least one TokHTML token")
	}
}

func TestTokenize_SetextUnderline(t *testing.T) {
	// Note: Setext underlines with dashes are ambiguous with thematic breaks
	// at the tokenizer level. The tokenizer treats standalone dash lines as
	// thematic breaks. Only equals-style setext underlines are unambiguous.
	// The goldmark parser handles the semantic distinction.
	tests := []struct {
		name    string
		content string
	}{
		{"equals", "Title\n====="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte(tt.content)
			tokens := Tokenize(content)

			if !mdast.ValidateTokens(tokens, len(content)) {
				t.Error("tokens are not valid")
			}

			// Should contain setext underline.
			foundSetext := false
			for _, tok := range tokens {
				if tok.Kind == mdast.TokSetextUnderline {
					foundSetext = true
					break
				}
			}

			if !foundSetext {
				t.Error("expected TokSetextUnderline")
			}
		})
	}
}

func TestTokenize_Newlines(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"LF", "line1\nline2"},
		{"CRLF", "line1\r\nline2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte(tt.content)
			tokens := Tokenize(content)

			if !mdast.ValidateTokens(tokens, len(content)) {
				t.Error("tokens are not valid")
			}

			// Should contain newline token.
			foundNewline := false
			for _, tok := range tokens {
				if tok.Kind == mdast.TokNewline {
					foundNewline = true
					break
				}
			}

			if !foundNewline {
				t.Error("expected TokNewline")
			}
		})
	}
}

func TestTokenize_Whitespace(t *testing.T) {
	content := []byte("hello   there")
	tokens := Tokenize(content)

	if !mdast.ValidateTokens(tokens, len(content)) {
		t.Error("tokens are not valid")
	}

	// Should contain whitespace token.
	foundWhitespace := false
	for _, tok := range tokens {
		if tok.Kind == mdast.TokWhitespace {
			foundWhitespace = true
			break
		}
	}

	if !foundWhitespace {
		t.Error("expected TokWhitespace")
	}
}

func TestTokenize_ComplexDocument(t *testing.T) {
	content := []byte(`# Main Title

This is a paragraph with *emphasis*, **strong**, and ` + "`code`" + `.

## Subsection

- Item 1
- Item 2
  - Nested item

> Blockquote with [link](url)

` + "```go" + `
func main() {
    fmt.Println("Hello")
}
` + "```" + `

---

1. First
2. Second
`)

	tokens := Tokenize(content)

	if !mdast.ValidateTokens(tokens, len(content)) {
		t.Error("tokens are not contiguous or do not cover content")
		for i, tok := range tokens {
			t.Logf("  token[%d]: kind=%v start=%d end=%d",
				i, tok.Kind, tok.StartOffset, tok.EndOffset)
		}
	}
}
