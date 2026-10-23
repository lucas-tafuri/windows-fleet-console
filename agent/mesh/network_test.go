package mesh

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func testNetwork(t *testing.T, count int) ([]*Node, []*httptest.Server) {
	t.Helper()
	stores := fleet(t, count)
	nodes := []*Node{}
	servers := []*httptest.Server{}
	for _, s := range stores {
		n, err := NewNode(s, member(s))
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewUnstartedServer(n.PeerHandler())
		server.TLS = &tls.Config{Certificates: []tls.Certificate{n.certificate}, ClientAuth: tls.RequireAnyClientCert, MinVersion: tls.VersionTLS13}
		server.StartTLS()
		n.Self.PeerURL = server.URL
		nodes = append(nodes, n)
		servers = append(servers, server)
		t.Cleanup(func() { n.cancel(); server.Close() })
	}
	for _, n := range nodes {
		for _, p := range nodes {
			if err := n.Store.Address(p.Self); err != nil {
				t.Fatal(err)
			}
		}
	}
	return nodes, servers
}

func TestTLSMeshSurvivesFounderLoss(t *testing.T) {
	nodes, servers := testNetwork(t, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	servers[0].Close()
	a, b := nodes[1], nodes[2]
	e, c := submit(t, a.Store, b.Self.ID, "launch", "online")
	if err := a.Deliver(ctx, e); err != nil {
		t.Fatal(err)
	}
	next, err := b.Store.Next(false)
	if err != nil || next == nil || next.ID != c.ID {
		t.Fatal(next, err)
	}
	if err = b.Store.Complete(*next, Result{Status: "ok", Message: "founder is gone"}); err != nil {
		t.Fatal(err)
	}
	if err = a.Sync(ctx, b.Self); err != nil {
		t.Fatal(err)
	}
	r, err := a.Store.Receipt(c.ID)
	if err != nil || r.Status != "ok" {
		t.Fatal(r, err)
	}
	snapshot, err := a.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Jobs) != 1 || snapshot.Jobs[0].ReplicaCount < 1 {
		t.Fatal("durability copies missing", snapshot.Jobs)
	}
	servers[2].Close()
	_, local := submit(t, a.Store, a.Self.ID, "launch", "online")
	if _, err = a.Store.Accept(local.ID, false); err != nil {
		t.Fatal(err)
	}
	next, err = a.Store.Next(false)
	if err != nil || next == nil {
		t.Fatal("single survivor cannot work", next, err)
	}
}

func TestLostAcceptanceResponseDoesNotDuplicateExecution(t *testing.T) {
	nodes, servers := testNetwork(t, 2)
	a, b := nodes[0], nodes[1]
	handler := b.PeerHandler()
	servers[1].Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/peer/v2/accept" {
			record := httptest.NewRecorder()
			handler.ServeHTTP(record, r)
			if record.Code != 200 {
				t.Errorf("accept failed: %s", record.Body.String())
			}
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				connection.Close()
			}
			return
		}
		handler.ServeHTTP(w, r)
	})
	e, c := submit(t, a.Store, b.Self.ID, "launch", "online")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.Deliver(ctx, e); err == nil {
		t.Fatal("expected dropped acknowledgment")
	}
	r, err := b.Store.Receipt(c.ID)
	if err != nil || r.Status != "queued" {
		t.Fatal(r, err)
	}
	next, err := b.Store.Next(false)
	if err != nil || next == nil {
		t.Fatal(next, err)
	}
	if err = b.Store.Complete(*next, Result{Status: "ok"}); err != nil {
		t.Fatal(err)
	}
	if err = a.Sync(ctx, b.Self); err != nil {
		t.Fatal(err)
	}
	if _, err = b.Store.Accept(c.ID, false); err != nil {
		t.Fatal(err)
	}
	if next, err = b.Store.Next(false); err != nil || next != nil {
		t.Fatal("duplicated after lost ACK", next, err)
	}
}

func TestTLSIdentityAndRevokedConnection(t *testing.T) {
	nodes, _ := testNetwork(t, 2)
	a, b := nodes[0], nodes[1]
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	wrong := b.Self
	wrong.ID = a.Self.ID
	if err := a.Sync(ctx, wrong); err == nil {
		t.Fatal("wrong certificate accepted")
	}
	if _, err := b.Store.Append("remove", map[string]string{"id": a.Self.ID}); err != nil {
		t.Fatal(err)
	}
	var hello Hello
	if err := a.request(ctx, b.Self, "/peer/v2/hello", nil, &hello); err == nil {
		t.Fatal("removed identity accepted")
	}
}

func TestJoinRequiresApprovalAndFingerprint(t *testing.T) {
	nodes, _ := testNetwork(t, 1)
	root := nodes[0]
	s := newStore(t)
	joining, err := NewNode(s, member(s))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- joining.Join(ctx, root.Self.PeerURL, root.Self.ID, root.Store.FleetID) }()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		_ = root.Store.db.QueryRow("SELECT COUNT(*) FROM joins").Scan(&count)
		if count == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if root.Store.Active(joining.Self.ID) {
		t.Fatal("auto-approved")
	}
	if _, err = root.Approve(joining.Self.ID); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if !s.Active(s.Identity.ID()) {
		t.Fatal("approval missing")
	}
}

func TestMissingOutputRetriesAfterEventCursorAdvances(t *testing.T) {
	nodes, _ := testNetwork(t, 2)
	a, b := nodes[0], nodes[1]
	_, c := submit(t, b.Store, b.Self.ID, "launch", "online")
	if _, err := b.Store.Accept(c.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Store.Next(false); err != nil {
		t.Fatal(err)
	}
	if err := b.Store.Complete(c, Result{Status: "ok", Output: "durable output"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.Sync(ctx, b.Self); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Store.db.Exec("DELETE FROM outputs"); err != nil {
		t.Fatal(err)
	}
	// No new events exist; reconciliation must still repair the missing blob.
	if err := a.Sync(ctx, b.Self); err != nil {
		t.Fatal(err)
	}
	var output string
	if err := a.Store.db.QueryRow("SELECT body FROM outputs").Scan(&output); err != nil || output != "durable output" {
		t.Fatal(output, err)
	}
}

func TestFiftyNodesTenMiBConvergence(t *testing.T) {
	if os.Getenv("FLEET_SCALE_TEST") != "1" {
		t.Skip("set FLEET_SCALE_TEST=1 for the 50-node 10 MiB integration test")
	}
	nodes, servers := testNetwork(t, 50)
	servers[0].Close()
	source := nodes[1]
	for i := 0; i < 100; i++ {
		if _, err := source.Store.Append("inventory", Observation{Hostname: "scale", OS: strings.Repeat("x", 105000)}); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	budget := 60 * time.Second
	if configured := os.Getenv("FLEET_SCALE_BUDGET_SECONDS"); configured != "" {
		seconds, err := strconv.Atoi(configured)
		if err != nil || seconds < 1 || seconds > 240 {
			t.Fatal("invalid scale-test budget")
		}
		budget = time.Duration(seconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	slots := make(chan struct{}, 8)
	var wg sync.WaitGroup
	errs := make(chan error, 48)
	for _, n := range nodes[2:] {
		wg.Add(1)
		go func(n *Node) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			if err := n.Sync(ctx, source.Self); err != nil {
				errs <- err
			}
		}(n)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	want, _ := source.Store.Cursors()
	for _, n := range nodes[2:] {
		got, _ := n.Store.Cursors()
		a, _ := json.Marshal(want)
		b, _ := json.Marshal(got)
		if string(a) != string(b) {
			t.Errorf("node failed to converge")
		}
	}
	if !t.Failed() {
		t.Logf("49 surviving nodes converged 10 MiB after founder loss in %s (budget %s)", time.Since(start), budget)
	}
	if time.Since(start) > budget {
		t.Fatal("convergence budget exceeded")
	}
}
