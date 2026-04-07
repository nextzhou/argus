package toolbox

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/nextzhou/argus/internal/core"
)

// RunTouchTimestamp writes the current compact UTC timestamp to the file at args[0].
func RunTouchTimestamp(args []string, _, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, "usage: argus toolbox touch-timestamp <file>\n")
		return 1
	}

	ts := core.FormatTimestamp(time.Now())
	if err := os.WriteFile(args[0], []byte(ts), 0o644); err != nil {
		_, _ = fmt.Fprintf(stderr, "writing timestamp: %s\n", err)
		return 1
	}

	return 0
}
