package session

import "testing"

func TestShellFromEnv(t *testing.T) {
	t.Run("unset preserves login-shell resolution", func(t *testing.T) {
		got := ShellFromEnv(func(string) string { return "" })
		if got.Path != "" || got.Args != nil {
			t.Errorf("ShellFromEnv with %s unset = %+v, want the zero Shell", ExplicitShellEnv, got)
		}
	})
	t.Run("set selects the named test shell", func(t *testing.T) {
		got := ShellFromEnv(func(name string) string {
			if name == ExplicitShellEnv {
				return "/bin/zsh"
			}
			return ""
		})
		if got.Path != "/bin/zsh" || got.Args != nil {
			t.Errorf("ShellFromEnv with %s=/bin/zsh = %+v, want Shell{Path: /bin/zsh}", ExplicitShellEnv, got)
		}
	})
}
