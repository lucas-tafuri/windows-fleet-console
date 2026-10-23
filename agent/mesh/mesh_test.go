package mesh

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func member(s *Store) Member {
	return s.Identity.Member("PC-"+s.Identity.ID()[:6], "https://127.0.0.1:43125", "http://127.0.0.1:43123")
}
func fleet(t *testing.T, count int) []*Store {
	t.Helper()
	out := []*Store{newStore(t)}
	if err := out[0].Bootstrap(member(out[0])); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < count; i++ {
		s := newStore(t)
		if _, err := out[0].Append("member", member(s)); err != nil {
			t.Fatal(err)
		}
		env, err := out[0].Export(nil, MaxBatch)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.InitializeJoin(out[0].FleetID, env); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	for _, s := range out[1:] {
		copyEvents(t, out[0], s)
	}
	return out
}
func copyEvents(t *testing.T, from, to *Store) {
	t.Helper()
	for {
		c, err := to.Cursors()
		if err != nil {
			t.Fatal(err)
		}
		env, err := from.Export(c, MaxBatch)
		if err != nil {
			t.Fatal(err)
		}
		if len(env.Events) == 0 {
			return
		}
		if _, err = to.Ingest(env); err != nil {
			t.Fatal(err)
		}
	}
}
func submit(t *testing.T, s *Store, target, kind, delivery string) (Event, Command) {
	t.Helper()
	req := SubmitRequest{SubmissionID: digest([]byte(t.Name() + target + kind + delivery)), Kind: kind, MachineIDs: []string{target}, Payload: map[string]string{"target": "test.exe"}, Delivery: delivery}
	if delivery == "queued" {
		req.ExpiresAt = nowMS() + 60000
	}
	events, err := s.CreateCommands(req)
	if err != nil {
		t.Fatal(err)
	}
	c, err := decode[Command](events[0].Body)
	if err != nil {
		t.Fatal(err)
	}
	return events[0], c
}

func TestReplicationNeverAcceptsImmediateCommands(t *testing.T) {
	f := fleet(t, 2)
	_, c := submit(t, f[0], f[1].Identity.ID(), "launch", "online")
	copyEvents(t, f[0], f[1])
	next, err := f[1].Next(false)
	if err != nil || next != nil {
		t.Fatalf("history executed: %v %v", next, err)
	}
	r, err := f[1].Accept(c.ID, false)
	if err != nil || r.Status != "queued" {
		t.Fatal(r, err)
	}
	if _, err = f[1].Accept(c.ID, false); err != nil {
		t.Fatal(err)
	}
	next, err = f[1].Next(false)
	if err != nil || next == nil {
		t.Fatal(next, err)
	}
	if err = f[1].Complete(*next, Result{Status: "ok", Message: "launched"}); err != nil {
		t.Fatal(err)
	}
	r, err = f[1].Accept(c.ID, false)
	if err != nil || r.Status != "ok" {
		t.Fatal(r, err)
	}
	next, err = f[1].Next(false)
	if err != nil || next != nil {
		t.Fatal("duplicate execution", next, err)
	}
	copyEvents(t, f[1], f[0])
	if r, err = f[0].Receipt(c.ID); err != nil || r.Status != "ok" {
		t.Fatal(r, err)
	}
}

func TestCrashBoundariesAndReadOnlyRecovery(t *testing.T) {
	f := fleet(t, 1)
	s := f[0]
	_, queued := submit(t, s, s.Identity.ID(), "map_drive", "online")
	if _, err := s.Accept(queued.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Recover(); err != nil {
		t.Fatal(err)
	}
	c, err := s.Next(false)
	if err != nil || c == nil {
		t.Fatal(c, err)
	}
	if err = s.Recover(); err != nil {
		t.Fatal(err)
	}
	r, err := s.Receipt(c.ID)
	if err != nil || r.Status != "unknown" {
		t.Fatal(r, err)
	}
	if c, err = s.Next(false); err != nil || c != nil {
		t.Fatal("unsafe replay", c, err)
	}
	_, check := submit(t, s, s.Identity.ID(), "check", "online")
	if _, err = s.Accept(check.ID, false); err != nil {
		t.Fatal(err)
	}
	if c, err = s.Next(false); err != nil || c == nil {
		t.Fatal(c, err)
	}
	if err = s.Recover(); err != nil {
		t.Fatal(err)
	}
	if c, err = s.Next(false); err != nil || c == nil || c.ID != check.ID {
		t.Fatal(c, err)
	}
}

func TestQueuedExpiryAndClockHold(t *testing.T) {
	f := fleet(t, 1)
	s := f[0]
	_, c := submit(t, s, s.Identity.ID(), "launch", "queued")
	r, err := s.Accept(c.ID, true)
	if err != nil || r.Status != "held" {
		t.Fatal(r, err)
	}
	if next, err := s.Next(true); err != nil || next != nil {
		t.Fatal(next, err)
	}
	if next, err := s.Next(false); err != nil || next == nil {
		t.Fatal(next, err)
	}
	req := SubmitRequest{SubmissionID: digest([]byte("expired")), Kind: "launch", MachineIDs: []string{s.Identity.ID()}, Delivery: "queued", ExpiresAt: nowMS() + 100}
	events, err := s.CreateCommands(req)
	if err != nil {
		t.Fatal(err)
	}
	expired, _ := decode[Command](events[0].Body)
	time.Sleep(120 * time.Millisecond)
	r, err = s.Accept(expired.ID, false)
	if err != nil || r.Status != "expired" {
		t.Fatal(r, err)
	}
}

func TestSixNodePartitionAndConvergence(t *testing.T) {
	f := fleet(t, 6)
	for i, s := range f {
		_, c := submit(t, s, s.Identity.ID(), "launch", "online")
		if _, err := s.Accept(c.ID, false); err != nil {
			t.Fatal(err)
		}
		next, err := s.Next(false)
		if err != nil || next == nil {
			t.Fatal(next, err)
		}
		if err = s.Complete(*next, Result{Status: "ok", Message: "partition result"}); err != nil {
			t.Fatal(err)
		}
		start := i / 3 * 3
		for j := start; j < start+3; j++ {
			if i != j {
				copyEvents(t, s, f[j])
			}
		}
	}
	for _, s := range f[1:] {
		copyEvents(t, s, f[0])
	}
	for _, s := range f[1:] {
		copyEvents(t, f[0], s)
	}
	want, _ := f[0].Cursors()
	for _, s := range f {
		got, _ := s.Cursors()
		if !bytes.Equal(encode(want), encode(got)) {
			t.Fatalf("divergence %v %v", want, got)
		}
		if next, err := s.Next(false); err != nil || next != nil {
			t.Fatal("replayed work after merge", next, err)
		}
	}
}

func TestSequenceGapsForgeryAndForeignFleet(t *testing.T) {
	f := fleet(t, 2)
	a, _ := f[0].Append("catalog", CatalogItem{ID: "a", Name: "A"})
	b, _ := f[0].Append("catalog", CatalogItem{ID: "b", Name: "B"})
	if _, err := f[1].Ingest(Envelope{Events: []Event{b}}); err == nil {
		t.Fatal("accepted gap")
	}
	forged := a
	forged.Body = encode(CatalogItem{ID: "evil", Name: "Evil"})
	if _, err := f[1].Ingest(Envelope{Events: []Event{forged}}); err == nil {
		t.Fatal("accepted forgery")
	}
	foreign := a
	foreign.FleetID = "another"
	f[0].Identity.sign(&foreign)
	if _, err := f[1].Ingest(Envelope{Events: []Event{foreign}}); err == nil {
		t.Fatal("accepted foreign fleet")
	}
	if _, err := f[1].Ingest(Envelope{Events: []Event{a, b}}); err != nil {
		t.Fatal(err)
	}
}

func TestReceiptForgeryAndTerminalRegression(t *testing.T) {
	f := fleet(t, 2)
	_, c := submit(t, f[0], f[1].Identity.ID(), "launch", "online")
	if _, err := f[0].Append("receipt", Receipt{CommandID: c.ID, Target: c.Target, Status: "ok"}); err == nil {
		t.Fatal("non-target receipt accepted")
	}
	copyEvents(t, f[0], f[1])
	_, _ = f[1].Accept(c.ID, false)
	_, _ = f[1].Next(false)
	if err := f[1].Complete(c, Result{Status: "ok"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f[1].Append("receipt", Receipt{CommandID: c.ID, Target: c.Target, Status: "running"}); err == nil {
		t.Fatal("terminal regression accepted")
	}
}

func TestInventoryDoesNotJournalTelemetrySamples(t *testing.T) {
	f := fleet(t, 1)
	n, err := NewNode(f[0], member(f[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer n.cancel()
	value := 20.0
	o := Observation{Hostname: "PC", OS: "Windows", CPU: &value}
	if err = n.persistInventory(o); err != nil {
		t.Fatal(err)
	}
	before, _ := f[0].Cursors()
	value = 90
	if err = n.persistInventory(o); err != nil {
		t.Fatal(err)
	}
	after, _ := f[0].Cursors()
	if !bytes.Equal(encode(before), encode(after)) {
		t.Fatal("telemetry added a durable event")
	}
	var body []byte
	if err = f[0].db.QueryRow("SELECT body FROM inventory").Scan(&body); err != nil {
		t.Fatal(err)
	}
	stored, err := decode[Observation](body)
	if err != nil || stored.CPU != nil {
		t.Fatal("telemetry persisted", err)
	}
	o.OS = "Windows updated"
	if err = n.persistInventory(o); err != nil {
		t.Fatal(err)
	}
	after, _ = f[0].Cursors()
	if after[n.Self.ID] != before[n.Self.ID]+1 {
		t.Fatal("inventory change not recorded")
	}
}

func TestSubmissionIdentityAndTargetSecrets(t *testing.T) {
	f := fleet(t, 2)
	req := SubmitRequest{SubmissionID: digest([]byte("stable")), Kind: "map_drive", MachineIDs: []string{f[1].Identity.ID()}, Payload: map[string]string{"letter": "Z", "password": "secret-do-not-replicate"}, Delivery: "online"}
	first, err := f[0].CreateCommands(req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f[0].CreateCommands(req)
	if err != nil || first[0].ID != second[0].ID {
		t.Fatal(err)
	}
	c, _ := decode[Command](first[0].Body)
	if bytes.Contains(encode(first), []byte(req.Payload["password"])) {
		t.Fatal("plaintext persisted")
	}
	plain, err := f[1].Identity.open(c.Secret)
	if err != nil || plain != req.Payload["password"] {
		t.Fatal(plain, err)
	}
	if _, err = f[0].Identity.open(c.Secret); err == nil {
		t.Fatal("another node decrypted target secret")
	}
	var prefs []byte
	if err = f[0].db.QueryRow("SELECT value FROM meta WHERE key='mapPrefs'").Scan(&prefs); err != nil || bytes.Contains(prefs, []byte(req.Payload["password"])) {
		t.Fatal("local preferences missing or contain password", err)
	}
	copyEvents(t, f[0], f[1])
	if err = f[1].db.QueryRow("SELECT value FROM meta WHERE key='mapPrefs'").Scan(&prefs); err != sql.ErrNoRows {
		t.Fatal("preferences replicated", err)
	}
	req.Payload["password"] = "changed"
	if _, err = f[0].CreateCommands(req); err == nil {
		t.Fatal("changed submission accepted")
	}
}

func TestRemovalAndCatalogDeleteWin(t *testing.T) {
	f := fleet(t, 3)
	_, _ = f[0].EditCatalog(CatalogItem{ID: "app", Name: "Original"}, false)
	for _, s := range f[1:] {
		copyEvents(t, f[0], s)
	}
	_, _ = f[0].EditCatalog(CatalogItem{ID: "app"}, true)
	_, _ = f[1].EditCatalog(CatalogItem{ID: "app", Name: "Concurrent edit"}, false)
	for _, s := range f[1:] {
		copyEvents(t, f[0], s)
	}
	copyEvents(t, f[1], f[0])
	for _, s := range f {
		items, err := s.Catalog()
		if err != nil || len(items) != 0 {
			t.Fatal(items, err)
		}
	}
	_, c := submit(t, f[0], f[1].Identity.ID(), "launch", "online")
	copyEvents(t, f[0], f[1])
	_, _ = f[0].Append("remove", map[string]string{"id": f[1].Identity.ID()})
	copyEvents(t, f[0], f[1])
	if _, err := f[1].Accept(c.ID, false); err == nil {
		t.Fatal("removed target accepted work")
	}
	_, _ = f[0].Append("member", member(f[1]))
	if f[0].Active(f[1].Identity.ID()) {
		t.Fatal("old approval resurrected member")
	}
}

func TestDurableRestartCorruptionAndRestore(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Bootstrap(member(s)); err != nil {
		t.Fatal(err)
	}
	_, c := submit(t, s, s.Identity.ID(), "launch", "online")
	_, _ = s.Accept(c.ID, false)
	_, _ = s.Next(false)
	backup := filepath.Join(t.TempDir(), "backup.db")
	if err = s.Backup(backup); err != nil {
		t.Fatal(err)
	}
	id := s.Identity.ID()
	s.Close()
	s, err = Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if s.Identity.ID() != id {
		t.Fatal("identity changed")
	}
	if err = s.Recover(); err != nil {
		t.Fatal(err)
	}
	r, err := s.Receipt(c.ID)
	if err != nil || r.Status != "unknown" {
		t.Fatal(r, err)
	}
	s.Close()
	restored, err := RestoreHistory(backup, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if restored.Identity.ID() == id || restored.Active(restored.Identity.ID()) {
		t.Fatal("restore revived old execution identity")
	}
	if next, err := restored.Next(false); err != nil || next != nil {
		t.Fatal(next, err)
	}
	if err = os.WriteFile(filepath.Join(dir, "mesh.db"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if bad, err := Open(dir, false); err == nil {
		bad.Close()
		t.Fatal("corrupt database reset")
	}
}

func TestBrowserOriginProtection(t *testing.T) {
	s := fleet(t, 1)[0]
	n, err := NewNode(s, member(s))
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"", "http://evil.example", "null"} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:43123/api/v2/software", strings.NewReader(`{"name":"test"}`))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Fleet-Request", "v2")
		w := httptest.NewRecorder()
		n.BrowserHandler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(origin, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "http://evil.example:43123/api/v2/fleet", nil)
	w := httptest.NewRecorder()
	n.BrowserHandler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("DNS rebinding accepted")
	}
	r = httptest.NewRequest("GET", "http://127.0.0.1:43123/api/v2/fleet", nil)
	w = httptest.NewRecorder()
	n.BrowserHandler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
}

func TestImportIsRedactedAndNeverExecutable(t *testing.T) {
	s := fleet(t, 1)[0]
	path := filepath.Join(t.TempDir(), "fleet.json")
	legacy := `{"fleetToken":"legacy-secret","machines":{"pc":{"id":"pc","hostname":"PC","demo":false},"demo":{"demo":true}},"jobs":[{"id":"legacy-job","kind":"launch","payload":{"password":"drive-secret"},"machineIds":["pc"],"results":{"pc":{"machineId":"pc","status":"running","message":"drive-secret legacy-secret"}}}],"software":[{"id":"app","name":"App","match":"App"}],"mapPrefs":{"password":"drive-secret"}}`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	counts, err := s.ImportLegacy(path)
	if err != nil {
		t.Fatal(err)
	}
	if counts["machines"] != 1 || counts["jobs"] != 1 {
		t.Fatal(counts)
	}
	env, err := s.Export(nil, MaxBatch)
	if err != nil {
		t.Fatal(err)
	}
	b := encode(env)
	if bytes.Contains(b, []byte("drive-secret")) || bytes.Contains(b, []byte("legacy-secret")) {
		t.Fatal("import leaked secret")
	}
	if c, err := s.Next(false); err != nil || c != nil {
		t.Fatal("legacy work executable", c, err)
	}
}

func TestWriteFailureDoesNotAcknowledgeAcceptance(t *testing.T) {
	s := fleet(t, 1)[0]
	_, c := submit(t, s, s.Identity.ID(), "launch", "online")
	if _, err := s.db.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Accept(c.ID, false); err == nil {
		t.Fatal("acknowledged unwritable inbox")
	}
	if _, err := s.Receipt(c.ID); err != sql.ErrNoRows {
		t.Fatal(err)
	}
	_, _ = s.db.Exec("PRAGMA query_only=OFF")
	if _, err := s.Accept(c.ID, false); err != nil {
		t.Fatal(err)
	}
}

func TestOutputBoundAndRetention(t *testing.T) {
	s := fleet(t, 1)[0]
	_, c := submit(t, s, s.Identity.ID(), "check", "online")
	_, _ = s.Accept(c.ID, false)
	_, _ = s.Next(false)
	if err := s.Complete(c, Result{Status: "ok", Output: strings.Repeat("é", MaxOutput)}); err != nil {
		t.Fatal(err)
	}
	r, _ := s.Receipt(c.ID)
	var output string
	if err := s.db.QueryRow("SELECT body FROM outputs WHERE hash=?", r.OutputHash).Scan(&output); err != nil || len(output) > MaxOutput {
		t.Fatal(len(output), err)
	}
}

// Ensure all cryptographic types used in the protocol keep their concrete wire
// representation. There is no ambient shared secret in the public membership.
func TestIdentityEncoding(t *testing.T) {
	s := newStore(t)
	m := member(s)
	b := encode(m)
	var round Member
	if err := json.Unmarshal(b, &round); err != nil {
		t.Fatal(err)
	}
	if len(round.PublicKey) != ed25519.PublicKeySize || round.ID != m.ID {
		t.Fatal(round)
	}
	_ = context.Background()
	_ = http.MethodGet
}
