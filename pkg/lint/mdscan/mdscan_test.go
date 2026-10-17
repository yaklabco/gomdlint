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

func TestScanHTMLStaysInRange(t *testing.T) {
	src := []byte("<!--\n<b>\n-->\n")
	constructs := ScanHTML(src, 0, len(src))
	require.Len(t, constructs, 1)
	assert.Equal(t, len(src)-1, constructs[0].End)

	// A comment that does not end in the range is not a construct, so the
	// markup after its opener is still found.
	src = []byte("<div>\n<!-- open\n<b>x</b>\n")
	names := []string{}
	for _, construct := range ScanHTML(src, 0, len(src)) {
		if construct.IsOpeningElement() {
			names = append(names, construct.Name)
		}
	}
	assert.Equal(t, []string{"div", "b"}, names)
}

func TestIsASCIIPunct(t *testing.T) {
	for _, ch := range []byte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~") {
		assert.True(t, IsASCIIPunct(ch), "%q", ch)
	}
	for _, ch := range []byte("aZ09 \t\n") {
		assert.False(t, IsASCIIPunct(ch), "%q", ch)
	}
}
