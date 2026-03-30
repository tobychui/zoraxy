//go:build (linux && mipsle) || (linux && riscv64) || (freebsd && amd64) || (darwin && arm64)
// +build linux,mipsle linux,riscv64 freebsd,amd64 darwin,arm64

package sshprox

import "embed"

/*
Binary embedding

Make sure when compile, gotty binary exists in static.gotty
*/
var (
	//go:embed gotty/LICENSE
	gotty embed.FS
)

// UseWinTTY indicates whether this platform should use wintty
// instead of the gotty binary
const UseWinTTY = false
