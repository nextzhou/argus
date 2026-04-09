package main

import "github.com/nextzhou/argus/internal/install"

type lifecycleOutput struct {
	Message string `json:"message"`
	Root    string `json:"root,omitempty"`
	Path    string `json:"path,omitempty"`
	install.Report
}
