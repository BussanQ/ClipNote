//go:build !darwin && !windows

package main

import "errors"

func newDesktop() (desktop, error) { return nil, errors.New("ClipNote 需要 macOS 或 Windows") }
