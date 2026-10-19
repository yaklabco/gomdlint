package config

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// parseNode parses content into a YAML document node.
func parseNode(t *testing.T, content string) *yaml.Node {
	t.Helper()

	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(content), &doc))
	return &doc
}

// TestExpandAliasKeysKeepsANodeWithoutAliasKeys covers the usual document,
// which writes no key as an alias, though it may hold aliases elsewhere:
// ExpandAliasKeys returns it as it is, with no copy.
func TestExpandAliasKeysKeepsANodeWithoutAliasKeys(t *testing.T) {
	t.Parallel()

	for _, content := range []string{
		"rules:\n  MD013: false\n",
		"base: &base {max: 1}\nrules:\n  MD013: *base\n  MD009:\n    <<: *base\n",
		"",
	} {
		doc := parseNode(t, content)
		assert.Same(t, doc, ExpandAliasKeys(doc), content)
	}
}

// TestExpandAliasKeysWritesAliasKeysOut covers a document that writes keys as
// aliases. The copy holds a copy of the node that each alias key names, at
// the alias's place and without the anchor, every other alias names the
// copy of its node, and the document itself is left as it was.
func TestExpandAliasKeysWritesAliasKeysOut(t *testing.T) {
	t.Parallel()

	doc := parseNode(t, "ignore: [&id MD013]\nrules:\n  *id : false\n  self: &self {*id : *self}\n")
	written := ExpandAliasKeys(doc)
	require.NotSame(t, doc, written)

	rules := written.Content[0].Content[3]
	key := rules.Content[0]
	assert.Equal(t, yaml.ScalarNode, key.Kind)
	assert.Equal(t, "MD013", key.Value)
	assert.Empty(t, key.Anchor)
	assert.Equal(t, 3, key.Line)
	assert.Equal(t, 3, key.Column)

	self := rules.Content[3]
	assert.Equal(t, "MD013", self.Content[0].Value, "a key inside a node that contains itself is written out too")
	assert.Same(t, self, self.Content[1].Alias, "an alias names the copy of its node")

	assert.Equal(t, yaml.AliasNode, doc.Content[0].Content[3].Content[0].Kind, "the document keeps its alias keys")
}

// TestExpandAliasKeysKeepsTheCommentsOfAnAliasKey covers the comments of an
// expanded key: those written at the alias, as the key written out would
// hold them, and not those of the scalar that the alias names.
func TestExpandAliasKeysKeepsTheCommentsOfAnAliasKey(t *testing.T) {
	t.Parallel()

	doc := parseNode(t, "x: &k name # on the anchor\nm:\n  # before the alias\n  *k : 1 # after the alias\n  # after the entry\n")
	alias := doc.Content[0].Content[3].Content[0]
	require.Equal(t, yaml.AliasNode, alias.Kind)

	key := ExpandAliasKeys(doc).Content[0].Content[3].Content[0]
	assert.Equal(t, "name", key.Value)
	assert.Equal(t, "# before the alias", key.HeadComment)
	assert.Empty(t, key.LineComment, "the anchor's comment stays at the anchor")
	assert.Equal(t, "# after the entry", key.FootComment)
	assert.Equal(t, [3]string{alias.HeadComment, alias.LineComment, alias.FootComment},
		[3]string{key.HeadComment, key.LineComment, key.FootComment})
}

// TestExpandAliasKeysLeavesAliasesOfCollectionsAndMergeKeys covers the
// alias keys that stay aliases: one of a collection, which may contain the
// alias, and one of a merge key, which decoding reads as an ordinary key.
// A document whose only alias keys are such is returned as it is.
func TestExpandAliasKeysLeavesAliasesOfCollectionsAndMergeKeys(t *testing.T) {
	t.Parallel()

	for _, content := range []string{
		"x: &m {*m : 1}\n",
		"x: &c [a]\ny:\n  ? *c\n  : 1\n",
		"x: &m <<\ny:\n  *m : {a: 1}\n",
	} {
		doc := parseNode(t, content)
		assert.Same(t, doc, ExpandAliasKeys(doc), content)
	}
}

// TestReachesAliasKeyLooksAtEachNodeOnce covers the cost of the walk: a
// large node that many aliases name is looked at once, not once for each
// alias, so the walk stays linear in the size of the document.
func TestReachesAliasKeyLooksAtEachNodeOnce(t *testing.T) {
	t.Parallel()

	const count = 200
	var text strings.Builder
	text.WriteString("h: &h [" + strings.Repeat("1, ", count) + "1]\nrules:\n")
	for idx := range count {
		fmt.Fprintf(&text, "  r%d: {x: *h}\n", idx)
	}
	doc := parseNode(t, text.String())

	walked := make(map[*yaml.Node]bool)
	require.False(t, reachesAliasKey(doc, walked))
	// The document, its root, h and its value with the items, rules and its
	// mapping, and for each rule its key, its mapping, x and the alias.
	assert.Len(t, walked, 6+count+1+4*count)
}
