//go:build !windows

package main

import (
	"os"
	"runtime"
)

type snapshot struct {
	Hostname       string
	User           string
	OS             string
	GPU            *float64
	CPU            *float64
	Memory         *float64
	Frozen         bool
	MetricsLimited bool
	LastInputAgeMs *int64
	MappedDrives   []MappedDrive
}

func collectSnapshot() snapshot {
	host, _ := os.Hostname()
	return snapshot{
		Hostname:       host,
		User:           os.Getenv("USER"),
		OS:             runtime.GOOS + " (agent stub)",
		MetricsLimited: true,
	}
}

func checkPackage(jobID, pkg string) JobResult {
	return unsupported(jobID, "check "+pkg)
}
func installPackage(jobID, pkg string) JobResult {
	return unsupported(jobID, "install "+pkg)
}
func uninstallPackage(jobID, pkg string) JobResult {
	return unsupported(jobID, "uninstall "+pkg)
}
func mapDrive(jobID, letter, unc, user, pass string) JobResult {
	return unsupported(jobID, "map "+letter)
}
func unmapDrive(jobID, letter string) JobResult {
	return unsupported(jobID, "unmap "+letter)
}
func cleanDownloads(jobID string) JobResult { return unsupported(jobID, "clean downloads") }
func emptyRecycle(jobID string) JobResult   { return unsupported(jobID, "empty recycle") }
func launchProgram(jobID, target, args string) JobResult {
	return unsupported(jobID, "launch "+target)
}

func unsupported(jobID, what string) JobResult {
	return JobResult{
		JobID:   jobID,
		Status:  "error",
		Via:     "stub",
		Message: "This agent build is not Windows; " + what + " is unavailable",
	}
}
