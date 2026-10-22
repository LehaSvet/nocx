//go:build !nocx_e2e_shell_override

package session

import "testing"

func TestShellFromEnv_DefaultBuildIgnoresTestOverride(t *testing.T) {
	got := ShellFromEnv(func(string) string { return "/bin/zsh" })
	if got.Path != "" || got.Args != nil {
		t.Fatalf("default helper build honored test shell override: %+v", got)
	}
}
