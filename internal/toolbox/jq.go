// Package toolbox provides built-in tool implementations for the argus toolbox command.
package toolbox

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/itchyny/gojq"
)

// RunJQ runs a jq-compatible query against JSON input from stdin.
func RunJQ(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, "usage: argus toolbox jq <expression>\n")
		return 1
	}

	query, err := gojq.Parse(args[0])
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "parsing query: %s\n", err)
		return 1
	}

	code, err := gojq.Compile(query)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "compiling query: %s\n", err)
		return 1
	}

	var v any
	if err := json.NewDecoder(stdin).Decode(&v); err != nil {
		_, _ = fmt.Fprintf(stderr, "decoding JSON input: %s\n", err)
		return 1
	}

	iter := code.Run(v)
	for {
		val, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := val.(error); isErr {
			_, _ = fmt.Fprintf(stderr, "evaluating query: %s\n", err)
			return 1
		}

		out, err := gojq.Marshal(val)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "marshaling result: %s\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "%s\n", out)
	}

	return 0
}
