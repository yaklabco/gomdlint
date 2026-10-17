package goldmark

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// TestConcurrentParsersAndParses creates parsers and parses documents from
// many goroutines at once, as the runner and parallel tests do. Run it with
// -race: goldmark's context keys are package-global counters that must not
// be allocated while other goroutines create parse contexts.
func TestConcurrentParsersAndParses(t *testing.T) {
	const workers = 16
	input := []byte("# Title\n\n- [link](https://x.y)\n- `code`\n\n> quote\n")

	shared := New(FlavorGFM)
	var wg sync.WaitGroup
	errs := make(chan error, 2*workers)
	for worker := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			flavor := FlavorCommonMark
			if worker%2 == 0 {
				flavor = FlavorGFM
			}
			for _, parser := range []*Parser{New(flavor), shared} {
				file, err := parser.Parse(context.Background(), "concurrent.md", input)
				if err != nil {
					errs <- err
					continue
				}
				if len(mdast.FindByKind(file.Root, mdast.NodeHeading)) != 1 {
					errs <- errHeadingMissing
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

// errHeadingMissing reports a parse that lost the document's heading.
var errHeadingMissing = &parseCheckError{"heading missing from concurrent parse"}

type parseCheckError struct{ msg string }

func (e *parseCheckError) Error() string { return e.msg }
