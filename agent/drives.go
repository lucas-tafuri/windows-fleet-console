package main

import "time"

// Independent of the console connection: a LAN share can recover while the
// console is offline. Session workers scope saved mappings to the signed-in user.
func monitorDrives() {
	for {
		result := runJob(AssignedJob{ID: "drive-maintenance", Kind: "maintain_drives"})
		if result.Status == "error" {
			logf("drive maintenance: %s", result.Message)
		}
		time.Sleep(time.Minute)
	}
}
