package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// The worker has the user's token, never the background agent's SYSTEM token.
func runSessionJobFile(file string) {
	raw, err := os.ReadFile(file)
	if err != nil {
		os.Exit(1)
	}
	var job AssignedJob
	if json.Unmarshal(raw, &job) != nil {
		os.Exit(1)
	}
	var result JobResult
	switch job.Kind {
	case "check", "install", "uninstall", "map_drive", "unmap_drive", "clean_downloads", "empty_recycle", "launch":
		background = false
		result = runJob(job)
	default:
		result = JobResult{JobID: job.ID, Status: "error", Message: "Unsupported session job"}
	}
	output, err := json.Marshal(result)
	if err != nil {
		os.Exit(1)
	}
	if os.WriteFile(filepath.Join(filepath.Dir(file), "result.json"), output, 0600) != nil {
		os.Exit(1)
	}
}
