//go:build tools

// Package tools records the tools of the build in go.mod: the generator of the documentation.
package tools

import (
	// Documentation generation
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)
