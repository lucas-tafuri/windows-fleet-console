package main

import (
	"regexp"
	"sort"
	"strings"
)

func softwareNameMatches(name, match string) bool {
	match = strings.ToLower(strings.TrimSpace(match))
	return match != "" && strings.Contains(strings.ToLower(name), match)
}

func uniqueInstallations(entries []SoftwareInstallation) []SoftwareInstallation {
	result := make([]SoftwareInstallation, 0, len(entries))
	seen := make(map[string]bool)
	for _, entry := range entries {
		entry.Name = strings.TrimSpace(entry.Name)
		entry.Version = strings.TrimSpace(entry.Version)
		key := strings.ToLower(entry.Name) + "\x00" + entry.Version
		if entry.Name != "" && !seen[key] {
			seen[key] = true
			result = append(result, entry)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].Version < result[j].Version
	})
	return result
}

func softwareCheckResult(jobID, pkg, via string, entries []SoftwareInstallation) JobResult {
	entries = uniqueInstallations(entries)
	installed := len(entries) > 0
	message := pkg + " is not installed"
	var details []string
	if installed {
		message = pkg + " is installed"
	}
	for _, entry := range entries {
		version := entry.Version
		if version == "" {
			version = "Version unavailable"
		}
		details = append(details, entry.Name+": "+version)
	}
	return JobResult{JobID: jobID, Status: "ok", Via: via, Message: message,
		Installed: &installed, Installations: entries, Output: strings.Join(details, "\n")}
}

var wingetColumnGap = regexp.MustCompile(` {2,}`)

// Use column positions, not whitespace in data rows: names can contain spaces,
// and the Available column contains the upgrade version, not the installed one.
// Header labels are localized; their positions have the same meaning.
func parseWingetInstallations(output, query string, exactID bool) []SoftwareInstallation {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	var columns []int
	var entries []SoftwareInstallation
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) >= 3 && strings.Trim(trimmed, "-") == "" && i > 0 {
			header := lines[i-1]
			columns = []int{0}
			for _, gap := range wingetColumnGap.FindAllStringIndex(header, -1) {
				if gap[1] < len(header) {
					columns = append(columns, len([]rune(header[:gap[1]])))
				}
			}
			continue
		}
		if len(columns) < 3 {
			continue
		}
		runes := []rune(line)
		if len(runes) <= columns[2] {
			continue
		}
		name := strings.TrimSpace(string(runes[:columns[1]]))
		id := strings.TrimSpace(string(runes[columns[1]:columns[2]]))
		end := len(runes)
		if len(columns) > 3 && columns[3] < end {
			end = columns[3]
		}
		version := strings.TrimSpace(string(runes[columns[2]:end]))
		if (exactID && strings.EqualFold(id, query)) || (!exactID && softwareNameMatches(name, query)) {
			entries = append(entries, SoftwareInstallation{Name: name, Version: version})
		}
	}
	return uniqueInstallations(entries)
}
