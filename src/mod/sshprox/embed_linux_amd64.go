//go:build linux && amd64
// +build linux,amd64

package sshprox

import "embed"

/*
	Bianry embedding for AMD64 builds

	Make sure when compile, gotty binary exists in static.gotty
*/
var (
	//go:embed gotty/gotty_linux_amd64
	//go:embed gotty/.gotty
	//go:embed gotty/LICENSE
	gotty embed.FS
)
