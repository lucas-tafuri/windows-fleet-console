package main

import (
	"strings"
	"sync"
)

var driveJobs sync.Mutex

func runJob(job AssignedJob) JobResult {
	if job.Kind == "map_drive" || job.Kind == "unmap_drive" || job.Kind == "maintain_drives" {
		driveJobs.Lock()
		defer driveJobs.Unlock()
	}
	if background {
		switch job.Kind {
		case "check", "install", "uninstall", "map_drive", "unmap_drive", "maintain_drives", "clean_downloads", "empty_recycle", "launch":
			return runInteractiveJob(job)
		}
	}
	pkg := payloadString(job.Payload, "package")
	letter := strings.ToUpper(payloadString(job.Payload, "letter"))
	unc := payloadString(job.Payload, "unc")
	user := payloadString(job.Payload, "username")
	pass := payloadString(job.Payload, "password")
	target := payloadString(job.Payload, "target")
	args := payloadString(job.Payload, "args")

	switch job.Kind {
	case "maintain_drives":
		return maintainDrives(job.ID)
	case "check":
		return checkPackage(job.ID, pkg, payloadString(job.Payload, "match"))
	case "install":
		return installPackage(job.ID, pkg)
	case "uninstall":
		return uninstallPackage(job.ID, pkg)
	case "map_drive":
		return mapDrive(job.ID, letter, unc, user, pass)
	case "unmap_drive":
		return unmapDrive(job.ID, letter)
	case "clean_downloads":
		return cleanDownloads(job.ID)
	case "empty_recycle":
		return emptyRecycle(job.ID)
	case "launch":
		return launchProgram(job.ID, target, args)
	case "self_update":
		return selfUpdate(job.ID, payloadString(job.Payload, "repo"), payloadString(job.Payload, "branch"))
	default:
		return JobResult{JobID: job.ID, Status: "error", Message: "unknown job kind " + job.Kind}
	}
}
