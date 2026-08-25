//go:build !windows

package main

func ensureInstalled() (bool, error) { return false, nil }

func selfUpdate(jobID, repo, branch string) JobResult {
	return unsupported(jobID, "self-update")
}

func spawnRestart(_ *Client) {}
