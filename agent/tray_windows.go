//go:build windows

package main

import (
	"os"

	"github.com/cursor/windows-fleet-console/wintray"
)

const (
	trayUpdateID = 1
	trayCloseID  = 2
)

func serveTray(c *Client) {
	err := wintray.Run(wintray.Config{
		Tooltip: "Fleet Agent",
		Items: []wintray.Item{
			{ID: trayUpdateID, Title: "Update"},
			{ID: trayCloseID, Title: "Close"},
		},
		OnCommand: func(id uint32) {
			switch id {
			case trayUpdateID:
				go trayUpdate(c)
			case trayCloseID:
				wintray.Quit()
			}
		},
	})
	if err != nil {
		logf("tray: %v", err)
		waitSignal()
		return
	}
	os.Exit(0)
}

func trayUpdate(c *Client) {
	res := selfUpdate("tray", "", "")
	logf("tray update: %s — %s", res.Status, res.Message)
	if res.Status != "ok" {
		return
	}
	restartRequested = false
	spawnRestart(c)
	os.Exit(0)
}
