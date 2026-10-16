package rules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// markerExpectation describes the marker expected for one list item, in
// document order.
type markerExpectation struct {
	line      int
	column    int
	text      string
	spacing   int
	ordered   bool
	number    int
	delimiter byte
}

func TestFindListItemMarker(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []markerExpectation
	}{
		{
			name:  "bullet at line start",
			input: "- item\n",
			want:  []markerExpectation{{line: 1, column: 0, text: "-", spacing: 1}},
		},
		{
			name:  "indented asterisk",
			input: "  * item\n",
			want:  []markerExpectation{{line: 1, column: 2, text: "*", spacing: 1}},
		},
		{
			name:  "ordered with period",
			input: "1. item\n",
			want: []markerExpectation{
				{line: 1, column: 0, text: "1.", spacing: 1, ordered: true, number: 1, delimiter: '.'},
			},
		},
		{
			name:  "ordered with parenthesis",
			input: "10) item\n",
			want: []markerExpectation{
				{line: 1, column: 0, text: "10)", spacing: 1, ordered: true, number: 10, delimiter: ')'},
			},
		},
		{
			name:  "leading zeros",
			input: "007. item\n",
			want: []markerExpectation{
				{line: 1, column: 0, text: "007.", spacing: 1, ordered: true, number: 7, delimiter: '.'},
			},
		},
		{
			name:  "extra spacing",
			input: "-   spaced\n",
			want:  []markerExpectation{{line: 1, column: 0, text: "-", spacing: 3}},
		},
		{
			name:  "tab after marker",
			input: "-\titem\n",
			want:  []markerExpectation{{line: 1, column: 0, text: "-", spacing: 1}},
		},
		{
			name:  "inside blockquote",
			input: "> - item\n",
			want:  []markerExpectation{{line: 1, column: 2, text: "-", spacing: 1}},
		},
		{
			name:  "inside nested blockquote",
			input: "> > 3. item\n",
			want: []markerExpectation{
				{line: 1, column: 4, text: "3.", spacing: 1, ordered: true, number: 3, delimiter: '.'},
			},
		},
		{
			name:  "ordered list nested on the bullet line",
			input: "- 1. alpha\n  2. beta\n",
			want: []markerExpectation{
				{line: 1, column: 0, text: "-", spacing: 1},
				{line: 1, column: 2, text: "1.", spacing: 1, ordered: true, number: 1, delimiter: '.'},
				{line: 2, column: 2, text: "2.", spacing: 1, ordered: true, number: 2, delimiter: '.'},
			},
		},
		{
			name:  "bullet nested on the bullet line",
			input: "- - x\n",
			want: []markerExpectation{
				{line: 1, column: 0, text: "-", spacing: 1},
				{line: 1, column: 2, text: "-", spacing: 1},
			},
		},
		{
			name:  "loose list items",
			input: "* one\n\n* two\n",
			want: []markerExpectation{
				{line: 1, column: 0, text: "*", spacing: 1},
				{line: 3, column: 0, text: "*", spacing: 1},
			},
		},
		{
			name:  "right aligned numbers",
			input: " 9. nine\n10. ten\n",
			want: []markerExpectation{
				{line: 1, column: 1, text: "9.", spacing: 1, ordered: true, number: 9, delimiter: '.'},
				{line: 2, column: 0, text: "10.", spacing: 1, ordered: true, number: 10, delimiter: '.'},
			},
		},
		{
			name:  "content starting with escaped marker",
			input: "- \\- not a list\n",
			want:  []markerExpectation{{line: 1, column: 0, text: "-", spacing: 1}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			items := mdast.FindByKind(snapshot.Root, mdast.NodeListItem)
			require.Len(t, items, len(tt.want))
			for idx, item := range items {
				marker, ok := findListItemMarker(snapshot, item)
				require.True(t, ok, "item %d marker not found", idx)
				want := tt.want[idx]
				assert.Equal(t, want.line, marker.Line, "item %d line", idx)
				assert.Equal(t, want.column, marker.Column(), "item %d column", idx)
				assert.Equal(t, want.text, string(snapshot.Content[marker.Start:marker.End]), "item %d text", idx)
				assert.Equal(t, want.spacing, marker.Spacing(), "item %d spacing", idx)
				assert.Equal(t, want.ordered, marker.Ordered, "item %d ordered", idx)
				if want.ordered {
					assert.Equal(t, want.number, marker.Number, "item %d number", idx)
					assert.Equal(t, want.delimiter, marker.Delimiter, "item %d delimiter", idx)
				}
			}
		})
	}
}

func TestFindListItemMarkerRejectsNonItems(t *testing.T) {
	snapshot, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte("Text\n"))
	require.NoError(t, err)
	paragraphs := lint.Paragraphs(snapshot.Root)
	require.Len(t, paragraphs, 1)
	_, ok := findListItemMarker(snapshot, paragraphs[0])
	assert.False(t, ok)
	_, ok = findListItemMarker(nil, nil)
	assert.False(t, ok)
}
