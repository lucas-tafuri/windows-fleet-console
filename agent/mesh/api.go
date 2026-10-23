package mesh

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type JobResultView struct {
	MachineID    string `json:"machineId"`
	Hostname     string `json:"hostname"`
	CommandID    string `json:"commandId"`
	Status       string `json:"status"`
	Message      string `json:"message"`
	Via          string `json:"via,omitempty"`
	Output       string `json:"output,omitempty"`
	FinishedAt   int64  `json:"finishedAt,omitempty"`
	ReplicaCount int    `json:"replicaCount"`
}
type JobView struct {
	ID           string                   `json:"id"`
	Kind         string                   `json:"kind"`
	CreatedAt    int64                    `json:"createdAt"`
	Payload      map[string]string        `json:"payload"`
	MachineIDs   []string                 `json:"machineIds"`
	Results      map[string]JobResultView `json:"results"`
	Status       string                   `json:"status"`
	Delivery     string                   `json:"delivery"`
	ExpiresAt    int64                    `json:"expiresAt,omitempty"`
	RetryOf      string                   `json:"retryOf,omitempty"`
	ReplicaCount int                      `json:"replicaCount"`
	Historical   bool                     `json:"historical,omitempty"`
}
type Snapshot struct {
	Machines          []map[string]any          `json:"machines"`
	Jobs              []JobView                 `json:"jobs"`
	Software          []CatalogItem             `json:"software"`
	SoftwareStatus    map[string]map[string]any `json:"softwareStatus"`
	PendingJoins      []map[string]any          `json:"pendingJoins"`
	MapPrefs          map[string]any            `json:"mapPrefs"`
	DemoActive        bool                      `json:"demoActive"`
	PinRequired       bool                      `json:"pinRequired"`
	Unlocked          bool                      `json:"unlocked"`
	Unprotected       bool                      `json:"unprotected"`
	ServerTime        int64                     `json:"serverTime"`
	NodeID            string                    `json:"nodeId"`
	FleetID           string                    `json:"fleetId"`
	Peers             []PeerState               `json:"peers"`
	ClockUncertain    bool                      `json:"clockUncertain"`
	Removed           bool                      `json:"removed"`
	DuplicateIdentity bool                      `json:"duplicateIdentity"`
}

func (n *Node) Snapshot() (Snapshot, error) {
	s := Snapshot{Machines: []map[string]any{}, Jobs: []JobView{}, Software: []CatalogItem{}, SoftwareStatus: map[string]map[string]any{}, PendingJoins: []map[string]any{}, MapPrefs: map[string]any{"letter": "Z", "unc": "", "username": "", "password": "", "uncHistory": []string{}}, Unlocked: true, Unprotected: true, ServerTime: nowMS(), NodeID: n.Self.ID, FleetID: n.Store.FleetID, Peers: []PeerState{}, ClockUncertain: n.clockUncertain(), Removed: !n.Store.Active(n.Self.ID)}
	members, err := n.Store.Members()
	if err != nil {
		return s, err
	}
	var preferences []byte
	if n.Store.db.QueryRow("SELECT value FROM meta WHERE key='mapPrefs'").Scan(&preferences) == nil {
		if prefs, err := decode[map[string]any](preferences); err == nil {
			prefs["password"] = ""
			s.MapPrefs = prefs
		}
	}
	s.Software, err = n.Store.Catalog()
	if err != nil {
		return s, err
	}
	inventory := map[string]Observation{}
	rows, err := n.Store.db.Query("SELECT id,body FROM inventory")
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var id string
		var b []byte
		if err = rows.Scan(&id, &b); err != nil {
			rows.Close()
			return s, err
		}
		o, _ := decode[Observation](b)
		inventory[id] = o
	}
	rows.Close()
	memberMap := map[string]Member{}
	n.mu.RLock()
	s.DuplicateIdentity = n.duplicate
	for _, m := range members {
		memberMap[m.ID] = m
		if m.Removed {
			continue
		}
		o := inventory[m.ID]
		last := int64(0)
		state := PeerState{Member: m, Observation: o}
		if m.ID == n.Self.ID {
			o = n.observation
			last = nowMS()
		} else if p := n.peers[m.ID]; p != nil {
			state = *p
			o = p.Observation
			last = p.LastSeen
		}
		if m.ID != n.Self.ID {
			s.Peers = append(s.Peers, state)
		}
		if o.Hostname == "" {
			o.Hostname = m.Hostname
		}
		if o.OS == "" {
			o.OS = "Windows"
		}
		if o.MappedDrives == nil {
			o.MappedDrives = []map[string]string{}
		}
		row, _ := decode[map[string]any](encode(o))
		row["id"] = m.ID
		row["demo"] = false
		row["lastSeen"] = last
		row["transport"] = "mesh"
		row["dashboardUrl"] = m.DashboardURL
		row["legacyId"] = m.LegacyID
		status := "online"
		label := "just now"
		if nowMS()-last > 20000 {
			status = "offline"
			label = "unreachable from this node"
		} else if o.Frozen {
			status = "frozen"
		} else if o.Limited && o.CPU == nil && o.Memory == nil {
			status = "limited"
		} else if o.CPU != nil && *o.CPU >= 85 || o.Memory != nil && *o.Memory >= 90 {
			status = "under_load"
		}
		row["status"] = status
		row["lastSeenLabel"] = label
		row["stale"] = status == "offline"
		s.Machines = append(s.Machines, row)
	}
	n.mu.RUnlock()
	sort.Slice(s.Machines, func(i, j int) bool {
		return fmt.Sprint(s.Machines[i]["hostname"]) < fmt.Sprint(s.Machines[j]["hostname"])
	})
	jobs, err := n.jobViews(memberMap, s.SoftwareStatus, s.Software)
	if err != nil {
		return s, err
	}
	s.Jobs = jobs
	rows, err = n.Store.db.Query("SELECT body,created FROM joins WHERE created>?", nowMS()-int64(10*time.Minute/time.Millisecond))
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var b []byte
		var created int64
		if err = rows.Scan(&b, &created); err != nil {
			rows.Close()
			return s, err
		}
		m, _ := decode[Member](b)
		s.PendingJoins = append(s.PendingJoins, map[string]any{"id": m.ID, "hostname": m.Hostname, "user": "", "os": "Windows", "ip": m.PeerURL, "createdAt": created, "status": "pending", "fingerprint": m.ID, "legacyId": m.LegacyID})
	}
	rows.Close()
	if err = n.mergeImports(&s); err != nil {
		return s, err
	}
	return s, nil
}

func (n *Node) jobViews(members map[string]Member, software map[string]map[string]any, catalog []CatalogItem) ([]JobView, error) {
	rows, err := n.Store.db.Query(`SELECT c.body,c.event_id,COALESCE(r.body,''),COALESCE(r.event_id,''),COALESCE(d.status,''),COALESCE(o.body,''),
 (SELECT COUNT(*) FROM replicas WHERE event_id=c.event_id), (SELECT COUNT(*) FROM replicas WHERE event_id=r.event_id)
 FROM commands c LEFT JOIN receipts r ON r.id=c.id LEFT JOIN delivery d ON d.id=c.id
 LEFT JOIN outputs o ON o.hash=json_extract(r.body,'$.outputHash') AND o.expires>?
 ORDER BY json_extract(c.body,'$.createdAt'),c.id`, nowMS())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := map[string]*JobView{}
	for rows.Next() {
		var cb, rb []byte
		var ce, re, delivery, output string
		var commandCopies, receiptCopies int
		if err = rows.Scan(&cb, &ce, &rb, &re, &delivery, &output, &commandCopies, &receiptCopies); err != nil {
			return nil, err
		}
		c, err := decode[Command](cb)
		if err != nil {
			return nil, err
		}
		job := groups[c.JobID]
		if job == nil {
			job = &JobView{ID: c.JobID, Kind: c.Kind, Payload: c.Payload, CreatedAt: c.CreatedAt, Results: map[string]JobResultView{}, MachineIDs: []string{}, Status: "done", Delivery: c.Delivery, ExpiresAt: c.ExpiresAt, RetryOf: c.RetryOf, ReplicaCount: commandCopies}
			groups[c.JobID] = job
		}
		job.ReplicaCount = min(job.ReplicaCount, commandCopies)
		job.MachineIDs = append(job.MachineIDs, c.Target)
		r := JobResultView{MachineID: c.Target, Hostname: members[c.Target].Hostname, CommandID: c.ID, Status: "unavailable", Message: "No target acceptance recorded", ReplicaCount: commandCopies}
		if c.Delivery == "queued" {
			r.Status = "queued"
			r.Message = "Waiting for target acceptance"
			if c.ExpiresAt <= nowMS() {
				r.Status = "expired"
				r.Message = "Delivery window expired"
			}
		}
		if delivery == "acceptance_unknown" {
			r.Status = delivery
			r.Message = "Target acknowledgment was lost; querying the original execution ID"
		}
		if len(rb) > 0 {
			receipt, err := decode[Receipt](rb)
			if err != nil {
				return nil, err
			}
			r.Status = receipt.Status
			r.Message = receipt.Message
			r.Via = receipt.Via
			r.Output = output
			r.ReplicaCount = min(commandCopies, receiptCopies)
			if terminal(r.Status) {
				r.FinishedAt = receipt.UpdatedAt
			}
			if receipt.Status == "ok" && (c.Kind == "install" || c.Kind == "uninstall" || c.Kind == "check") {
				for _, app := range catalog {
					if !strings.EqualFold(c.Payload["package"], app.Name) && !strings.EqualFold(c.Payload["package"], app.Match) && !strings.EqualFold(c.Payload["package"], app.WingetID) {
						continue
					}
					installed := c.Kind == "install"
					if receipt.Installed != nil {
						installed = *receipt.Installed
					}
					if software[c.Target] == nil {
						software[c.Target] = map[string]any{}
					}
					prior, _ := software[c.Target][app.ID].(map[string]any)
					if prior == nil || prior["lastChecked"].(int64) < receipt.UpdatedAt {
						software[c.Target][app.ID] = map[string]any{"installed": installed, "installations": receipt.Installations, "lastChecked": receipt.UpdatedAt, "via": receipt.Via, "detail": receipt.Message}
					}
				}
			}
		}
		if r.Status == "running" || r.Status == "queued" || r.Status == "held" || r.Status == "acceptance_unknown" {
			job.Status = "running"
		}
		job.Results[c.Target] = r
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	hidden := map[string]bool{}
	hr, err := n.Store.db.Query("SELECT id FROM hidden")
	if err != nil {
		return nil, err
	}
	for hr.Next() {
		var id string
		if err = hr.Scan(&id); err != nil {
			hr.Close()
			return nil, err
		}
		hidden[id] = true
	}
	hr.Close()
	jobs := []JobView{}
	for _, j := range groups {
		if !hidden[j.ID] {
			jobs = append(jobs, *j)
		}
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].CreatedAt == jobs[j].CreatedAt {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].CreatedAt > jobs[j].CreatedAt
	})
	if len(jobs) > 120 {
		jobs = jobs[:120]
	}
	return jobs, nil
}

func (n *Node) validHost(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		return false
	}
	if strings.EqualFold(h, "localhost") {
		return true
	}
	if h == "127.0.0.1" || h == "::1" {
		return true
	}
	if machine, err := os.Hostname(); err == nil && strings.EqualFold(h, machine) {
		return true
	}
	if u, err := url.Parse(n.Self.DashboardURL); err == nil && strings.EqualFold(h, u.Hostname()) {
		return true
	}
	addresses, _ := net.InterfaceAddrs()
	for _, a := range addresses {
		ip, _, _ := net.ParseCIDR(a.String())
		if ip != nil && ip.String() == h {
			return true
		}
	}
	return false
}
func (n *Node) BrowserHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
		if !n.validHost(r.Host) {
			failHTTP(w, 403, errors.New("unrecognized dashboard host"))
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			n.mu.RLock()
			duplicate := n.duplicate
			n.mu.RUnlock()
			if duplicate {
				failHTTP(w, 409, errors.New("duplicate identity detected; re-enroll this installation"))
				return
			}
			if r.Header.Get("Origin") != "http://"+r.Host || r.Header.Get("X-Fleet-Request") != "v2" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				failHTTP(w, 403, errors.New("same-origin JSON request required"))
				return
			}
			if !n.Store.Active(n.Self.ID) {
				failHTTP(w, 403, errors.New("local node removed"))
				return
			}
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			if n.UI != nil {
				n.UI.ServeHTTP(w, r)
			} else {
				http.NotFound(w, r)
			}
			return
		}
		n.handleAPI(w, r)
	})
}

func (n *Node) handleAPI(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v2/retry":
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var req struct {
			CommandID    string `json:"commandId"`
			SubmissionID string `json:"submissionId"`
		}
		if err := readJSON(w, r, &req); err != nil {
			failHTTP(w, 400, err)
			return
		}
		event, err := n.Store.RetryCommand(req.CommandID, req.SubmissionID)
		if err != nil {
			failHTTP(w, 409, err)
			return
		}
		ctx, cancel := context.WithTimeout(n.ctx, 15*time.Second)
		defer cancel()
		_ = n.Deliver(ctx, event)
		writeJSON(w, 200, map[string]bool{"ok": true})
	case "/api/v2/health":
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "nodeId": n.Self.ID, "version": n.Version, "sha256": n.BinaryHash})
	case "/api/v2/fleet", "/api/v2/peers", "/api/v2/diagnostics":
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		s, err := n.Snapshot()
		if err != nil {
			failHTTP(w, 500, err)
			return
		}
		writeJSON(w, 200, s)
	case "/api/v2/events":
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		f, ok := w.(http.Flusher)
		if !ok {
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			s, err := n.Snapshot()
			if err != nil {
				return
			}
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err = fmt.Fprintf(w, "data: %s\n\n", encode(s)); err != nil {
				return
			}
			f.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-n.ctx.Done():
				return
			case <-tick.C:
			}
		}
	case "/api/v2/jobs":
		if r.Method == "GET" {
			s, err := n.Snapshot()
			if err != nil {
				failHTTP(w, 500, err)
				return
			}
			writeJSON(w, 200, map[string]any{"jobs": s.Jobs})
			return
		}
		if r.Method == "DELETE" {
			_, err := n.Store.db.Exec("INSERT OR IGNORE INTO hidden SELECT DISTINCT job_id FROM commands; INSERT OR IGNORE INTO hidden SELECT json_extract(j.value,'$.id') FROM imports,json_each(imports.body,'$.jobs') j;")
			if err != nil {
				failHTTP(w, 500, err)
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var req SubmitRequest
		if err := readJSON(w, r, &req); err != nil {
			failHTTP(w, 400, err)
			return
		}
		events, err := n.Store.CreateCommands(req)
		if err != nil {
			failHTTP(w, 409, err)
			return
		}
		if req.Delivery != "queued" {
			var wg sync.WaitGroup
			for _, e := range events {
				wg.Add(1)
				go func(e Event) {
					defer wg.Done()
					ctx, cancel := context.WithTimeout(n.ctx, 15*time.Second)
					defer cancel()
					_ = n.Deliver(ctx, e)
				}(e)
			}
			wg.Wait()
		}
		s, err := n.Snapshot()
		if err != nil {
			failHTTP(w, 500, err)
			return
		}
		for _, job := range s.Jobs {
			if job.ID == req.SubmissionID {
				writeJSON(w, 200, map[string]any{"job": job})
				return
			}
		}
		writeJSON(w, 200, map[string]string{"id": req.SubmissionID})
	case "/api/v2/software":
		if r.Method == "GET" {
			items, err := n.Store.Catalog()
			if err != nil {
				failHTTP(w, 500, err)
				return
			}
			writeJSON(w, 200, items)
			return
		}
		if r.Method != "POST" && r.Method != "DELETE" {
			w.WriteHeader(405)
			return
		}
		var item CatalogItem
		if err := readJSON(w, r, &item); err != nil {
			failHTTP(w, 400, err)
			return
		}
		if item.ID == "" {
			id := make([]byte, 16)
			_, _ = rand.Read(id)
			item.ID = hex.EncodeToString(id)
		}
		if item.Match == "" {
			item.Match = item.Name
		}
		if _, err := n.Store.EditCatalog(item, r.Method == "DELETE"); err != nil {
			failHTTP(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]any{"item": item})
	case "/api/v2/enroll":
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		writeJSON(w, 200, map[string]any{"fleetId": n.Store.FleetID, "nodeId": n.Self.ID, "peerUrl": n.Self.PeerURL, "dashboardUrl": n.Self.DashboardURL, "protocol": Protocol})
	case "/api/v2/clock/acknowledge":
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		n.mu.Lock()
		n.clockHold = false
		n.mu.Unlock()
		writeJSON(w, 200, map[string]bool{"ok": true})
	case "/api/v2/alias":
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var v struct {
			ID       string `json:"id"`
			LegacyID string `json:"legacyId"`
		}
		if err := readJSON(w, r, &v); err != nil {
			failHTTP(w, 400, err)
			return
		}
		if _, err := n.Store.Append("alias", v); err != nil {
			failHTTP(w, 409, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		if strings.HasPrefix(r.URL.Path, "/api/v2/join/") {
			n.decideJoinHTTP(w, r, strings.TrimPrefix(r.URL.Path, "/api/v2/join/"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/v2/receipts/") {
			if r.Method != "GET" {
				w.WriteHeader(405)
				return
			}
			receipt, err := n.Store.Receipt(strings.TrimPrefix(r.URL.Path, "/api/v2/receipts/"))
			if err != nil {
				failHTTP(w, 404, err)
				return
			}
			writeJSON(w, 200, receipt)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/v2/remove/") {
			if r.Method != "POST" {
				w.WriteHeader(405)
				return
			}
			id := strings.TrimPrefix(r.URL.Path, "/api/v2/remove/")
			if _, err := n.Store.Member(id); err != nil {
				failHTTP(w, 404, err)
				return
			}
			if _, err := n.Store.Append("remove", map[string]string{"id": id}); err != nil {
				failHTTP(w, 500, err)
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		http.NotFound(w, r)
	}
}
