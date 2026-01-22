package lint_test

import (
	"context"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

const defaultTestValue = "default"

func TestNewRuleContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	file := &mdast.FileSnapshot{
		Path:    "test.md",
		Content: []byte("# Hello"),
		Root:    mdast.NewNode(mdast.NodeDocument),
	}
	cfg := config.NewConfig()
	ruleCfg := &config.RuleConfig{
		Options: map[string]any{"key": "value"},
	}

	rc := lint.NewRuleContext(ctx, file, cfg, ruleCfg)

	if rc.Ctx != ctx {
		t.Error("Ctx mismatch")
	}
	if rc.File != file {
		t.Error("File mismatch")
	}
	if rc.Root != file.Root {
		t.Error("Root should equal File.Root")
	}
	if rc.Config != cfg {
		t.Error("Config mismatch")
	}
	if rc.RuleConfig != ruleCfg {
		t.Error("RuleConfig mismatch")
	}
	if rc.Builder == nil {
		t.Error("Builder should be initialized")
	}
}

func TestNewRuleContext_NilFile(t *testing.T) {
	t.Parallel()

	rc := lint.NewRuleContext(context.Background(), nil, nil, nil)

	if rc.File != nil {
		t.Error("File should be nil")
	}
	if rc.Root != nil {
		t.Error("Root should be nil when File is nil")
	}
}

func TestRuleContext_Cancelled(t *testing.T) {
	t.Parallel()

	t.Run("not cancelled", func(t *testing.T) {
		t.Parallel()

		rc := lint.NewRuleContext(context.Background(), nil, nil, nil)

		if rc.Cancelled() {
			t.Error("should not be cancelled")
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		rc := lint.NewRuleContext(ctx, nil, nil, nil)

		if !rc.Cancelled() {
			t.Error("should be cancelled")
		}
	})
}

func TestRuleContext_Option(t *testing.T) {
	t.Parallel()

	t.Run("returns default when RuleConfig is nil", func(t *testing.T) {
		t.Parallel()

		rc := lint.NewRuleContext(context.Background(), nil, nil, nil)

		result := rc.Option("key", defaultTestValue)
		if result != defaultTestValue {
			t.Errorf("got %v, want %s", result, defaultTestValue)
		}
	})

	t.Run("returns default when Options is nil", func(t *testing.T) {
		t.Parallel()

		rc := lint.NewRuleContext(context.Background(), nil, nil, &config.RuleConfig{})

		result := rc.Option("key", defaultTestValue)
		if result != defaultTestValue {
			t.Errorf("got %v, want %s", result, defaultTestValue)
		}
	})

	t.Run("returns default when key not found", func(t *testing.T) {
		t.Parallel()

		rc := lint.NewRuleContext(context.Background(), nil, nil, &config.RuleConfig{
			Options: map[string]any{"other": "value"},
		})

		result := rc.Option("key", defaultTestValue)
		if result != defaultTestValue {
			t.Errorf("got %v, want %s", result, defaultTestValue)
		}
	})

	t.Run("returns value when found", func(t *testing.T) {
		t.Parallel()

		rc := lint.NewRuleContext(context.Background(), nil, nil, &config.RuleConfig{
			Options: map[string]any{"key": "found"},
		})

		result := rc.Option("key", "default")
		if result != "found" {
			t.Errorf("got %v, want found", result)
		}
	})
}

func TestRuleContext_OptionInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options map[string]any
		key     string
		def     int
		want    int
	}{
		{
			name:    "returns default when nil options",
			options: nil,
			key:     "max",
			def:     100,
			want:    100,
		},
		{
			name:    "returns int value",
			options: map[string]any{"max": 50},
			key:     "max",
			def:     100,
			want:    50,
		},
		{
			name:    "converts float64 to int",
			options: map[string]any{"max": float64(75)},
			key:     "max",
			def:     100,
			want:    75,
		},
		{
			name:    "returns default for wrong type",
			options: map[string]any{"max": "not an int"},
			key:     "max",
			def:     100,
			want:    100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var ruleCfg *config.RuleConfig
			if tt.options != nil {
				ruleCfg = &config.RuleConfig{Options: tt.options}
			}

			rc := lint.NewRuleContext(context.Background(), nil, nil, ruleCfg)
			got := rc.OptionInt(tt.key, tt.def)

			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRuleContext_OptionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options map[string]any
		key     string
		def     string
		want    string
	}{
		{
			name:    "returns default when nil options",
			options: nil,
			key:     "style",
			def:     "default",
			want:    "default",
		},
		{
			name:    "returns string value",
			options: map[string]any{"style": "custom"},
			key:     "style",
			def:     "default",
			want:    "custom",
		},
		{
			name:    "returns default for wrong type",
			options: map[string]any{"style": 123},
			key:     "style",
			def:     "default",
			want:    "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var ruleCfg *config.RuleConfig
			if tt.options != nil {
				ruleCfg = &config.RuleConfig{Options: tt.options}
			}

			rc := lint.NewRuleContext(context.Background(), nil, nil, ruleCfg)
			got := rc.OptionString(tt.key, tt.def)

			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRuleContext_OptionBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options map[string]any
		key     string
		def     bool
		want    bool
	}{
		{
			name:    "returns default when nil options",
			options: nil,
			key:     "enabled",
			def:     true,
			want:    true,
		},
		{
			name:    "returns bool value true",
			options: map[string]any{"enabled": true},
			key:     "enabled",
			def:     false,
			want:    true,
		},
		{
			name:    "returns bool value false",
			options: map[string]any{"enabled": false},
			key:     "enabled",
			def:     true,
			want:    false,
		},
		{
			name:    "returns default for wrong type",
			options: map[string]any{"enabled": "yes"},
			key:     "enabled",
			def:     true,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var ruleCfg *config.RuleConfig
			if tt.options != nil {
				ruleCfg = &config.RuleConfig{Options: tt.options}
			}

			rc := lint.NewRuleContext(context.Background(), nil, nil, ruleCfg)
			got := rc.OptionBool(tt.key, tt.def)

			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRuleContext_HasRegistry(t *testing.T) {
	t.Parallel()

	reg := lint.NewRegistry()
	ctx := &lint.RuleContext{
		Registry: reg,
	}

	if ctx.Registry == nil {
		t.Error("Registry should not be nil")
	}
	if ctx.Registry != reg {
		t.Error("Registry should be the same instance")
	}
}

// createTestDocumentWithNodes creates a test document with multiple node types.
func createTestDocumentWithNodes() *mdast.Node {
	// Create a document with headings, lists, code blocks, links, images, paragraphs.
	doc := mdast.NewNode(mdast.NodeDocument)

	// Add heading
	h1 := mdast.NewNode(mdast.NodeHeading)
	h1.Block = &mdast.BlockAttrs{HeadingLevel: 1}
	mdast.AppendChild(doc, h1)

	// Add paragraph
	para := mdast.NewNode(mdast.NodeParagraph)
	mdast.AppendChild(doc, para)

	// Add link inside paragraph
	link := mdast.NewNode(mdast.NodeLink)
	link.Inline = &mdast.InlineAttrs{Link: &mdast.LinkAttrs{Destination: "http://example.com"}}
	mdast.AppendChild(para, link)

	// Add image
	img := mdast.NewNode(mdast.NodeImage)
	img.Inline = &mdast.InlineAttrs{Link: &mdast.LinkAttrs{Destination: "image.png"}}
	mdast.AppendChild(para, img)

	// Add code span
	codeSpan := mdast.NewNode(mdast.NodeCodeSpan)
	mdast.AppendChild(para, codeSpan)

	// Add another heading
	h2 := mdast.NewNode(mdast.NodeHeading)
	h2.Block = &mdast.BlockAttrs{HeadingLevel: 2}
	mdast.AppendChild(doc, h2)

	// Add list
	list := mdast.NewNode(mdast.NodeList)
	mdast.AppendChild(doc, list)

	// Add list items
	li1 := mdast.NewNode(mdast.NodeListItem)
	mdast.AppendChild(list, li1)
	li2 := mdast.NewNode(mdast.NodeListItem)
	mdast.AppendChild(list, li2)

	// Add code block
	codeBlock := mdast.NewNode(mdast.NodeCodeBlock)
	mdast.AppendChild(doc, codeBlock)

	// Add blockquote
	blockquote := mdast.NewNode(mdast.NodeBlockquote)
	mdast.AppendChild(doc, blockquote)

	// Add thematic break
	hr := mdast.NewNode(mdast.NodeThematicBreak)
	mdast.AppendChild(doc, hr)

	// Add HTML block
	htmlBlock := mdast.NewNode(mdast.NodeHTMLBlock)
	mdast.AppendChild(doc, htmlBlock)

	// Add paragraph with inline HTML
	para2 := mdast.NewNode(mdast.NodeParagraph)
	mdast.AppendChild(doc, para2)
	htmlInline := mdast.NewNode(mdast.NodeHTMLInline)
	mdast.AppendChild(para2, htmlInline)

	// Add emphasis
	emph := mdast.NewNode(mdast.NodeEmphasis)
	mdast.AppendChild(para2, emph)

	// Add strong
	strong := mdast.NewNode(mdast.NodeStrong)
	mdast.AppendChild(para2, strong)

	return doc
}

func TestRuleContext_Headings(t *testing.T) {
	t.Parallel()

	doc := createTestDocumentWithNodes()
	file := &mdast.FileSnapshot{Root: doc}
	rc := lint.NewRuleContext(context.Background(), file, nil, nil)

	headings := rc.Headings()

	if len(headings) != 2 {
		t.Errorf("got %d headings, want 2", len(headings))
	}
}

func TestRuleContext_Lists(t *testing.T) {
	t.Parallel()

	doc := createTestDocumentWithNodes()
	file := &mdast.FileSnapshot{Root: doc}
	rc := lint.NewRuleContext(context.Background(), file, nil, nil)

	lists := rc.Lists()

	if len(lists) != 1 {
		t.Errorf("got %d lists, want 1", len(lists))
	}
}

func TestRuleContext_ListItems(t *testing.T) {
	t.Parallel()

	doc := createTestDocumentWithNodes()
	file := &mdast.FileSnapshot{Root: doc}
	rc := lint.NewRuleContext(context.Background(), file, nil, nil)

	items := rc.ListItems()

	if len(items) != 2 {
		t.Errorf("got %d list items, want 2", len(items))
	}
}

func TestRuleContext_CodeBlocks(t *testing.T) {
	t.Parallel()

	doc := createTestDocumentWithNodes()
	file := &mdast.FileSnapshot{Root: doc}
	rc := lint.NewRuleContext(context.Background(), file, nil, nil)

	codeBlocks := rc.CodeBlocks()

	if len(codeBlocks) != 1 {
		t.Errorf("got %d code blocks, want 1", len(codeBlocks))
	}
}

func TestRuleContext_Links(t *testing.T) {
	t.Parallel()

	doc := createTestDocumentWithNodes()
	file := &mdast.FileSnapshot{Root: doc}
	rc := lint.NewRuleContext(context.Background(), file, nil, nil)

	links := rc.Links()

	if len(links) != 1 {
		t.Errorf("got %d links, want 1", len(links))
	}
}

func TestRuleContext_Images(t *testing.T) {
	t.Parallel()

	doc := createTestDocumentWithNodes()
	file := &mdast.FileSnapshot{Root: doc}
	rc := lint.NewRuleContext(context.Background(), file, nil, nil)

	images := rc.Images()

	if len(images) != 1 {
		t.Errorf("got %d images, want 1", len(images))
	}
}

func TestRuleContext_Paragraphs(t *testing.T) {
	t.Parallel()

	doc := createTestDocumentWithNodes()
	file := &mdast.FileSnapshot{Root: doc}
	rc := lint.NewRuleContext(context.Background(), file, nil, nil)

	paragraphs := rc.Paragraphs()

	if len(paragraphs) != 2 {
		t.Errorf("got %d paragraphs, want 2", len(paragraphs))
	}
}

func TestRuleContext_Blockquotes(t *testing.T) {
	t.Parallel()

	doc := createTestDocumentWithNodes()
	file := &mdast.FileSnapshot{Root: doc}
	rc := lint.NewRuleContext(context.Background(), file, nil, nil)

	blockquotes := rc.Blockquotes()

	if len(blockquotes) != 1 {
		t.Errorf("got %d blockquotes, want 1", len(blockquotes))
	}
}

func TestRuleContext_ThematicBreaks(t *testing.T) {
	t.Parallel()

	doc := createTestDocumentWithNodes()
	file := &mdast.FileSnapshot{Root: doc}
	rc := lint.NewRuleContext(context.Background(), file, nil, nil)

	hrs := rc.ThematicBreaks()

	if len(hrs) != 1 {
		t.Errorf("got %d thematic breaks, want 1", len(hrs))
	}
}

func TestRuleContext_HTMLBlocks(t *testing.T) {
	t.Parallel()

	doc := createTestDocumentWithNodes()
	file := &mdast.FileSnapshot{Root: doc}
	rc := lint.NewRuleContext(context.Background(), file, nil, nil)

	htmlBlocks := rc.HTMLBlocks()

	if len(htmlBlocks) != 1 {
		t.Errorf("got %d HTML blocks, want 1", len(htmlBlocks))
	}
}

func TestRuleContext_HTMLInlines(t *testing.T) {
	t.Parallel()

	doc := createTestDocumentWithNodes()
	file := &mdast.FileSnapshot{Root: doc}
	rc := lint.NewRuleContext(context.Background(), file, nil, nil)

	htmlInlines := rc.HTMLInlines()

	if len(htmlInlines) != 1 {
		t.Errorf("got %d HTML inlines, want 1", len(htmlInlines))
	}
}

func TestRuleContext_EmphasisNodes(t *testing.T) {
	t.Parallel()

	doc := createTestDocumentWithNodes()
	file := &mdast.FileSnapshot{Root: doc}
	rc := lint.NewRuleContext(context.Background(), file, nil, nil)

	emphasis := rc.EmphasisNodes()

	if len(emphasis) != 1 {
		t.Errorf("got %d emphasis nodes, want 1", len(emphasis))
	}
}

func TestRuleContext_StrongNodes(t *testing.T) {
	t.Parallel()

	doc := createTestDocumentWithNodes()
	file := &mdast.FileSnapshot{Root: doc}
	rc := lint.NewRuleContext(context.Background(), file, nil, nil)

	strong := rc.StrongNodes()

	if len(strong) != 1 {
		t.Errorf("got %d strong nodes, want 1", len(strong))
	}
}

func TestRuleContext_NodeCache_LazyInitialization(t *testing.T) {
	t.Parallel()

	doc := createTestDocumentWithNodes()
	file := &mdast.FileSnapshot{Root: doc}
	rc := lint.NewRuleContext(context.Background(), file, nil, nil)

	// First call builds the cache
	headings1 := rc.Headings()
	// Second call should return the same slice (cached)
	headings2 := rc.Headings()

	if len(headings1) != len(headings2) {
		t.Errorf("headings count mismatch: %d vs %d", len(headings1), len(headings2))
	}

	// Verify they're the same slice (same underlying array)
	if len(headings1) > 0 && &headings1[0] != &headings2[0] {
		t.Error("headings should be the same slice (cached)")
	}
}

func TestRuleContext_NodeCache_NilRoot(t *testing.T) {
	t.Parallel()

	// Create context with nil root
	rc := lint.NewRuleContext(context.Background(), nil, nil, nil)

	// All accessors should return empty slices without panicking
	if len(rc.Headings()) != 0 {
		t.Error("headings should be empty for nil root")
	}
	if len(rc.Lists()) != 0 {
		t.Error("lists should be empty for nil root")
	}
	if len(rc.CodeBlocks()) != 0 {
		t.Error("code blocks should be empty for nil root")
	}
}
