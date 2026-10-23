//go:build !windows

package main

import (
	"context"
	"errors"
	"github.com/cursor/fleet-agent/mesh"
)

func configureMeshUpdates(n *mesh.Node, dir, exe string) {}
func stageMeshUpdate(ctx context.Context, n *mesh.Node, c mesh.Command, dir, exe string) mesh.Result {
	return mesh.Result{Status: "error", Message: "Windows updater unavailable"}
}
func applyMeshUpdate(path string) error { return errors.New("Windows updater unavailable") }
