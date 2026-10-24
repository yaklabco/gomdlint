package ruledocs

import (
	"html"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMarkdownFenceOutlastsTheContent covers a code block whose content
// holds backticks, such as an example with a code fence: the fence around
// it is longer than any run of backticks in it, and the block renders the
// content as it is.
func TestMarkdownFenceOutlastsTheContent(t *testing.T) {
	for _, tt := range []struct {
		content string
		want    string
	}{
		{content: "# Title\n", want: "```markdown\n# Title\n```"},
		{content: "# Title", want: "```markdown\n# Title\n```"},
		{content: "Use `x`.\n", want: "```markdown\nUse `x`.\n```"},
		{content: "```go\nx\n```\n", want: "````markdown\n```go\nx\n```\n````"},
		{content: "a ````` b\n", want: "``````markdown\na ````` b\n``````"},
	} {
		got := markdownFence("markdown", tt.content)

		assert.Equal(t, tt.want, got, "%q", tt.content)
		content := strings.TrimSuffix(tt.content, "\n") + "\n"
		assert.Equal(t, "<pre><code class=\"language-markdown\">"+html.EscapeString(content)+"</code></pre>\n",
			renderGFM(t, got+"\n"), "%q", tt.content)
	}
}
