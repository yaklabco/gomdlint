package rules

// markdownlintOption is one parameter of a markdownlint rule: its type, as
// markdownlint writes types in its documentation, its default, and the
// values it allows when it allows only some.
type markdownlintOption struct {
	Type    string
	Default any
	Values  []string
}

// markdownlintOptions is markdownlint v0.41.1's rule parameters, keyed by
// rule ID and parameter name, for every rule that markdownlint has.
//
// Source: the rule properties of schema/markdownlint-config-schema.json in
// markdownlint v0.41.1 (npm package markdownlint@0.41.1), which lists each
// parameter of lib/md*.mjs with its type, default and allowed values. The
// schema gives front_matter_title the default ^\s*title\s*[:=], while the
// code applies ^\s*"?title"?\s*[:=] (frontMatterHasTitle in
// helpers/helpers.cjs), and gomdlint follows the code. The schema also gives
// start_indent, heading_line_length, code_block_line_length and
// table_allowed_elements a fixed default, while the code takes the value
// of indent, line_length or allowed_elements when they are not set.
var markdownlintOptions = map[string]map[string]markdownlintOption{
	"MD001": {
		"front_matter_title": {Type: "string", Default: `^\s*title\s*[:=]`},
	},
	"MD003": {
		"style": {Type: "string", Default: "consistent", Values: []string{"consistent", "atx", "atx_closed", "setext", "setext_with_atx", "setext_with_atx_closed"}},
	},
	"MD004": {
		"style": {Type: "string", Default: "consistent", Values: []string{"consistent", "asterisk", "plus", "dash", "sublist"}},
	},
	"MD005": {},
	"MD007": {
		"indent":         {Type: "integer", Default: 2},
		"start_indented": {Type: "boolean", Default: false},
		"start_indent":   {Type: "integer", Default: 2},
	},
	"MD009": {
		"br_spaces":             {Type: "integer", Default: 2},
		"code_blocks":           {Type: "boolean", Default: false},
		"list_item_empty_lines": {Type: "boolean", Default: false},
		"strict":                {Type: "boolean", Default: false},
	},
	"MD010": {
		"code_blocks":           {Type: "boolean", Default: true},
		"ignore_code_languages": {Type: "string[]", Default: []string{}},
		"spaces_per_tab":        {Type: "integer", Default: 1},
	},
	"MD011": {},
	"MD012": {
		"maximum": {Type: "integer", Default: 1},
	},
	"MD013": {
		"line_length":            {Type: "integer", Default: 80},
		"heading_line_length":    {Type: "integer", Default: 80},
		"code_block_line_length": {Type: "integer", Default: 80},
		"code_blocks":            {Type: "boolean", Default: true},
		"tables":                 {Type: "boolean", Default: true},
		"headings":               {Type: "boolean", Default: true},
		"strict":                 {Type: "boolean", Default: false},
		"stern":                  {Type: "boolean", Default: false},
	},
	"MD014": {},
	"MD018": {},
	"MD019": {},
	"MD020": {},
	"MD021": {},
	"MD022": {
		"include_front_matter": {Type: "boolean", Default: false},
		"lines_above":          {Type: "integer|integer[]", Default: 1},
		"lines_below":          {Type: "integer|integer[]", Default: 1},
	},
	"MD023": {},
	"MD024": {
		"siblings_only": {Type: "boolean", Default: false},
	},
	"MD025": {
		"front_matter_title": {Type: "string", Default: `^\s*title\s*[:=]`},
		"level":              {Type: "integer", Default: 1},
	},
	"MD026": {
		"punctuation": {Type: "string", Default: ".,;:!\u3002\uff0c\uff1b\uff1a\uff01"},
	},
	"MD027": {
		"list_items": {Type: "boolean", Default: true},
	},
	"MD028": {},
	"MD029": {
		"style": {Type: "string", Default: "one_or_ordered", Values: []string{"one", "ordered", "one_or_ordered", "zero"}},
	},
	"MD030": {
		"ul_single": {Type: "integer", Default: 1},
		"ol_single": {Type: "integer", Default: 1},
		"ul_multi":  {Type: "integer", Default: 1},
		"ol_multi":  {Type: "integer", Default: 1},
	},
	"MD031": {
		"list_items": {Type: "boolean", Default: true},
	},
	"MD032": {},
	"MD033": {
		"allowed_elements":       {Type: "string[]", Default: []string{}},
		"table_allowed_elements": {Type: "string[]", Default: []string{}},
	},
	"MD034": {},
	"MD035": {
		"style": {Type: "string", Default: "consistent"},
	},
	"MD036": {
		"punctuation": {Type: "string", Default: ".,;:!?\u3002\uff0c\uff1b\uff1a\uff01\uff1f"},
	},
	"MD037": {},
	"MD038": {},
	"MD039": {},
	"MD040": {
		"allowed_languages": {Type: "string[]", Default: []string{}},
		"language_only":     {Type: "boolean", Default: false},
	},
	"MD041": {
		"allow_preamble":     {Type: "boolean", Default: false},
		"front_matter_title": {Type: "string", Default: `^\s*title\s*[:=]`},
		"level":              {Type: "integer", Default: 1},
	},
	"MD042": {},
	"MD043": {
		"headings":   {Type: "string[]", Default: []string{}},
		"match_case": {Type: "boolean", Default: false},
	},
	"MD044": {
		"names":         {Type: "string[]", Default: []string{}},
		"code_blocks":   {Type: "boolean", Default: true},
		"html_elements": {Type: "boolean", Default: true},
	},
	"MD045": {},
	"MD046": {
		"style": {Type: "string", Default: "consistent", Values: []string{"consistent", "fenced", "indented"}},
	},
	"MD047": {},
	"MD048": {
		"style": {Type: "string", Default: "consistent", Values: []string{"consistent", "backtick", "tilde"}},
	},
	"MD049": {
		"style": {Type: "string", Default: "consistent", Values: []string{"consistent", "asterisk", "underscore"}},
	},
	"MD050": {
		"style": {Type: "string", Default: "consistent", Values: []string{"consistent", "asterisk", "underscore"}},
	},
	"MD051": {
		"ignore_case":     {Type: "boolean", Default: false},
		"ignored_pattern": {Type: "string", Default: ""},
	},
	"MD052": {
		"ignored_labels":  {Type: "string[]", Default: []string{"x"}},
		"shortcut_syntax": {Type: "boolean", Default: false},
	},
	"MD053": {
		"ignored_definitions": {Type: "string[]", Default: []string{"//"}},
	},
	"MD054": {
		"autolink":   {Type: "boolean", Default: true},
		"inline":     {Type: "boolean", Default: true},
		"full":       {Type: "boolean", Default: true},
		"collapsed":  {Type: "boolean", Default: true},
		"shortcut":   {Type: "boolean", Default: true},
		"url_inline": {Type: "boolean", Default: true},
	},
	"MD055": {
		"style": {Type: "string", Default: "consistent", Values: []string{"consistent", "leading_only", "trailing_only", "leading_and_trailing", "no_leading_or_trailing"}},
	},
	"MD056": {},
	"MD058": {},
	"MD059": {
		"prohibited_texts": {Type: "string[]", Default: []string{"click here", "here", "link", "more"}},
	},
	"MD060": {
		"style":             {Type: "string", Default: "any", Values: []string{"any", "aligned", "compact", "tight"}},
		"aligned_delimiter": {Type: "boolean", Default: false},
	},
}
