package lint

// NoticeKind classifies a Notice.
type NoticeKind string

// The kinds of notices.
const (
	// NoticeUnknownRule marks a word in a markdownlint comment, or a key of
	// a markdownlint-configure-file comment, that names no rule and no tag.
	// The comment does nothing for the word, as in markdownlint.
	NoticeUnknownRule NoticeKind = "unknown-rule"

	// NoticeDeprecatedTag marks a word in a markdownlint comment that is one
	// of gomdlint's former tags: style, lists, layout, metadata, gfm or
	// tables, which markdownlint does not have. The word still names the
	// rules that report it themselves (Rule.Tags), but will stop working in
	// a later release.
	NoticeDeprecatedTag NoticeKind = "deprecated-tag"

	// NoticeUnknownOption marks an option in the setting of a
	// markdownlint-configure-file key that the rule the key names does not
	// read, or, for a tag, that none of its rules reads. The comment does
	// nothing with the option, as in markdownlint, and applies the rest of
	// the setting. gomdlint's own spelling of an option that markdownlint
	// spells otherwise, such as max for line_length, and an option that
	// only gomdlint has are such options, since markdownlint does not read
	// them.
	NoticeUnknownOption NoticeKind = "unknown-option"

	// NoticeInvalidConfiguration marks a markdownlint-configure-file
	// comment whose parameter holds no object of settings: text that is
	// neither JSON, with comments and trailing commas, nor YAML, or a value
	// other than an object, such as a list. The comment changes nothing,
	// as in markdownlint.
	NoticeInvalidConfiguration NoticeKind = "invalid-configuration"

	// NoticeUnreadConfiguration marks a markdownlint-configure-file comment
	// that gomdlint does not read: one without a "-->" of its own, which runs
	// on to that of a later comment, in a file whose comments like it hold
	// more text than gomdlint reads (see configureFile). markdownlint reads
	// it, but the comment changes nothing in gomdlint.
	NoticeUnreadConfiguration NoticeKind = "unread-configuration"
)

// Notice reports a problem in a file that changes neither its findings nor
// the exit code: a word in a markdownlint comment that names no rule or tag,
// or that is one of gomdlint's deprecated former tags, or a
// markdownlint-configure-file comment that markdownlint ignores, in whole
// or in part.
type Notice struct {
	// Kind classifies the problem.
	Kind NoticeKind

	// Line is the 1-based line on which the comment starts.
	Line int

	// Directive is the kind of comment, in lower case, such as
	// markdownlint-disable.
	Directive string

	// Name is the word, the configure-file key or, for an unknown option,
	// the option, as the comment writes it. It is empty for a
	// configure-file comment that holds no settings
	// (NoticeInvalidConfiguration), or that gomdlint does not read
	// (NoticeUnreadConfiguration).
	Name string

	// Suggestions are, for an unknown word, the rules and tags it most
	// likely means, any one of which may be meant (see Registry.Closest).
	// There are none when nothing is close enough, or when too many are.
	// Rules are named as markdownlint names them (RuleMeta.MarkdownlintName),
	// so that a comment written from them works in markdownlint too. For an
	// unknown option of one rule, they are the option names it most likely
	// means: markdownlint's spelling of a name of gomdlint's, or else the
	// closest of markdownlint's spellings.
	Suggestions []string

	// Rules are, for a deprecated tag, the names of the rules it covers,
	// which the comment can name instead, and, for an unknown option, the
	// rules whose setting holds it, none of which reads it, in rule ID
	// order, named as markdownlint names them.
	Rules []string

	// Message describes the problem in a sentence that starts with
	// Directive, as in `markdownlint-disable: unknown rule "x"; it is
	// ignored`.
	Message string
}
