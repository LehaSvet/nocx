//go:build !linux

package desktopidentity

// SetDefault does nothing on hosts without a GtkApplication.
func SetDefault() error { return nil }
