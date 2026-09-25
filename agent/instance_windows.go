//go:build windows

package main

import "golang.org/x/sys/windows"

func acquireInstance() bool {
	name, _ := windows.UTF16PtrFromString(`Global\PrettyDamnFleetAgent`)
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		return false
	}
	// Keep the handle alive until process exit.
	return true
}
