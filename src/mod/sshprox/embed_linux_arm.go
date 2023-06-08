//go:build linux && arm
// +build linux,arm

package sshprox

import "embed"

/*
	Bianry embedding for ARM(v6/7) builds

	Make sure when compile, gotty binary exists in static.gotty
*/
var (
	//go:embed gotty/gotty_linux_arm
	//go:embed gotty/.gotty
	//go:embed gotty/LICENSE
	gotty embed.FS
)
