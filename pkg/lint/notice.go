package lint

// NoticeKind classifies a Notice.
type NoticeKind string

// The kinds of notices.
const (
	// NoticeUnknownRule marks a word in a markdownlint comment that names no
	// rule and no tag. The comment does nothing for the word, as in
	// markdownlint.
	NoticeUnknownRule NoticeKind = "unknown-rule"

	// NoticeDeprecatedTag marks a word in a markdownlint comment that is one
	// of gomdlint's former tags: style, lists, layout, metadata, gfm or
	// tables, which markdownlint does not have. The word still names the
	// rules that report it themselves (Rule.Tags), but will stop working in
	// a later release.
	NoticeDeprecatedTag NoticeKind = "deprecated-tag"
)

// Notice reports a problem in a file that changes neither its findings nor
// the exit code: a word in a markdownlint comment that names no rule or tag,
// or that is one of gomdlint's deprecated former tags.
type Notice struct {
	// Kind classifies the problem.
	Kind NoticeKind

	// Line is the 1-based line on which the comment starts.
	Line int

	// Directive is the kind of comment, in lower case, such as
	// markdownlint-disable.
	Directive string

	// Name is the word as the comment writes it.
	Name string

	// Suggestions are, for an unknown word, the rules and tags it most
	// likely means, any one of which may be meant (see Registry.Closest).
	// There are none when nothing is close enough, or when too many are.
	Suggestions []string

	// Rules are, for a deprecated tag, the names of the rules it covers,
	// which the comment can name instead, in rule ID order.
	Rules []string

	// Message describes the problem in a sentence that starts with
	// Directive, as in `markdownlint-disable: unknown rule "x"; it is
	// ignored`.
	Message string
}
