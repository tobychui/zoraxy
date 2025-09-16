//go:build darwin && amd64
// +build darwin,amd64

package sshprox

import "embed"

/*
Binary embedding for AMD64 builds

Make sure when compile, gotty binary exists in static.gotty
*/
var (
	//go:embed gotty/gotty_darwin_amd64
	//go:embed gotty/.gotty
	//go:embed gotty/LICENSE
	gotty embed.FS
)
