// Package platform provides platform detection and skill installation for
// agent platforms such as Claude Code.
package platform

// Platform represents a recognized agent platform.
type Platform string

// PlatformClaudeCode identifies the Claude Code agent platform.
const PlatformClaudeCode Platform = "claude-code"
