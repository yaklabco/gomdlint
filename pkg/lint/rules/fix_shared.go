package rules

import (
	"context"

	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	gmparser "github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// Values the fix checks share.
//
// The fix checks of every rule compare candidate fixes with the original
// document, which is the same for all the rules of one lint of a file. What
// they derive from the original alone is built once per lint, by the first
// rule that needs it, and shared with the rules after it (see
// lint.SharedValue): the parse of the original with a check's parser, the
// outline that fixChecker compares (docOutline), the outlines that
// fixVerifier compares, one for each allowance, and the region layout of
// the original. Each is keyed by the flavor of the parser it comes from,
// gfm or commonmark as gmparser.New reads the configured flavor, and the
// outlines of fixVerifier also by their allowance; the lint fixes the
// file's path and content. No shared value is changed once built: the
// checks read the parse and the outlines, and each check takes a view of
// the layout (see docLayout.view) for what it builds and counts as it goes.

// originalKey is the key of the original document parsed with a check's
// parser of flavor.
type originalKey struct{ flavor string }

// checkerOutlineKey is the key of the outline that fixChecker compares, of
// the original parsed with a check's parser of flavor.
type checkerOutlineKey struct{ flavor string }

// verifierOutlineKey is the key of the outline that fixVerifier compares
// with allowance allow, of the original parsed with a check's parser of
// flavor.
type verifierOutlineKey struct {
	flavor string
	allow  fixAllowance
}

// layoutKey is the key of the region layout of the original, read as a
// parser of flavor reads it.
type layoutKey struct{ flavor string }

// sharedParse is the shared parse of the original, or nil when the parse
// failed.
type sharedParse struct {
	file *mdast.FileSnapshot
}

// sharedOutline is the shared outline that fixChecker compares of the
// original; ok is false when the original did not parse.
type sharedOutline struct {
	outline docOutline
	ok      bool
}

// checkFlavor returns the flavor of the parser the fix checks use for the
// file of ctx: gfm, or commonmark for any other configured flavor, as
// gmparser.New reads it.
func checkFlavor(ctx *lint.RuleContext) string {
	if parseFlavor(ctx) == gmparser.FlavorGFM {
		return gmparser.FlavorGFM
	}
	return gmparser.FlavorCommonMark
}

// originalDocument returns the original document of the file of ctx, as
// the checks read it (see checkedContent), parsed with a check's parser
// with the file's path. ok is false when the parse fails.
func originalDocument(ctx *lint.RuleContext) (*mdast.FileSnapshot, bool) {
	flavor := checkFlavor(ctx)
	parsed := lint.SharedValue(ctx, originalKey{flavor}, func() sharedParse {
		parseCtx := ctx.Ctx
		if parseCtx == nil {
			parseCtx = context.Background()
		}
		var path string
		var content []byte
		if ctx.File != nil {
			path, content = ctx.File.Path, checkedContent(ctx.File.Content)
		}
		countOriginalParse(parseCtx, false)
		file, err := gmparser.New(flavor).Parse(parseCtx, path, content)
		if err != nil || file == nil || file.Root == nil {
			return sharedParse{}
		}
		return sharedParse{file: file}
	})
	return parsed.file, parsed.file != nil
}

// originalOutline returns the outline that fixChecker compares of the
// original document of the file of ctx. ok is false when the original does
// not parse.
func originalOutline(ctx *lint.RuleContext) (docOutline, bool) {
	shared := lint.SharedValue(ctx, checkerOutlineKey{checkFlavor(ctx)}, func() sharedOutline {
		file, ok := originalDocument(ctx)
		if !ok {
			return sharedOutline{}
		}
		return sharedOutline{outline: outlineOf(file), ok: true}
	})
	return shared.outline, shared.ok
}

// verifierOutline returns the outline that fixVerifier compares with allow
// of the original document of the file of ctx, which must parse (see
// originalDocument).
func verifierOutline(ctx *lint.RuleContext, allow fixAllowance) string {
	return lint.SharedValue(ctx, verifierOutlineKey{flavor: checkFlavor(ctx), allow: allow}, func() string {
		original, ok := originalDocument(ctx)
		if !ok {
			return ""
		}
		return documentOutline(original, allow)
	})
}

// originalLayout returns a view of the region layout of the original
// document of the file of ctx, as the checks read it, or nil when region
// checks cannot be used for it.
func originalLayout(ctx *lint.RuleContext) *docLayout {
	flavor := checkFlavor(ctx)
	base := lint.SharedValue(ctx, layoutKey{flavor}, func() *docLayout {
		var content []byte
		if ctx.File != nil {
			content = checkedContent(ctx.File.Content)
		}
		countOriginalParse(ctx.Ctx, true)
		return newDocLayoutFlags(recordingMarkdown(flavor), content, true)
	})
	return base.view()
}

// originalParsesKey is the context key under which a test counts the
// parses of the whole original document that the fix checks make (see
// countOriginalParse). The builds above are the only places where the
// checks of fixChecker and fixVerifier parse the whole original, so the
// counts bound how often they do.
type originalParsesKey struct{}

// originalParses counts the parses of the whole original document that the
// fix checks of a lint make: checks counts the parses with a check's parser,
// and layouts the recording parses of region layouts.
type originalParses struct {
	checks, layouts int
}

// countOriginalParse counts a parse of the whole original document in the
// counts that ctx carries, if it carries any; layout tells a recording
// parse for a region layout from a parse with a check's parser. Only tests
// set the counts.
func countOriginalParse(ctx context.Context, layout bool) {
	if ctx == nil {
		return
	}
	counts, ok := ctx.Value(originalParsesKey{}).(*originalParses)
	if !ok {
		return
	}
	if layout {
		counts.layouts++
	} else {
		counts.checks++
	}
}
