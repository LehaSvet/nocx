package session

// ExplicitShellEnv is a daemon-environment-only override for test harnesses
// that must measure a specific local shell. It is not carried over the wire.
const ExplicitShellEnv = "NOCX_HELPER_SHELL"

// ShellFromEnv answers the Shell an explicit declaration names, or the zero
// Shell when none does. Production's unset value keeps normal login-shell
// resolution in internal/loginshell.
func ShellFromEnv(getenv func(string) string) Shell {
	if path := getenv(ExplicitShellEnv); path != "" {
		return Shell{Path: path}
	}
	return Shell{}
}
