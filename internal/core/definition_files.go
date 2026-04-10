package core

import "fmt"

// ExpectedYAMLFileName returns the canonical YAML file name for an Argus definition ID.
func ExpectedYAMLFileName(id string) string {
	return id + ".yaml"
}

// DefinitionFileNameMatchesID reports whether fileName matches the canonical <id>.yaml contract.
func DefinitionFileNameMatchesID(fileName, id string) bool {
	return fileName == ExpectedYAMLFileName(id)
}

// DefinitionFileNameMismatchMessage formats a stable validation error for a mismatched definition file name.
func DefinitionFileNameMismatchMessage(kind, fileName, id string) string {
	return fmt.Sprintf(
		"%s file name %q must match %s ID %q (expected %q)",
		kind,
		fileName,
		kind,
		id,
		ExpectedYAMLFileName(id),
	)
}
