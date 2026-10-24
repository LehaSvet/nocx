package main

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/shady2k/nocx/internal/desktopidentity"
)

// desktopIdentityService runs first, before another service can register the
// GtkApplication or create a window. It deliberately fails startup on a Wails
// lifecycle change rather than leaving an unmatched launcher and Wayland ID.
type desktopIdentityService struct{}

func (*desktopIdentityService) ServiceStartup(context.Context, application.ServiceOptions) error {
	return desktopidentity.SetDefault()
}
