//go:build !windows

package main

func runInteractiveJob(job AssignedJob) JobResult {
	return JobResult{JobID: job.ID, Status: "error", Message: "User sessions require Windows"}
}
