//go:build !nocx_e2e_shell_override

package session

// ShellFromEnv ignores shell overrides in default and shipped helper builds.
// Only the dedicated e2e helper build enables an explicit shell declaration.
func ShellFromEnv(_ func(string) string) Shell { return Shell{} }
