//go:build nocx_e2e_shell_override

package session

// ExplicitShellEnv is available only in the dedicated e2e helper build.
const ExplicitShellEnv = "NOCX_HELPER_SHELL"

// ShellFromEnv answers the shell explicitly named by the e2e harness, or the
// zero Shell when none is set so normal login-shell resolution remains intact.
func ShellFromEnv(getenv func(string) string) Shell {
	if path := getenv(ExplicitShellEnv); path != "" {
		return Shell{Path: path}
	}
	return Shell{}
}
