package mdscan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchHTML(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantOK  bool
		wantEnd int
		want    HTMLConstruct
	}{
		{name: "open tag", src: `<div class="x">rest`, wantOK: true, want: HTMLConstruct{End: 15, Name: "div"}},
		{name: "self closing", src: `<br/>`, wantOK: true, want: HTMLConstruct{End: 5, Name: "br"}},
		{name: "unquoted and boolean attributes", src: `<input disabled value=x>`, wantOK: true, want: HTMLConstruct{End: 24, Name: "input"}},
		{name: "attribute across lines", src: "<a\n  href='x'\n>", wantOK: true, want: HTMLConstruct{End: 15, Name: "a"}},
		{name: "closing tag", src: `</span >`, wantOK: true, want: HTMLConstruct{End: 8, Name: "span", Closing: true}},
		{name: "comment", src: `<!-- <b> -->`, wantOK: true, want: HTMLConstruct{End: 12}},
		{name: "empty comment", src: `<!-->`, wantOK: true, want: HTMLConstruct{End: 5}},
		{name: "declaration", src: `<!DOCTYPE html>`, wantOK: true, want: HTMLConstruct{End: 15}},
		{name: "cdata", src: `<![CDATA[ <b> ]]>`, wantOK: true, want: HTMLConstruct{End: 17}},
		{name: "processing instruction", src: `<?xml version="1.0"?>`, wantOK: true, want: HTMLConstruct{End: 21, Name: "?xml"}},
		{name: "space after angle bracket", src: `< div>`},
		{name: "unterminated tag", src: `<div class="x"`},
		{name: "equals without value", src: `<a href=>`},
		{name: "unterminated comment", src: `<!-- open`},
		{name: "digit tag name", src: `<1>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := MatchHTML([]byte(tt.src), 0)
			require.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestScanHTMLFindsEveryConstruct(t *testing.T) {
	src := []byte("x <div><span>a < b</span><!-- <i> --></div> y")
	constructs := ScanHTML(src, 0, len(src))
	names := make([]string, 0, len(constructs))
	for _, construct := range constructs {
		if construct.IsOpeningElement() {
			names = append(names, construct.Name)
		}
	}
	assert.Equal(t, []string{"div", "span"}, names)
	assert.Len(t, constructs, 5)
}

func TestScanHTMLExtendsOnlyLeadingConstruct(t *testing.T) {
	src := []byte("<!--\n<b>\n-->\n")
	constructs := ScanHTML(src, 0, len("<!--\n<b>\n"))
	require.Len(t, constructs, 1)
	assert.Equal(t, len(src)-1, constructs[0].End)

	src = []byte("<div>\n<!-- open\n<b>x</b>\n")
	names := []string{}
	for _, construct := range ScanHTML(src, 0, len(src)) {
		if construct.IsOpeningElement() {
			names = append(names, construct.Name)
		}
	}
	assert.Equal(t, []string{"div", "b"}, names)
}

func TestCodeSpanEnd(t *testing.T) {
	end, ok := CodeSpanEnd([]byte("`a` b"), 0)
	require.True(t, ok)
	assert.Equal(t, 3, end)

	end, ok = CodeSpanEnd([]byte("`` a`b `` c"), 0)
	require.True(t, ok)
	assert.Equal(t, 9, end)

	_, ok = CodeSpanEnd([]byte("`` a ` b"), 0)
	assert.False(t, ok)
}

func TestMatchAutolink(t *testing.T) {
	for _, src := range []string{"<https://example.com/a?b=c>", "<mailto:x@y.z>", "<user@example.com>"} {
		end, ok := MatchAutolink([]byte(src), 0)
		assert.True(t, ok, src)
		assert.Equal(t, len(src), end, src)
	}
	for _, src := range []string{"<https://exa mple.com>", "<a>", "<x:>"} {
		_, ok := MatchAutolink([]byte(src), 0)
		assert.False(t, ok, src)
	}
}

func TestLinkTextEnd(t *testing.T) {
	tests := []struct {
		src  string
		want int
	}{
		{"[text](url)", 5},
		{"[a [b] c](u)", 8},
		{"[a \\] b](u)", 7},
		{"[a `]` b](u)", 8},
		{"[a <span title=\"]\">b</span>](u)", 27},
		{"[a <https://x.y/]> b](u)", 20},
	}
	for _, tt := range tests {
		got, ok := LinkTextEnd([]byte(tt.src), 0)
		require.True(t, ok, tt.src)
		assert.Equal(t, tt.want, got, tt.src)
		assert.Equal(t, byte(']'), tt.src[got], tt.src)
	}
	_, ok := LinkTextEnd([]byte("[never closed"), 0)
	assert.False(t, ok)
}

func TestLinkLabelEnd(t *testing.T) {
	got, ok := LinkLabelEnd([]byte("[label] rest"), 0)
	require.True(t, ok)
	assert.Equal(t, 6, got)

	got, ok = LinkLabelEnd([]byte(`[a\]b]`), 0)
	require.True(t, ok)
	assert.Equal(t, 5, got)

	for _, src := range []string{"[]", "[  ]", "[a[b]", "[open"} {
		_, ok := LinkLabelEnd([]byte(src), 0)
		assert.False(t, ok, src)
	}
}

func TestIsEscaped(t *testing.T) {
	src := []byte(`a\[b\\[c`)
	assert.True(t, IsEscaped(src, 2))
	assert.False(t, IsEscaped(src, 6))
}
