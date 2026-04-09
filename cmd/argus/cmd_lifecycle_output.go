package main

import (
	"fmt"
	"io"

	"github.com/nextzhou/argus/internal/install"
)

type lifecycleOutput struct {
	Message string `json:"message"`
	Root    string `json:"root,omitempty"`
	Path    string `json:"path,omitempty"`
	install.Report
}

func renderLifecycleText(w io.Writer, out lifecycleOutput, nextSteps []string) {
	_, _ = fmt.Fprintf(w, "Argus: %s\n", out.Message)
	if out.Root != "" {
		_, _ = fmt.Fprintf(w, "Project root: %s\n", out.Root)
	}
	if out.Path != "" {
		_, _ = fmt.Fprintf(w, "Workspace path: %s\n", out.Path)
	}

	_, _ = fmt.Fprintln(w)
	if hasLifecycleChanges(out.Changes) {
		renderLifecycleChangeSection(w, "Created", out.Changes.Created)
		renderLifecycleChangeSection(w, "Updated", out.Changes.Updated)
		renderLifecycleChangeSection(w, "Removed", out.Changes.Removed)
	} else {
		_, _ = fmt.Fprintln(w, "No filesystem changes.")
		_, _ = fmt.Fprintln(w)
	}

	_, _ = fmt.Fprintln(w, "Affected paths:")
	for _, path := range out.AffectedPaths {
		_, _ = fmt.Fprintf(w, "- %s\n", path)
	}

	if len(nextSteps) == 0 {
		return
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Next steps:")
	for _, step := range nextSteps {
		_, _ = fmt.Fprintf(w, "- %s\n", step)
	}
}

func renderLifecycleChangeSection(w io.Writer, title string, paths []string) {
	if len(paths) == 0 {
		return
	}

	_, _ = fmt.Fprintf(w, "%s:\n", title)
	for _, path := range paths {
		_, _ = fmt.Fprintf(w, "- %s\n", path)
	}
	_, _ = fmt.Fprintln(w)
}

func hasLifecycleChanges(changes install.ChangeSet) bool {
	return len(changes.Created) > 0 || len(changes.Updated) > 0 || len(changes.Removed) > 0
}
