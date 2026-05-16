//go:build openbsd

package sshprox

import "embed"

const UseWinTTY = false

//go:embed gotty/*
var gotty embed.FS
