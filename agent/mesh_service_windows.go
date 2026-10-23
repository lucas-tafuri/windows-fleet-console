//go:build windows

package main

import (
	"context"
	"github.com/cursor/windows-fleet-console/wintray"
	"golang.org/x/sys/windows/svc"
	"time"
)

const meshServiceName = "PrettyDamnFleetMesh"

type meshService struct{ run func(context.Context) error }

func (h *meshService) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.run(ctx) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-done:
			if err != nil {
				logf("mesh service: %v", err)
				return false, 1
			}
			return false, 0
		case change := <-requests:
			switch change.Cmd {
			case svc.Interrogate:
				status <- change.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending, WaitHint: 25000}
				cancel()
				select {
				case <-done:
				case <-time.After(25 * time.Second):
				}
				return false, 0
			}
		}
	}
}
func runMeshService(run func(context.Context) error) error {
	return svc.Run(meshServiceName, &meshService{run: run})
}
func runMeshTray(url string) error {
	release, err := acquireNamedInstance(`Local\PrettyDamnFleetMeshTray`, false)
	if err != nil {
		return err
	}
	defer release()
	return wintray.Run(wintray.Config{Tooltip: "PrettyDamnFleet Mesh", Items: []wintray.Item{{ID: 1, Title: "Open manager"}, {ID: 2, Title: "Hide tray icon"}}, OnCommand: func(id uint32) {
		if id == 1 {
			_ = shellExec(url, "")
		} else if id == 2 {
			wintray.Quit()
		}
	}})
}
