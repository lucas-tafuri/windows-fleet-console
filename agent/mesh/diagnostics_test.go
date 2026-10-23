package mesh

import "testing"

func TestDiagnosticValidationAndCrashRecovery(t *testing.T) {
	s := fleet(t, 1)[0]
	req := SubmitRequest{SubmissionID: "diagnostic-report-test-001", Kind: "diagnostics", MachineIDs: []string{s.Identity.ID()}, Payload: map[string]string{"profile": "disks"}}
	for _, payload := range []map[string]string{{"profile": "system; whoami"}, {"profile": "system", "script": "whoami"}, {}} {
		req.Payload = payload
		if _, err := s.CreateCommands(req); err == nil {
			t.Fatal("accepted arbitrary diagnostic parameters")
		}
	}
	req.Payload = map[string]string{"profile": "disks"}
	events, err := s.CreateCommands(req)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := decode[Command](events[0].Body)
	if _, err = s.Accept(c.ID, false); err != nil {
		t.Fatal(err)
	}
	if next, err := s.Next(false); err != nil || next == nil {
		t.Fatal(next, err)
	}
	if err = s.Recover(); err != nil {
		t.Fatal(err)
	}
	next, err := s.Next(false)
	if err != nil || next == nil || next.ID != c.ID {
		t.Fatal("read-only report did not resume", next, err)
	}
	if err = s.Complete(*next, Result{Status: "ok", Output: `{"rows":[]}`}); err != nil {
		t.Fatal(err)
	}
	if receipt, err := s.Accept(c.ID, false); err != nil || receipt.Status != "ok" {
		t.Fatal(receipt, err)
	}
	if next, err := s.Next(false); err != nil || next != nil {
		t.Fatal("completed report repeated", next, err)
	}
}

func TestTargetRejectsInvalidRelayedDiagnostic(t *testing.T) {
	f := fleet(t, 2)
	c := Command{ID: commandID(f[0].FleetID, "invalid-diagnostic", f[1].Identity.ID()), JobID: "invalid-diagnostic", Target: f[1].Identity.ID(), Kind: "diagnostics", Payload: map[string]string{"profile": "shell"}, Delivery: "online", CreatedAt: nowMS()}
	if _, err := f[0].Append("command", c); err != nil {
		t.Fatal(err)
	}
	copyEvents(t, f[0], f[1])
	r, err := f[1].Accept(c.ID, false)
	if err != nil || r.Status != "error" {
		t.Fatal("target accepted invalid report", r, err)
	}
	if next, err := f[1].Next(false); err != nil || next != nil {
		t.Fatal("invalid report entered executor", next, err)
	}
}
