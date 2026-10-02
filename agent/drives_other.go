//go:build !windows

package main

func maintainDrives(id string) JobResult { return JobResult{JobID: id, Status: "ok"} }
