package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// The worker uses the user's token (elevated only for package changes), never SYSTEM.
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
	case "check", "install", "uninstall", "map_drive", "unmap_drive", "maintain_drives", "clean_downloads", "empty_recycle", "launch":
		background = false
		result = runJob(job)
	default:
		result = JobResult{JobID: job.ID, Status: "error", Message: "Unsupported session job"}
	}
	output, err := json.Marshal(result)
	if err != nil {
		os.Exit(1)
	}
	if os.WriteFile(filepath.Join(filepath.Dir(file), "output", "result.json"), output, 0600) != nil {
		os.Exit(1)
	}
}
