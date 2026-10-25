package lint

import "github.com/yaklabco/gomdlint/pkg/mdast"

// sharedValues holds the values that the rules of one lint of one file share
// (see SharedValue). The engine makes one for each lint of a file, so each
// pass of --fix, which lints the content it fixed, starts with none. Rules
// run one after another within a lint, so it needs no lock.
type sharedValues struct {
	// file is the snapshot the values were built from.
	file   *mdast.FileSnapshot
	values map[any]any
}

// newSharedValues returns an empty set of values shared by the rules of one
// lint of file.
func newSharedValues(file *mdast.FileSnapshot) *sharedValues {
	return &sharedValues{file: file, values: make(map[any]any)}
}

// SharedValue returns the value of key for the file that rc lints: the
// value that build returns for the first rule of a lint of the file to ask
// for it, kept and returned to every rule after it in that lint. It is for
// values that are a pure function of what key holds and the file's content,
// such as a parse of the document or what a check derives from one, which
// several rules would otherwise each build. A rule must not change a value
// that it gets, nor anything the value refers to, since the rules after it
// get the same value.
//
// key must be comparable, or SharedValue panics as a map does, and should be
// of a type of the caller's own, so that keys of different callers cannot
// collide. A build may ask for another key's value. A value is kept only when
// build returns normally, with the lint not cancelled: a build that panics
// keeps nothing, so no rule is given a value left half built. A key that
// holds a value of another type than T is built again and not kept. A
// context that the engine did not make, or whose File is not the file the
// engine parsed, shares nothing: build runs on every call.
//
//nolint:ireturn // T is the caller's own type, which need not be an interface.
func SharedValue[T any](rc *RuleContext, key any, build func() T) T {
	shared := rc.shared
	if shared == nil || rc.File != shared.file {
		return build()
	}
	if held, found := shared.values[key]; found {
		if value, ok := held.(T); ok {
			return value
		}
		return build()
	}
	value := build()
	if rc.Ctx != nil && rc.Ctx.Err() != nil {
		return value
	}
	shared.values[key] = value
	return value
}
