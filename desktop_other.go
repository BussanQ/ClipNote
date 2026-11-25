//go:build !darwin

package main

import "errors"

func newDesktop() (desktop, error) { return nil, errors.New("ClipNote 需要 macOS") }
