package toolbox

import (
	"fmt"
	"io"
	"strings"

	logging "gopkg.in/op/go-logging.v1"

	"github.com/mikefarah/yq/v4/pkg/yqlib"
)

// RunYQ runs a yq-compatible query against YAML input from stdin.
func RunYQ(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, "usage: argus toolbox yq <expression>\n")
		return 1
	}

	backend := logging.NewLogBackend(io.Discard, "", 0)
	yqlib.GetLogger().SetBackend(logging.AddModuleLevel(backend))

	input, err := io.ReadAll(stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "reading input: %s\n", err)
		return 1
	}

	encoder := yqlib.NewYamlEncoder(yqlib.ConfiguredYamlPreferences)
	decoder := yqlib.NewYamlDecoder(yqlib.ConfiguredYamlPreferences)
	evaluator := yqlib.NewStringEvaluator()

	result, err := evaluator.EvaluateAll(args[0], string(input), encoder, decoder)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "evaluating expression: %s\n", err)
		return 1
	}

	result = strings.TrimRight(result, "\n")
	_, _ = fmt.Fprintf(stdout, "%s\n", result)
	return 0
}
