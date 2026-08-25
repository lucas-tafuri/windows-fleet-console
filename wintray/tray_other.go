//go:build !windows

package wintray

import "fmt"

func run(Config) error {
	return fmt.Errorf("tray is only available on Windows")
}

func quit() {}
