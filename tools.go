//go:build tools

// Package tools pins module dependencies that are not yet imported by
// application code. This file is excluded from normal builds by the
// "tools" build constraint and exists solely to prevent `go mod tidy`
// from removing dependencies declared for future use.
package tools

import (
	_ "github.com/spf13/cobra"
	_ "github.com/spf13/viper"
	_ "github.com/stretchr/testify"
	_ "gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)
