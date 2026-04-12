package lifecycle

import (
	"path/filepath"
	"strings"
)

// IsArgusCommand reports whether command is a direct Argus CLI invocation or the
// current managed wrapper that resolves the Argus binary before exec'ing it.
func IsArgusCommand(command string) bool {
	_, ok := parseArgusCommand(command)
	return ok
}

// IsArgusAgentCommand reports whether command invokes `argus <subcommand>` for
// the requested agent, either directly or via the current managed wrapper.
func IsArgusAgentCommand(command string, subcommand string, agent string) bool {
	parsed, ok := parseArgusCommand(command)
	if !ok {
		return false
	}
	if parsed.subcommand != subcommand {
		return false
	}

	for index := range len(parsed.args) {
		arg := parsed.args[index]
		if value, ok := strings.CutPrefix(arg, "--agent="); ok {
			return value == agent
		}
		if arg == "--agent" && index+1 < len(parsed.args) {
			return parsed.args[index+1] == agent
		}
	}

	return false
}

type parsedArgusCommand struct {
	subcommand string
	args       []string
}

func parseArgusCommand(command string) (parsedArgusCommand, bool) {
	if parsed, ok := parseDirectArgusCommand(command); ok {
		return parsed, true
	}

	return parseManagedWrapperArgusCommand(command)
}

func parseDirectArgusCommand(command string) (parsedArgusCommand, bool) {
	trimmed := strings.TrimSpace(command)
	for strings.HasPrefix(trimmed, "exec ") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "exec "))
	}

	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return parsedArgusCommand{}, false
	}

	binary := strings.Trim(fields[0], `"'`)
	if filepath.Base(binary) != "argus" {
		return parsedArgusCommand{}, false
	}

	return parsedArgusCommand{
		subcommand: fields[1],
		args:       fields[2:],
	}, true
}

func parseManagedWrapperArgusCommand(command string) (parsedArgusCommand, bool) {
	if !strings.Contains(command, "command -v argus") {
		return parsedArgusCommand{}, false
	}

	lastSeparator := strings.LastIndex(command, ";")
	if lastSeparator == -1 {
		return parsedArgusCommand{}, false
	}

	return parseDirectArgusCommand(command[lastSeparator+1:])
}
