//go:build windows && amd64
// +build windows,amd64

package sshprox

import "embed"

/*
	Binary embedding for Windows AMD64 builds

	Windows uses wintty (pure Go SSH terminal) instead of gotty binary.
	This embed only includes the license file for compatibility.
*/
var (
	//go:embed gotty/LICENSE
	gotty embed.FS
)

// UseWinTTY indicates whether this platform should use wintty
// instead of the gotty binary
const UseWinTTY = true
