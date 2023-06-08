//go:build linux && arm64
// +build linux,arm64

package sshprox

import "embed"

/*
	Bianry embedding for ARM64 builds

	Make sure when compile, gotty binary exists in static.gotty
*/
var (
	//go:embed gotty/gotty_linux_arm64
	//go:embed gotty/.gotty
	//go:embed gotty/LICENSE
	gotty embed.FS
)
