//go:build windows

package main

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

func acquireInstance(wait bool) (func(), error) {
	return acquireNamedInstance(`Global\PrettyDamnFleetAgent`, wait)
}

func acquireNamedInstance(key string, wait bool) (func(), error) {
	name, err := windows.UTF16PtrFromString(key)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		handle, err := windows.CreateMutex(nil, false, name)
		if err == nil {
			return func() { windows.CloseHandle(handle) }, nil
		}
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		// A mutex owned by SYSTEM can deny access to an interactive launch.
		// Both cases must reject the second agent, never fail open.
		if !errors.Is(err, windows.ERROR_ALREADY_EXISTS) && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return nil, fmt.Errorf("check running agent: %w", err)
		}
		if !wait || time.Now().After(deadline) {
			return nil, fmt.Errorf("another Fleet agent is already running: %w", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
