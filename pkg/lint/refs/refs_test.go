package refs

import (
	"bytes"
	"context"
	"strconv"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

func TestNormalizeLabel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "foo", "foo"},
		{"uppercase", "FOO", "foo"},
		{"mixed case", "FoO BaR", "foo bar"},
		{"extra spaces", "foo  bar", "foo bar"},
		{"leading spaces", "  foo", "foo"},
		{"trailing spaces", "foo  ", "foo"},
		{"tabs", "foo\tbar", "foo bar"},
		{"newlines", "foo\nbar", "foo bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeLabel(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeLabel(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestExtractFragment(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"no fragment", "https://example.com", ""},
		{"with fragment", "https://example.com#section", "#section"},
		{"only fragment", "#section", "#section"},
		{"empty fragment", "https://example.com#", "#"},
		{"relative with fragment", "page.md#heading", "#heading"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractFragment(tt.url)
			if got != tt.expected {
				t.Errorf("ExtractFragment(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}

func TestIsGitHubLineReference(t *testing.T) {
	tests := []struct {
		id       string
		expected bool
	}{
		{"L20", true},
		{"L1", true},
		{"L19C5", true},
		{"L19C5-L21C11", true},
		{"L19-L21", true},
		{"l20", true},      // lowercase
		{"heading", false}, // not a line reference
		{"L", false},       // no number
		{"", false},        // empty
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := isGitHubLineReference(tt.id)
			if got != tt.expected {
				t.Errorf("isGitHubLineReference(%q) = %v, want %v", tt.id, got, tt.expected)
			}
		})
	}
}

func TestContext_ValidateFragment(t *testing.T) {
	ctx := NewContext(nil)

	// Add some anchors
	ctx.Anchors.Add(&Anchor{ID: "heading-one", Source: AnchorFromHeading})
	ctx.Anchors.Add(&Anchor{ID: "custom-id", Source: AnchorFromHTMLID})

	tests := []struct {
		name     string
		fragment string
		expected bool
	}{
		{"empty fragment", "", true},
		{"just hash", "#", true},
		{"special top", "#top", true},
		// markdownlint matches "#top" in lowercase only, unless case is
		// ignored.
		{"special TOP", "#TOP", false},
		{"github line ref", "#L20", true},
		{"valid anchor", "#heading-one", true},
		{"valid html anchor", "#custom-id", true},
		{"invalid anchor", "#nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ctx.ValidateFragment(tt.fragment)
			if got != tt.expected {
				t.Errorf("ValidateFragment(%q) = %v, want %v", tt.fragment, got, tt.expected)
			}
		})
	}

	for fragment, want := range map[string]bool{"#TOP": true, "#Top": true, "#HEADING-ONE": true, "#nonexistent": false} {
		if got := ctx.ValidateFragmentIgnoreCase(fragment); got != want {
			t.Errorf("ValidateFragmentIgnoreCase(%q) = %v, want %v", fragment, got, want)
		}
	}
}

func TestContext_ResolveLabel(t *testing.T) {
	ctx := NewContext(nil)

	// Add a definition
	def := &ReferenceDefinition{
		Label:           "Example",
		NormalizedLabel: "example",
		Destination:     "https://example.com",
	}
	ctx.Definitions["example"] = def

	tests := []struct {
		name     string
		label    string
		expected bool
	}{
		{"exact match", "example", true},
		{"different case", "Example", true},
		{"uppercase", "EXAMPLE", true},
		{"not found", "other", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ctx.ResolveLabel(tt.label)
			if (got != nil) != tt.expected {
				t.Errorf("ResolveLabel(%q) found = %v, want %v", tt.label, got != nil, tt.expected)
			}
		})
	}
}

func TestContext_UnusedDefinitions(t *testing.T) {
	ctx := NewContext(nil)

	// Add definitions
	used := &ReferenceDefinition{Label: "used", NormalizedLabel: "used", UsageCount: 1}
	unused := &ReferenceDefinition{Label: "unused", NormalizedLabel: "unused", UsageCount: 0}
	duplicate := &ReferenceDefinition{Label: "dup", NormalizedLabel: "dup", UsageCount: 0, IsDuplicate: true}

	ctx.AllDefinitions = []*ReferenceDefinition{used, unused, duplicate}

	got := ctx.UnusedDefinitions()
	if len(got) != 1 {
		t.Errorf("UnusedDefinitions() returned %d items, want 1", len(got))
	}
	if len(got) > 0 && got[0] != unused {
		t.Errorf("UnusedDefinitions() returned wrong definition")
	}
}

func TestContext_DuplicateDefinitions(t *testing.T) {
	ctx := NewContext(nil)

	// Add definitions
	first := &ReferenceDefinition{Label: "dup", NormalizedLabel: "dup", IsDuplicate: false}
	duplicate := &ReferenceDefinition{Label: "dup", NormalizedLabel: "dup", IsDuplicate: true}
	unique := &ReferenceDefinition{Label: "unique", NormalizedLabel: "unique", IsDuplicate: false}

	ctx.AllDefinitions = []*ReferenceDefinition{first, duplicate, unique}

	got := ctx.DuplicateDefinitions()
	if len(got) != 1 {
		t.Errorf("DuplicateDefinitions() returned %d items, want 1", len(got))
	}
	if len(got) > 0 && got[0] != duplicate {
		t.Errorf("DuplicateDefinitions() returned wrong definition")
	}
}

func TestAnchorMap_GenerateAnchor(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{"simple", "Hello World", "hello-world"},
		{"uppercase", "API Reference", "api-reference"},
		{"numbers", "Version 1.0.0", "version-100"},
		{"punctuation", "Don't Panic!", "dont-panic"},
		{"c++", "C++ Guide", "c-guide"},
		{"underscores", "foo_bar_baz", "foo_bar_baz"},
		// github-slugger turns every space into a hyphen and never trims or collapses.
		{"multiple spaces", "hello   world", "hello---world"},
		{"leading trailing spaces", "  hello  ", "--hello--"},
		{"emoji", "Hello World 🎉", "hello-world-"},
		{"only special", "!!!???", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewAnchorMap() // Fresh map for each test
			got := m.GenerateAnchor(tt.text)
			if got != tt.expected {
				t.Errorf("GenerateAnchor(%q) = %q, want %q", tt.text, got, tt.expected)
			}
		})
	}
}

// TestGenerateAnchor_GitHubSlugger checks github-slugger semantics: lowercase,
// drop everything except letters, marks, numbers, connector punctuation,
// hyphens and spaces, then turn each space into a hyphen. Nothing is trimmed
// or collapsed.
func TestGenerateAnchor_GitHubSlugger(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"A - B", "a---b"},
		{"--flag option", "--flag-option"},
		{"हिन्दी", "हिन्दी"},
		{"Café Menu", "café-menu"},
		{"Hello World 🎉", "hello-world-"},
		{"hello   world", "hello---world"},
		{"Tab\tseparated", "tabseparated"},
		{"snake_case_name", "snake_case_name"},
		{"Version 1.0.0", "version-100"},
		{"Don't Panic!", "dont-panic"},
		{"!!!???", ""},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			if got := NewAnchorMap().GenerateAnchor(tt.text); got != tt.want {
				t.Errorf("GenerateAnchor(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

// TestGenerateAnchor_GitHubSluggerDuplicates checks that a suffixed slug that
// collides with a later heading's slug keeps counting, as github-slugger does.
func TestGenerateAnchor_GitHubSluggerDuplicates(t *testing.T) {
	anchors := NewAnchorMap()
	got := []string{anchors.GenerateAnchor("Foo"), anchors.GenerateAnchor("Foo"), anchors.GenerateAnchor("Foo 1")}
	want := []string{"foo", "foo-1", "foo-1-1"}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Errorf("anchor %d = %q, want %q", idx, got[idx], want[idx])
		}
	}

	anchors = NewAnchorMap()
	got = []string{anchors.GenerateAnchor("Foo 1"), anchors.GenerateAnchor("Foo"), anchors.GenerateAnchor("Foo")}
	want = []string{"foo-1", "foo", "foo-2"}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Errorf("anchor %d = %q, want %q", idx, got[idx], want[idx])
		}
	}
}

func TestContext_ValidateFragmentPercentEncoded(t *testing.T) {
	ctx := NewContext(nil)
	ctx.Anchors.AddFromHeading("Café Menu", mdast.SourcePosition{})

	for _, fragment := range []string{"#café-menu", "#caf%C3%A9-menu", "#caf%c3%a9-menu"} {
		if !ctx.ValidateFragment(fragment) {
			t.Errorf("ValidateFragment(%q) = false, want true", fragment)
		}
	}
	for _, fragment := range []string{"#cafe-menu", "#caf%zz-menu"} {
		if ctx.ValidateFragment(fragment) {
			t.Errorf("ValidateFragment(%q) = true, want false", fragment)
		}
	}
	if !ctx.ValidateFragmentIgnoreCase("#CAF%C3%A9-Menu") {
		t.Error("ValidateFragmentIgnoreCase(\"#CAF%C3%A9-Menu\") = false, want true")
	}
}

func TestCollect_HeadingTextForAnchors(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     string
	}{
		{"code span with leading hyphens", "## `--flag` option\n", "--flag-option"},
		{"escaped punctuation", "## Learn C\\#\n", "learn-c"},
		{"entity reference", "## Tom &amp; Jerry\n", "tom--jerry"},
		{"link text counts", "## [Docs](https://example.com) page\n", "docs-page"},
		{"image alt text does not count", "## ![logo](logo.png) Title\n", "-title"},
		{"emphasis markers do not count", "## *Important* note\n", "important-note"},
		{"inline html does not count", "## <span>Styled</span> heading\n", "styled-heading"},
		{"multi-line setext heading", "First\nsecond line\n---\n", "firstsecond-line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := goldmark.New("gfm").Parse(context.Background(), "test.md", []byte(tt.markdown))
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			refCtx := Collect(file.Root, file)
			if !refCtx.Anchors.Has(tt.want) {
				var ids []string
				for _, anchor := range refCtx.Anchors.All() {
					ids = append(ids, anchor.ID)
				}
				t.Errorf("anchor %q not found; anchors: %q", tt.want, ids)
			}
		})
	}
}

func TestCollect_UnresolvedReferences(t *testing.T) {
	type found struct {
		style ReferenceStyle
		label string
		image bool
	}
	tests := []struct {
		name     string
		markdown string
		want     []found
	}{
		{"full", "[text][label]\n", []found{{StyleFull, "label", false}}},
		{"collapsed", "[label][]\n", []found{{StyleCollapsed, "label", false}}},
		{"shortcut", "[label]\n", []found{{StyleShortcut, "label", false}}},
		{"image", "![alt][pic]\n", []found{{StyleFull, "pic", true}}},
		{"escaped image marker", "\\![alt][pic]\n", []found{{StyleFull, "pic", false}}},
		// The failed pair inside leaves "]" in the text, so markdownlint
		// records no full reference and "[d]" is a shortcut.
		{"nested brackets in link text", "[a [b] c][d]\n", []found{{StyleShortcut, "b", false}, {StyleShortcut, "d", false}}},
		{"brackets cannot form a shortcut label", "[a [b] c]\n", []found{{StyleShortcut, "b", false}}},
		{"emphasis inside link text", "[*em*][ref]\n", []found{{StyleFull, "ref", false}}},
		{"code span in a collapsed label", "[a `code`][]\n", []found{{StyleCollapsed, "a `code`", false}}},
		{"emphasis delimiters stay in a label", "[*a*][]\n", []found{{StyleCollapsed, "*a*", false}}},
		{"emphasis in a full label", "[t][*x*]\n", []found{{StyleFull, "*x*", false}}},
		{"bracket inside a code span makes no label", "[a `]` b][]\n", nil},
		{"raw HTML in a shortcut label", "[a <b>x</b>]\n", []found{{StyleShortcut, "a <b>x</b>", false}}},
		{"multi-line code span keeps its padding", "[a ` x\ny `][]\n", []found{{StyleCollapsed, "a ` x\ny `", false}}},
		{"hard break backslash stays in the label", "[a\\\nb][]\n", []found{{StyleCollapsed, "a\\\nb", false}}},
		{"failed inline link falls back to a shortcut", "[text](\n", []found{{StyleShortcut, "text", false}}},
		{"autolink text is not searched", "<https://example.com/[a][b]>\n", nil},
		{"code span", "`[text][label]`\n", nil},
		{"blank label", "[ ]\n", nil},
		{"resolved reference is not unresolved", "[text][ok]\n\n[ok]: https://example.com\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := goldmark.New("commonmark").Parse(context.Background(), "test.md", []byte(tt.markdown))
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			var got []found
			for _, usage := range Collect(file.Root, file).Usages {
				if usage.Unresolved {
					got = append(got, found{usage.Style, usage.Label, usage.IsImage})
				}
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			for idx := range tt.want {
				if got[idx] != tt.want[idx] {
					t.Errorf("reference %d = %+v, want %+v", idx, got[idx], tt.want[idx])
				}
			}
		})
	}
}

// TestCollect_UnresolvedReferencesAsMarkdownlint checks the unresolved
// references recorded where the text of a bracket pair decides, as
// markdownlint 0.41.1 records them (each want was its MD052 result with
// shortcut_syntax and no ignored labels). markdownlint records no reference
// whose text, trimmed as JavaScript trims it, is empty or holds a closing
// bracket, and the brackets after such text start afresh. Brackets of
// nothing but white space after a pair drop the pair too.
func TestCollect_UnresolvedReferencesAsMarkdownlint(t *testing.T) {
	type found struct {
		style        ReferenceStyle
		label        string
		line, column int
	}
	tests := []struct {
		markdown string
		want     []found
	}{
		{"Use [ ][y] done.\n", []found{{StyleShortcut, "y", 1, 8}}},
		{"Use [\f][y] done.\n", []found{{StyleShortcut, "y", 1, 8}}},
		{"Use [\v\t][y] done.\n", []found{{StyleShortcut, "y", 1, 9}}},
		{"Use [\xc2\xa0][y] done.\n", []found{{StyleShortcut, "y", 1, 9}}},
		{"Use [\xe3\x80\x80][y] done.\n", []found{{StyleShortcut, "y", 1, 10}}},
		{"Use [][y] done.\n", []found{{StyleShortcut, "y", 1, 7}}},
		{"Use ![ ][y] done.\n", []found{{StyleShortcut, "y", 1, 9}}},
		{"Use [ ][y][z] done.\n", []found{{StyleFull, "z", 1, 8}}},
		{"Use [ ][y][] done.\n", []found{{StyleCollapsed, "y", 1, 8}}},
		{"> Use [ ][y\n> z] done.\n", []found{{StyleShortcut, "y\n> z", 1, 10}}},
		{"[a [b] c][d]\n", []found{{StyleShortcut, "b", 1, 4}, {StyleShortcut, "d", 1, 10}}},
		{"[*em*][ref]\n", []found{{StyleFull, "ref", 1, 1}}},
		{"[a\\]b][d]\n", []found{{StyleFull, "d", 1, 1}}},
		{"[\xc2\xa0]\n", nil},
		{"[a][\xc2\xa0]\n", nil},
		{"[a][ ]\n", nil},
		{"[a][\t]\n", nil},
		{"[a][]\n", []found{{StyleCollapsed, "a", 1, 1}}},
		// Brackets right after a reference join it: markdownlint reads one
		// reference from the start of the first, with the last label, or
		// none when the last brackets hold nothing but white space.
		{"Use [x][y][z] done.\n", []found{{StyleFull, "z", 1, 5}}},
		{"Use [x][y][z][w] done.\n", []found{{StyleFull, "w", 1, 5}}},
		{"Use [x][y][] done.\n", []found{{StyleCollapsed, "y", 1, 5}}},
		{"Use [x][y][z][] done.\n", []found{{StyleCollapsed, "z", 1, 5}}},
		{"Use [x][][y] done.\n", []found{{StyleFull, "y", 1, 5}}},
		{"Use [x]![y] done.\n", []found{{StyleFull, "y", 1, 5}}},
		{"Use [x][]![y] done.\n", []found{{StyleFull, "y", 1, 5}}},
		{"Use [x][y][ ] done.\n", nil},
		{"Use [x][y][\xc2\xa0] done.\n", nil},
		{"Use [a][b][x][\xc2\xa0] done.\n", nil},
		{"Use [x] [y][z] done.\n", []found{{StyleShortcut, "x", 1, 5}, {StyleFull, "z", 1, 9}}},
	}
	for _, tt := range tests {
		t.Run(strconv.Quote(tt.markdown), func(t *testing.T) {
			file, err := goldmark.New("commonmark").Parse(context.Background(), "test.md", []byte(tt.markdown))
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			var got []found
			for _, usage := range Collect(file.Root, file).Usages {
				if usage.Unresolved {
					got = append(got, found{usage.Style, usage.Label, usage.Position.StartLine, usage.Position.StartColumn})
				}
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			for idx := range tt.want {
				if got[idx] != tt.want[idx] {
					t.Errorf("reference %d = %+v, want %+v", idx, got[idx], tt.want[idx])
				}
			}
		})
	}
}

// TestMarkdownlintKey checks the key markdownlint's normalizeReference makes
// of a label: lowercased as JavaScript lowercases, trimmed as JavaScript
// trims, and each run of white space made one space. It folds no case, so
// "Straße" and "strasse" keep different keys.
func TestMarkdownlintKey(t *testing.T) {
	tests := map[string]string{
		"Label":                "label",
		"  A \t\n B  ":         "a b",
		"Straße":               "straße",
		"STRASSE":              "strasse",
		"\u0130":               "i\u0307",
		"\f":                   "",
		"\u00a0x\u3000y\ufeff": "x y",
		"a\u0085b":             "a\u0085b",
	}
	for label, want := range tests {
		if got := MarkdownlintKey(label); got != want {
			t.Errorf("MarkdownlintKey(%q) = %q, want %q", label, got, want)
		}
	}
}

// TestFootnoteTextUnknownParagraph checks that a definition whose paragraph
// is unknown, as in a Context built by hand, is never footnote text.
func TestFootnoteTextUnknownParagraph(t *testing.T) {
	ctx := NewContext(nil)
	ctx.AllDefinitions = []*ReferenceDefinition{{Label: "^1", Lines: [2]int{1, 1}}, {Label: "y", Lines: [2]int{2, 2}}}
	if text := FootnoteText(ctx); len(text) != 0 {
		t.Errorf("FootnoteText = %v, want none", text)
	}
}

// TestMicromarkLabel checks which labels micromark reads: any that holds a
// character other than a space, a tab or a line ending.
func TestMicromarkLabel(t *testing.T) {
	for label, want := range map[string]bool{
		"": false, " ": false, "\t": false, " \r\n ": false,
		"\f": true, "\v": true, "\xc2\xa0": true, "x": true, " x ": true,
	} {
		if got := MicromarkLabel(label); got != want {
			t.Errorf("MicromarkLabel(%q) = %v, want %v", label, got, want)
		}
	}
}

// TestMarkdownlintDefinitions checks the definitions keyed as markdownlint
// keys them: the first definition of each key, with the destination as
// markdownlint reads it, which is empty in angle brackets.
func TestMarkdownlintDefinitions(t *testing.T) {
	content := "Text.\n\n[X]: /first\n[x]: /second\n[ Y  z ]: #\n[w]: <#>\n[v]: \\#\n"
	file, err := goldmark.New("gfm").Parse(context.Background(), "test.md", []byte(content))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	definitions := MarkdownlintDefinitions(Collect(file.Root, file))
	want := map[string]string{"x": "/first", "y z": "#", "w": "", "v": "\\#"}
	if len(definitions) != len(want) {
		t.Fatalf("got %d definitions, want %d", len(definitions), len(want))
	}
	for key, destination := range want {
		def, found := definitions[key]
		if !found {
			t.Errorf("no definition for %q", key)
			continue
		}
		if got := MarkdownlintDestination(def); got != destination {
			t.Errorf("destination of %q = %q, want %q", key, got, destination)
		}
	}
}

func TestCollect_Definitions(t *testing.T) {
	markdown := "Para text\n[notadef]: /in-paragraph\n\n" +
		"> [quoted]: /quoted \"Quoted title\"\n" +
		"- [listed]: </listed>\n\n" +
		"[multi\nline]:\n  /multi\n  'Multi title'\n" +
		"[next]: /next (Paren title)\n" +
		"[raw]: https://x.example/a>\n" +
		"[bad]: /dest trailing\n"
	file, err := goldmark.New("commonmark").Parse(context.Background(), "test.md", []byte(markdown))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	type want struct {
		label, destination, title string
		lines                     [2]int
		removable                 bool
	}
	expected := []want{
		{"quoted", "/quoted", "Quoted title", [2]int{4, 4}, true},
		{"listed", "/listed", "", [2]int{5, 5}, false},
		{"multi\nline", "/multi", "Multi title", [2]int{7, 10}, true},
		{"next", "/next", "Paren title", [2]int{11, 11}, true},
		{"raw", "https://x.example/a>", "", [2]int{12, 12}, true},
	}
	defs := Collect(file.Root, file).AllDefinitions
	if len(defs) != len(expected) {
		var labels []string
		for _, def := range defs {
			labels = append(labels, def.Label)
		}
		t.Fatalf("got definitions %q, want %d", labels, len(expected))
	}
	for idx, exp := range expected {
		def := defs[idx]
		got := want{def.Label, def.Destination, def.Title, def.Lines, def.Removable}
		if got != exp {
			t.Errorf("definition %d = %+v, want %+v", idx, got, exp)
		}
	}
	if defs[2].NormalizedLabel != "multi line" {
		t.Errorf("NormalizedLabel = %q, want %q", defs[2].NormalizedLabel, "multi line")
	}
}

func TestNormalizeLabel_UnicodeCaseFolding(t *testing.T) {
	if NormalizeLabel("Straße") != NormalizeLabel("STRASSE") {
		t.Errorf("NormalizeLabel(%q) = %q, NormalizeLabel(%q) = %q, want equal",
			"Straße", NormalizeLabel("Straße"), "STRASSE", NormalizeLabel("STRASSE"))
	}
}

func TestAnchorMap_DuplicateHandling(t *testing.T) {
	m := NewAnchorMap()

	// Generate same heading multiple times
	first := m.GenerateAnchor("Hello World")
	second := m.GenerateAnchor("Hello World")
	third := m.GenerateAnchor("Hello World")

	if first != "hello-world" {
		t.Errorf("first anchor = %q, want %q", first, "hello-world")
	}
	if second != "hello-world-1" {
		t.Errorf("second anchor = %q, want %q", second, "hello-world-1")
	}
	if third != "hello-world-2" {
		t.Errorf("third anchor = %q, want %q", third, "hello-world-2")
	}
}

func TestAnchorMap_Lookup(t *testing.T) {
	anchorMap := NewAnchorMap()

	pos := mdast.SourcePosition{StartLine: 1, EndLine: 1}
	anchorMap.AddFromHeading("Hello World", pos)

	// Test Has
	if !anchorMap.Has("hello-world") {
		t.Error("Has('hello-world') = false, want true")
	}
	if anchorMap.Has("nonexistent") {
		t.Error("Has('nonexistent') = true, want false")
	}

	// Test HasIgnoreCase
	if !anchorMap.HasIgnoreCase("HELLO-WORLD") {
		t.Error("HasIgnoreCase('HELLO-WORLD') = false, want true")
	}

	// Test Lookup
	anchor := anchorMap.Lookup("hello-world")
	if anchor == nil {
		t.Fatal("Lookup('hello-world') = nil")
	}
	if anchor.Text != "Hello World" {
		t.Errorf("anchor.Text = %q, want %q", anchor.Text, "Hello World")
	}

	// Test LookupIgnoreCase
	anchor = anchorMap.LookupIgnoreCase("HELLO-WORLD")
	if anchor == nil {
		t.Fatal("LookupIgnoreCase('HELLO-WORLD') = nil")
	}
}

func TestAnchorMap_Count(t *testing.T) {
	anchorMap := NewAnchorMap()

	pos := mdast.SourcePosition{StartLine: 1, EndLine: 1}
	anchorMap.AddFromHeading("First", pos)
	anchorMap.AddFromHeading("Second", pos)
	anchorMap.AddFromHeading("First", pos) // duplicate

	if anchorMap.Count() != 3 { // first, second, first-1
		t.Errorf("Count() = %d, want 3", anchorMap.Count())
	}
}

func TestReferenceUsage_IsSameFileFragment(t *testing.T) {
	tests := []struct {
		name        string
		destination string
		fragment    string
		expected    bool
	}{
		{"same-file fragment", "#section", "#section", true},
		{"same-file fragment with hyphen", "#my-heading", "#my-heading", true},
		{"external URL with fragment", "https://example.com/#section", "#section", false},
		{"local file with fragment", "./file.md#section", "#section", false},
		{"absolute path with fragment", "/path/file.md#section", "#section", false},
		{"no fragment", "https://example.com", "", false},
		{"empty destination with fragment", "", "#section", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := &ReferenceUsage{Destination: tt.destination, Fragment: tt.fragment}
			got := usage.IsSameFileFragment()
			if got != tt.expected {
				t.Errorf("IsSameFileFragment() = %v, want %v for dest=%q frag=%q", got, tt.expected, tt.destination, tt.fragment)
			}
		})
	}
}

func TestCollect_HeadingWithCodeSpan(t *testing.T) {
	// Test that headings with inline code spans are properly extracted for anchor generation
	content := []byte("### Using `go install`\n\nSome text.\n")

	parser := goldmark.New("gfm")
	file, err := parser.Parse(context.Background(), "test.md", content)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	ctx := Collect(file.Root, file)

	// Should generate anchor "using-go-install" (including code span text)
	if !ctx.Anchors.Has("using-go-install") {
		t.Errorf("Expected anchor 'using-go-install' to exist")
		t.Errorf("Available anchors:")
		for _, a := range ctx.Anchors.All() {
			t.Errorf("  %q (from: %q)", a.ID, a.Text)
		}
	}
}

func TestCollect_HeadingWithMultipleCodeSpans(t *testing.T) {
	// Test heading with multiple inline elements
	content := []byte("## Using `stave` in your CI\n\nText.\n")

	parser := goldmark.New("gfm")
	file, err := parser.Parse(context.Background(), "test.md", content)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	ctx := Collect(file.Root, file)

	// Should generate anchor including "stave" from code span
	if !ctx.Anchors.Has("using-stave-in-your-ci") {
		t.Errorf("Expected anchor 'using-stave-in-your-ci' to exist")
		t.Errorf("Available anchors:")
		for _, a := range ctx.Anchors.All() {
			t.Errorf("  %q (from: %q)", a.ID, a.Text)
		}
	}
}

func TestCollect_IgnoresCodeBlockContent(t *testing.T) {
	// This test verifies that reference definition patterns inside code blocks
	// are not mistakenly detected as markdown reference definitions.
	// This is a regression test for a bug where JavaScript computed property syntax
	// like [SemanticResourceAttributes.SERVICE_NAME]: 'value' was being detected
	// as a markdown reference definition.

	// Create content with a code block containing reference-like syntax
	// The pattern [label]: destination looks like a ref def but is inside a code block
	content := []byte(`# Test

` + "```javascript" + `
const resource = {
  [key]: 'value',
};
` + "```" + `

[real-ref]: https://example.com
`)

	// Parse the content to get a proper AST with token positions
	parser := goldmark.New("gfm")
	file, err := parser.Parse(context.Background(), "test.md", content)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	// Collect references
	ctx := Collect(file.Root, file)

	// Should find exactly one definition: [real-ref] on line 10
	// The [key]: 'value' inside the code block should be ignored
	if len(ctx.AllDefinitions) != 1 {
		t.Errorf("Expected 1 definition, got %d", len(ctx.AllDefinitions))
		for _, def := range ctx.AllDefinitions {
			t.Errorf("  Found definition at line %d: [%s]", def.LineNumber, def.Label)
		}
		return
	}

	def := ctx.AllDefinitions[0]
	if def.Label != "real-ref" {
		t.Errorf("Expected label 'real-ref', got %q", def.Label)
	}
	if def.LineNumber != 9 {
		t.Errorf("Expected line 9, got %d", def.LineNumber)
	}
}

func TestCollect_DefinitionAfterByteOrderMark(t *testing.T) {
	// goldmark parses the content after a byte order mark, so a definition on
	// the first line resolves links. The collector must find it there too.
	content := []byte("\xEF\xBB\xBF[foo]: /u\n\n[x][foo]\n")
	file, err := goldmark.New("gfm").Parse(context.Background(), "test.md", content)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	ctx := Collect(file.Root, file)

	if len(ctx.AllDefinitions) != 1 {
		t.Fatalf("Expected 1 definition, got %d", len(ctx.AllDefinitions))
	}
	def := ctx.AllDefinitions[0]
	if def.Label != "foo" || def.Destination != "/u" || def.LineNumber != 1 {
		t.Errorf("Expected [foo]: /u on line 1, got [%s]: %s on line %d", def.Label, def.Destination, def.LineNumber)
	}
	if len(ctx.Usages) != 1 || ctx.Usages[0].ResolvedDefinition != def {
		t.Errorf("Expected [x][foo] to resolve to the definition on line 1")
	}
}

// TestJSToLower mirrors JavaScript's String.prototype.toLowerCase, which
// github-slugger and markdownlint use: full case mapping, so U+0130 becomes
// "i" and a combining dot above, and a capital sigma at the end of a word
// becomes the final sigma.
func TestJSToLower(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Hello World", "hello world"},
		{"İ", "i̇"},
		{"ΟΣ", "ος"},
		{"ΣΑ", "σα"},
		{"Σ", "σ"},
		{"A.Σ", "a.ς"},
		{"ΟΔΟΣ ΚΑΙ ΣΟΦΙΑ", "οδος και σοφια"},
		{"ΑΣ́", "ας́"},
		{"Σ́Α", "σ́α"},
		{"x Σ y", "x σ y"},
		{"ǅ", "ǆ"},
		{"ẞ", "ß"},
		{"AΣͅ", "aςͅ"},
		{"Σ.", "σ."},
		{"AΣ.B", "aσ.b"},
	}
	for _, tt := range tests {
		if got := jsToLower(tt.in); got != tt.want {
			t.Errorf("jsToLower(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGenerateAnchor_JavaScriptLowercase(t *testing.T) {
	tests := []struct {
		text, want string
	}{
		{"İ", "i̇"},
		{"ΟΣ", "ος"},
	}
	for _, tt := range tests {
		if got := NewAnchorMap().GenerateAnchor(tt.text); got != tt.want {
			t.Errorf("GenerateAnchor(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}

// TestBlankLabel checks which labels markdownlint never reports: those of
// nothing but ASCII white space, which a no-break space is not.
func TestBlankLabel(t *testing.T) {
	tests := []struct {
		label string
		want  bool
	}{
		{label: "", want: true},
		{label: " ", want: true},
		{label: "\t", want: true},
		{label: "\n", want: true},
		{label: "\r", want: true},
		{label: "\v", want: true},
		{label: "\f", want: true},
		{label: " \f \v\t", want: true},
		{label: "x", want: false},
		{label: " x ", want: false},
		{label: "\xc2\xa0", want: false},
		{label: "^", want: false},
	}
	for _, tt := range tests {
		t.Run(strconv.Quote(tt.label), func(t *testing.T) {
			if got := BlankLabel(tt.label); got != tt.want {
				t.Errorf("BlankLabel(%q) = %v, want %v", tt.label, got, tt.want)
			}
		})
	}
}

// TestHTMLAnchor checks the anchor that an HTML tag names, as markdownlint
// 0.41.1 reads it in MD051: an opening tag names the value of its first id
// attribute, in any case and quoted or not, and an "a" tag without one the
// value of its first name attribute. Closing tags, comments and
// declarations name none, and neither does an empty value.
func TestHTMLAnchor(t *testing.T) {
	tests := []struct {
		tag    string
		id     string
		source AnchorSource
		ok     bool
	}{
		{tag: `<a id="x">`, id: "x", source: AnchorFromHTMLID, ok: true},
		{tag: `<div ID='x'>`, id: "x", source: AnchorFromHTMLID, ok: true},
		{tag: `<span id=x>`, id: "x", source: AnchorFromHTMLID, ok: true},
		{tag: "<a\nid=\"x\">", id: "x", source: AnchorFromHTMLID, ok: true},
		{tag: "<a\n> id=\"x\">", id: "x", source: AnchorFromHTMLID, ok: true},
		{tag: "<a\vid=\"x\">", id: "x", source: AnchorFromHTMLID, ok: true},
		{tag: "<a\u00a0id=\"x\">", id: "x", source: AnchorFromHTMLID, ok: true},
		{tag: `<a id = "x y">`, id: "x", source: AnchorFromHTMLID, ok: true},
		{tag: `<a name="x">`, id: "x", source: AnchorFromHTMLName, ok: true},
		{tag: `<A NAME='x'>`, id: "x", source: AnchorFromHTMLName, ok: true},
		{tag: `<a name="x" id="y">`, id: "y", source: AnchorFromHTMLID, ok: true},
		{tag: `<a id="" name="x">`},
		{tag: `<div name="x">`},
		{tag: `<a data-id="x">`},
		{tag: `<a/id="x">`},
		{tag: `</a id="x">`},
		{tag: `<!-- id="x" -->`},
		{tag: `<!DOCTYPE html id="x">`},
		{tag: `<a href="#x">`},
		{tag: `<abbr name="x">`},
		{tag: `<![CDATA[ id="x" ]]>`},
		// markdownlint reads a processing instruction's text and a quoted
		// value as it reads attributes.
		{tag: `<?php id="x" ?>`, id: "x", source: AnchorFromHTMLID, ok: true},
		{tag: `<a title="foo id=bar">`, id: "bar", source: AnchorFromHTMLID, ok: true},
	}
	for _, tt := range tests {
		t.Run(strconv.Quote(tt.tag), func(t *testing.T) {
			id, source, ok := HTMLAnchor([]byte(tt.tag))
			if id != tt.id || source != tt.source || ok != tt.ok {
				t.Errorf("HTMLAnchor(%q) = %q, %v, %v; want %q, %v, %v", tt.tag, id, source, ok, tt.id, tt.source, tt.ok)
			}
		})
	}
}

// TestCollect_HTMLBlockAnchors checks that every line of an HTML block can
// name an anchor. Without a reader for HTML blocks, the collector reads
// every HTML construct in a block's source; with one, only the ranges it
// gives. Inline HTML is read whole, over all of its lines.
func TestCollect_HTMLBlockAnchors(t *testing.T) {
	content := []byte("# Title\n\n<div>\n<a name=\"later\"></a>\n`<a id=\"code\">`\n</div>\n\n" +
		"Text <a\nid=\"spread\">x</a>.\n")
	file, err := goldmark.New("gfm").Parse(context.Background(), "test.md", content)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	lexical := Collect(file.Root, file)
	for _, id := range []string{"later", "code", "spread"} {
		if !lexical.Anchors.Has(id) {
			t.Errorf("Collect: anchor %q not found", id)
		}
	}

	later := bytes.Index(content, []byte("<a name"))
	tag := mdast.SourceRange{StartOffset: later, EndOffset: later + len(`<a name="later">`)}
	hooked := CollectWithOptions(file.Root, file, Options{
		HTMLBlockTags: func(*mdast.Node) []mdast.SourceRange { return []mdast.SourceRange{tag} },
	})
	if !hooked.Anchors.Has("later") || !hooked.Anchors.Has("spread") {
		t.Error("CollectWithOptions: anchors later and spread not found")
	}
	if hooked.Anchors.Has("code") {
		t.Error("CollectWithOptions: anchor code found outside the ranges HTMLBlockTags gives")
	}
	anchor := hooked.Anchors.Lookup("later")
	if anchor == nil || anchor.Source != AnchorFromHTMLName || anchor.Position.StartLine != 4 || anchor.Position.StartColumn != 1 {
		t.Errorf("anchor later = %+v, want an HTML name anchor at 4:1", anchor)
	}
}
