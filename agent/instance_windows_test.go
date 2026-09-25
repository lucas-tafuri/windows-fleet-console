//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func TestInstanceAcrossProcesses(t *testing.T) {
	key := fmt.Sprintf(`Global\FleetAgentTest-%d`, os.Getpid())
	release, err := acquireNamedInstance(key, false)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	// Run only the mutex test helper; never run the installed agent or its setup.
	cmd := exec.Command(os.Args[0], "-test.run=^TestInstanceChild$")
	cmd.Env = append(os.Environ(), "FLEET_TEST_MUTEX="+key)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("duplicate was not rejected: %v %s", err, out)
	}
}

func TestInstanceChild(t *testing.T) {
	key := os.Getenv("FLEET_TEST_MUTEX")
	if key == "" {
		t.Skip("subprocess helper")
	}
	release, err := acquireNamedInstance(key, false)
	if err == nil {
		release()
		t.Fatal("second process acquired the running agent's lock")
	}
}

func TestInstanceCanRestart(t *testing.T) {
	key := fmt.Sprintf(`Global\FleetAgentRestartTest-%d`, os.Getpid())
	release, err := acquireNamedInstance(key, false)
	if err != nil {
		t.Fatal(err)
	}
	release()
	nextRelease, err := acquireNamedInstance(key, false)
	if err != nil {
		t.Fatalf("restart could not acquire released lock: %v", err)
	}
	nextRelease()
}
