//go:build linux && 386
// +build linux,386

package sshprox

import "embed"

/*
	Bianry embedding for i386 builds

	Make sure when compile, gotty binary exists in static.gotty
*/
var (
	//go:embed gotty/gotty_linux_386
	//go:embed gotty/.gotty
	//go:embed gotty/LICENSE
	gotty embed.FS
)
