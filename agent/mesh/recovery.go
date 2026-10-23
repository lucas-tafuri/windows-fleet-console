package mesh

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type LegacyArchive struct {
	ID             string                    `json:"id"`
	Machines       []map[string]any          `json:"machines,omitempty"`
	Jobs           []JobView                 `json:"jobs,omitempty"`
	SoftwareStatus map[string]map[string]any `json:"softwareStatus,omitempty"`
}

func scrub(v any, secrets []string) any {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			key := strings.ToLower(k)
			if strings.Contains(key, "password") || strings.Contains(key, "token") {
				delete(x, k)
			} else {
				x[k] = scrub(value, secrets)
			}
		}
		return x
	case []any:
		for i, value := range x {
			x[i] = scrub(value, secrets)
		}
		return x
	case string:
		for _, secret := range secrets {
			if secret != "" {
				x = strings.ReplaceAll(x, secret, "[redacted]")
			}
		}
		return x
	default:
		return v
	}
}
func collectSecrets(v any, secrets *[]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			key := strings.ToLower(k)
			if strings.Contains(key, "password") || strings.Contains(key, "token") {
				if s, ok := value.(string); ok && s != "" {
					*secrets = append(*secrets, s)
				}
			}
			collectSecrets(value, secrets)
		}
	case []any:
		for _, value := range x {
			collectSecrets(value, secrets)
		}
	}
}

// ImportLegacy never produces executable commands or reuses legacy secrets.
// Its immutable archive records remain distinct from the target-owned journal.
func (s *Store) ImportLegacy(file string) (map[string]int, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 64*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 64*1024*1024 {
		return nil, errors.New("legacy export exceeds 64 MiB")
	}
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		return nil, errors.New("invalid legacy JSON; source left unchanged")
	}
	if _, ok := raw["machines"].(map[string]any); !ok {
		return nil, errors.New("legacy machines object required")
	}
	secrets := []string{}
	collectSecrets(raw, &secrets)
	raw = scrub(raw, secrets).(map[string]any)
	type legacy struct {
		Machines       map[string]map[string]any `json:"machines"`
		Jobs           []JobView                 `json:"jobs"`
		Software       []CatalogItem             `json:"software"`
		SoftwareStatus map[string]map[string]any `json:"softwareStatus"`
	}
	v, err := decode[legacy](encode(raw))
	if err != nil {
		return nil, errors.New("legacy data shape invalid")
	}
	counts := map[string]int{"machines": 0, "jobs": 0, "software": 0}
	appendArchive := func(a LegacyArchive) error {
		a.ID = digest(encode(a))
		var exists int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM imports WHERE json_extract(body,'$.id')=?", a.ID).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			return nil
		}
		_, err := s.Append("import", a)
		return err
	}
	ids := []string{}
	for id := range v.Machines {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		m := v.Machines[id]
		if demo, _ := m["demo"].(bool); demo {
			continue
		}
		defaults := map[string]any{"hostname": id, "user": "", "os": "Windows", "cpu": nil, "gpu": nil, "memory": nil, "mappedDrives": []any{}, "lastSeen": 0, "metricsLimited": true, "frozenHint": false}
		for key, value := range defaults {
			if _, ok := m[key]; !ok {
				m[key] = value
			}
		}
		m["id"] = id
		m["demo"] = false
		m["status"] = "offline"
		m["transport"] = "mesh"
		m["lastSeenLabel"] = "Historical inventory — awaiting identity binding"
		m["stale"] = true
		if err = appendArchive(LegacyArchive{Machines: []map[string]any{m}, SoftwareStatus: map[string]map[string]any{id: v.SoftwareStatus[id]}}); err != nil {
			return counts, err
		}
		counts["machines"]++
	}
	for _, j := range v.Jobs {
		real := false
		for _, id := range j.MachineIDs {
			m := v.Machines[id]
			demo, _ := m["demo"].(bool)
			if m != nil && !demo {
				real = true
			}
		}
		if !real {
			continue
		}
		j.Historical = true
		j.Status = "done"
		j.Delivery = "historical"
		if j.Payload == nil {
			j.Payload = map[string]string{}
		}
		delete(j.Payload, "password")
		delete(j.Payload, "token")
		for id, r := range j.Results {
			r.CommandID = ""
			r.Output = ""
			if r.Status == "queued" || r.Status == "running" {
				r.Status = "unknown"
				r.Message = "Legacy work was interrupted or queued; review manually. It will not be replayed."
			}
			j.Results[id] = r
		}
		if err = appendArchive(LegacyArchive{Jobs: []JobView{j}}); err != nil {
			return counts, err
		}
		counts["jobs"]++
	}
	for _, item := range v.Software {
		if _, err = s.EditCatalog(item, false); err != nil {
			return counts, err
		}
		counts["software"]++
	}
	return counts, nil
}

func (n *Node) mergeImports(s *Snapshot) error {
	hidden := map[string]bool{}
	hr, err := n.Store.db.Query("SELECT id FROM hidden")
	if err != nil {
		return err
	}
	for hr.Next() {
		var id string
		if err = hr.Scan(&id); err != nil {
			hr.Close()
			return err
		}
		hidden[id] = true
	}
	hr.Close()
	rows, err := n.Store.db.Query("SELECT body FROM imports ORDER BY id")
	if err != nil {
		return err
	}
	defer rows.Close()
	bound := map[string]string{}
	machines := map[string]bool{}
	jobs := map[string]bool{}
	archives := map[string]bool{}
	for _, m := range s.Machines {
		machines[fmt.Sprint(m["id"])] = true
		if id, ok := m["legacyId"].(string); ok && id != "" {
			bound[id] = fmt.Sprint(m["id"])
		}
	}
	for _, j := range s.Jobs {
		jobs[j.ID] = true
	}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return err
		}
		a, err := decode[LegacyArchive](b)
		if err != nil {
			return err
		}
		if archives[a.ID] {
			continue
		}
		archives[a.ID] = true
		for _, m := range a.Machines {
			id := fmt.Sprint(m["id"])
			if !machines[id] && bound[id] == "" {
				m["historical"] = true
				s.Machines = append(s.Machines, m)
				machines[id] = true
			}
		}
		for _, j := range a.Jobs {
			if !jobs[j.ID] && !hidden[j.ID] {
				s.Jobs = append(s.Jobs, j)
				jobs[j.ID] = true
			}
		}
		for id, apps := range a.SoftwareStatus {
			if target := bound[id]; target != "" {
				id = target
			}
			if s.SoftwareStatus[id] == nil {
				s.SoftwareStatus[id] = map[string]any{}
			}
			for key, value := range apps {
				if _, exists := s.SoftwareStatus[id][key]; !exists {
					s.SoftwareStatus[id][key] = value
				}
			}
		}
	}
	sort.Slice(s.Jobs, func(i, j int) bool { return s.Jobs[i].CreatedAt > s.Jobs[j].CreatedAt })
	if len(s.Jobs) > 120 {
		s.Jobs = s.Jobs[:120]
	}
	return rows.Err()
}

// RestoreHistory creates a NEW identity and verifies every signed event. It
// deliberately does not restore a stale execution inbox, sequence writer, or
// private key. The restored node must be admitted before it can issue work.
func RestoreHistory(backup, dir string) (*Store, error) {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return nil, errors.New("restore destination must be empty")
	}
	abs, err := filepath.Abs(backup)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.ToSlash(abs)+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var check, fleet string
	if err = db.QueryRow("PRAGMA integrity_check").Scan(&check); err != nil || check != "ok" {
		return nil, fmt.Errorf("backup integrity check failed: %s (%v)", check, err)
	}
	if err = db.QueryRow("SELECT value FROM meta WHERE key='fleet'").Scan(&fleet); err != nil {
		return nil, err
	}
	s, err := Open(dir, true)
	if err != nil {
		return nil, err
	}
	s.FleetID = fleet
	if _, err = s.db.Exec("INSERT INTO meta VALUES ('fleet',?)", fleet); err != nil {
		s.Close()
		return nil, err
	}
	rows, err := db.Query("SELECT body FROM events ORDER BY lamport,origin,seq")
	if err != nil {
		s.Close()
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			s.Close()
			return nil, err
		}
		e, err := decode[Event](b)
		if err != nil {
			s.Close()
			return nil, err
		}
		if _, err = s.Ingest(Envelope{Events: []Event{e}}); err != nil {
			s.Close()
			return nil, err
		}
	}
	if err = rows.Err(); err != nil {
		s.Close()
		return nil, err
	}
	rows.Close()
	outputs, err := db.Query("SELECT hash,body FROM outputs WHERE expires>?", nowMS())
	if err != nil {
		s.Close()
		return nil, err
	}
	defer outputs.Close()
	for outputs.Next() {
		var hash, body string
		if err = outputs.Scan(&hash, &body); err != nil {
			s.Close()
			return nil, err
		}
		if _, err = s.Ingest(Envelope{Outputs: map[string]string{hash: body}}); err != nil {
			s.Close()
			return nil, err
		}
	}
	if err = outputs.Err(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}
