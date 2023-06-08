//go:build (windows && amd64) || (linux && mipsle) || (linux && riscv64)
// +build windows,amd64 linux,mipsle linux,riscv64

package sshprox

import "embed"

/*
	Bianry embedding

	Make sure when compile, gotty binary exists in static.gotty
*/
var (
	//go:embed gotty/LICENSE
	gotty embed.FS
)
