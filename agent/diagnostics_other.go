//go:build !windows

package main

func diagnosticReport(jobID, profile string) JobResult {
	return JobResult{JobID: jobID, Status: "error", Message: "Diagnostics require Windows"}
}
