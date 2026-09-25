package main

import (
	"os"
	"testing"
)

func TestPairingSurvivesConfigUpdates(t *testing.T) {
	dataDir = t.TempDir()
	want := Config{Server: "http://console:43123", Token: "approved-token", MachineID: "pc-existing", Repo: defaultRepo, Branch: defaultBranch}
	if err := saveConfig(want); err != nil {
		t.Fatal(err)
	}
	got := loadConfig()
	got.Branch = "updated"
	if err := saveConfig(got); err != nil {
		t.Fatal(err)
	}
	got = loadConfig()
	if got.Token != want.Token || got.MachineID != want.MachineID || got.Server != want.Server {
		t.Fatalf("pairing changed: %+v", got)
	}
}

func TestInvalidPairingIsNotOverwritten(t *testing.T) {
	dataDir = t.TempDir()
	os.WriteFile(configPath(), []byte("broken"), 0600)
	defer func() {
		if recover() == nil {
			t.Error("expected invalid pairing to stop startup")
		}
		raw, _ := os.ReadFile(configPath())
		if string(raw) != "broken" {
			t.Error("saved pairing was replaced")
		}
	}()
	loadConfig()
}
