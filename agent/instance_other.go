//go:build !windows

package main

func acquireInstance(_ bool) (func(), error) { return func() {}, nil }
