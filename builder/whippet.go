package builder

import (
	"path/filepath"

	"github.com/tinygo-org/tinygo/goenv"
)

// WhippetGC is the serial, non-moving MMC collector from Whippet.
var WhippetGC = Library{
	name: "whippet",
	cflags: func(target, headerPath string) []string {
		root := goenv.Get("TINYGOROOT")
		return []string{
			"-DGC_GENERATIONAL=0",
			"-DGC_PARALLEL=0",
			"-DGC_CONSERVATIVE_ROOTS=1",
			"-DGC_CONSERVATIVE_TRACE=1",
			"-DGC_HAS_IMMEDIATES=0",
			"-DNDEBUG",
			`-DGC_ATTRS="mmc-attrs.h"`,
			`-DGC_EMBEDDER="tinygo-whippet-embedder.h"`,
			"-I" + filepath.Join(root, "src/runtime/whippet"),
			"-I" + filepath.Join(root, "lib/whippet/api"),
			"-I" + filepath.Join(root, "lib/whippet/src"),
		}
	},
	needsLibc: true,
	sourceDir: func() string {
		return goenv.Get("TINYGOROOT")
	},
	librarySources: func(target string, libcNeedsMalloc bool) ([]string, error) {
		return []string{
			"lib/whippet/src/mmc.c",
			"lib/whippet/src/gc-ephemeron.c",
			"lib/whippet/src/gc-finalizer.c",
			"lib/whippet/src/gc-options.c",
			"lib/whippet/src/gc-tracepoint.c",
			"src/runtime/whippet/gc-platform-tinygo.c",
			"src/runtime/whippet/gc-stack-tinygo.c",
		}, nil
	},
}
