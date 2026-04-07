package toolbox

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// RunSHA256Sum computes SHA256 and outputs in coreutils format.
func RunSHA256Sum(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	stdinMode := len(args) == 0 || args[0] == "-"

	var reader io.Reader
	var name string

	if stdinMode {
		reader = stdin
		name = "-"
	} else {
		f, err := os.Open(args[0])
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "opening file: %s\n", err)
			return 1
		}
		defer f.Close() //nolint:errcheck // read-only file, close error is not actionable
		reader = f
		name = args[0]
	}

	h := sha256.New()
	if _, err := io.Copy(h, reader); err != nil {
		_, _ = fmt.Fprintf(stderr, "reading input: %s\n", err)
		return 1
	}

	_, _ = fmt.Fprintf(stdout, "%s  %s\n", hex.EncodeToString(h.Sum(nil)), name)
	return 0
}
