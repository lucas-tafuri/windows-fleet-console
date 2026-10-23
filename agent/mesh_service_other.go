//go:build !windows

package main

import (
	"context"
	"errors"
)

func runMeshService(run func(context.Context) error) error {
	return errors.New("Windows service mode is unavailable")
}
func runMeshTray(url string) error {
	return errors.New("Windows tray mode is unavailable; open " + url)
}
