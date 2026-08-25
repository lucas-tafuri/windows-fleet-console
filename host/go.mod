module github.com/cursor/windows-fleet-console/host

go 1.22

require (
	github.com/cursor/windows-fleet-console/wintray v0.0.0
	golang.org/x/sys v0.25.0
)

replace github.com/cursor/windows-fleet-console/wintray => ../wintray
