package goldmark_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/parser"

	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// firstLink returns the first link node of a snapshot, or nil.
func firstLink(snapshot *mdast.FileSnapshot) *mdast.Node {
	var found *mdast.Node
	//nolint:errcheck // The visitor never returns an error.
	_ = mdast.Walk(snapshot.Root, func(node *mdast.Node) error {
		if found == nil && node.Kind == mdast.NodeLink {
			found = node
		}
		return nil
	})
	return found
}

// TestParseWithReferences checks that links resolve through preset
// definitions before the definitions the content holds, and that a
// reference to neither stays text.
func TestParseWithReferences(t *testing.T) {
	refs := map[string]parser.Reference{
		"r": parser.NewReference([]byte("R"), []byte("/preset"), []byte("Preset")),
	}
	mdParser := goldmark.New(goldmark.FlavorGFM)

	snapshot, err := mdParser.ParseWithReferences(context.Background(), "part.md", []byte("See [x][r].\n"), refs)
	require.NoError(t, err)
	link := firstLink(snapshot)
	require.NotNil(t, link, "the reference resolves through the preset definition")
	assert.Equal(t, "/preset", link.Inline.Link.Destination)
	assert.Equal(t, "Preset", link.Inline.Link.Title)

	snapshot, err = mdParser.ParseWithReferences(context.Background(), "part.md",
		[]byte("See [x][r].\n\n[r]: /local\n"), refs)
	require.NoError(t, err)
	link = firstLink(snapshot)
	require.NotNil(t, link)
	assert.Equal(t, "/preset", link.Inline.Link.Destination, "the preset definition comes first")

	snapshot, err = mdParser.ParseWithReferences(context.Background(), "part.md", []byte("See [x][other].\n"), refs)
	require.NoError(t, err)
	assert.Nil(t, firstLink(snapshot), "a reference to no definition stays text")

	plain, err := mdParser.Parse(context.Background(), "part.md", []byte("See [x][r].\n"))
	require.NoError(t, err)
	assert.Nil(t, firstLink(plain), "Parse presets nothing")
}
