package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSoftwareCheckIncludesEveryVersion(t *testing.T) {
	result := softwareCheckResult("job-1", "Adobe After Effects", "registry", []SoftwareInstallation{
		{Name: "Adobe After Effects 2025", Version: "25.6.1"},
		{Name: "Adobe After Effects 2026", Version: "26.0.2"},
		{Name: "adobe after effects 2025", Version: "25.6.1"},
		{Name: "Adobe After Effects 2026", Version: "26.1"},
	})
	if result.Installed == nil || !*result.Installed || len(result.Installations) != 3 {
		t.Fatalf("lost installations or duplicates remain: %+v", result)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded JobResult
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Installations) != 3 || decoded.Installations[0].Version != "25.6.1" {
		t.Fatalf("versions lost across worker/heartbeat JSON: %s", raw)
	}
}

func TestSoftwareCheckMissingAndUnknownVersion(t *testing.T) {
	missing := softwareCheckResult("job", "App", "registry", nil)
	raw, _ := json.Marshal(missing)
	if !strings.Contains(string(raw), `"installed":false`) {
		t.Fatalf("missing flag lost: %s", raw)
	}
	unknown := softwareCheckResult("job", "App", "registry", []SoftwareInstallation{{Name: "App 2026"}})
	if !*unknown.Installed || unknown.Installations[0].Version != "" {
		t.Fatalf("must not guess a version from display name: %+v", unknown)
	}
}

func TestWingetInstalledVersionAndLocalizedHeaders(t *testing.T) {
	for _, header := range [][5]string{{"Name", "Id", "Version", "Available", "Source"}, {"Nome", "ID", "Versão", "Disponível", "Origem"}} {
		row := func(fields [5]string) string {
			return fmt.Sprintf("%-30s %-25s %-12s %-12s %s\n", fields[0], fields[1], fields[2], fields[3], fields[4])
		}
		output := "progress\n" + row(header) + strings.Repeat("-", 90) + "\n" +
			row([5]string{"Adobe Photoshop 2026", "Adobe.Photoshop", "27.0.1", "27.2.0", "winget"}) +
			row([5]string{"Adobe Photoshop 2025", "Adobe.Photoshop.2025", "26.8.0", "", ""})
		found := parseWingetInstallations(output, "Adobe.Photoshop", true)
		if len(found) != 1 || found[0].Version != "27.0.1" {
			t.Fatalf("wrong installed version: %+v", found)
		}
		found = parseWingetInstallations(output, "Adobe Photoshop", false)
		if len(found) != 2 {
			t.Fatalf("lost side-by-side installation: %+v", found)
		}
		if len(parseWingetInstallations(output, "Missing.App", true)) != 0 {
			t.Fatal("matched unrelated package")
		}
	}
	if len(parseWingetInstallations("No installed package found: Adobe.Photoshop", "Adobe.Photoshop", true)) != 0 {
		t.Fatal("matched diagnostic text")
	}
}

func TestSoftwareNameMatching(t *testing.T) {
	if !softwareNameMatches("Adobe After Effects 2026", " adobe after effects ") {
		t.Fatal("display name fallback failed")
	}
	if softwareNameMatches("Unrelated App", "") {
		t.Fatal("empty match matched everything")
	}
}
