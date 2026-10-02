//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	} else {
		return strings.TrimSpace(string(out))
	}
	return ""
}

func fixtureRelease(t *testing.T) (string, []byte) {
	t.Helper()
	dir := t.TempDir()
	testGit(t, dir, "init", "-b", "main")
	testGit(t, dir, "config", "user.name", "Fleet Test")
	testGit(t, dir, "config", "user.email", "test@example.invalid")
	os.Mkdir(filepath.Join(dir, "dist"), 0700)
	exe, _ := os.Executable()
	binary, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range releaseFiles {
		contents := []byte("release-script")
		if strings.HasSuffix(file, ".exe") {
			contents = binary
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(file)), contents, 0600); err != nil {
			t.Fatal(err)
		}
	}
	testGit(t, dir, "add", ".")
	testGit(t, dir, "commit", "-m", "first release")
	return dir, binary
}

func TestUpdateFetchesFreshCommitAndFailsClosed(t *testing.T) {
	previousDir, previousCfg, previousExe, previousRestart := dataDir, agentCfg, exePath, restartRequested
	defer func() {
		dataDir, agentCfg, exePath, restartRequested = previousDir, previousCfg, previousExe, previousRestart
	}()
	dataDir = t.TempDir()
	if !gitOK() {
		t.Skip("Git required")
	}
	repo, binary := fixtureRelease(t)
	agentCfg = Config{Server: "http://manager", Token: "saved-token", MachineID: "saved-id", Repo: repo, Branch: "main"}
	exePath = filepath.Join(dataDir, "fleet-agent.exe")
	if err := os.WriteFile(exePath, binary, 0600); err != nil {
		t.Fatal(err)
	}
	if result := selfUpdate("first", repo, "main"); result.Status != "ok" {
		t.Fatal(result.Message)
	}
	if !restartRequested {
		t.Fatal("restart not requested after successful staging")
	}
	if cfg := loadConfig(); cfg.Token != "saved-token" || cfg.MachineID != "saved-id" {
		t.Fatal("pairing changed")
	}
	newBinary := append(append([]byte(nil), binary...), []byte("new release overlay")...)
	os.WriteFile(filepath.Join(repo, "dist", "fleet-agent.exe"), newBinary, 0600)
	testGit(t, repo, "add", ".")
	testGit(t, repo, "commit", "-m", "second release")
	latest := testGit(t, repo, "rev-parse", "HEAD")
	restartRequested = false
	result := selfUpdate("second", repo, "main")
	if result.Status != "ok" || !strings.Contains(result.Message, latest) {
		t.Fatalf("latest release not staged: %+v", result)
	}
	staged, _ := os.ReadFile(exePath + ".new")
	if !bytes.Equal(staged, newBinary) {
		t.Fatal("staged stale binary")
	}
	// A missing branch must not use the successful cache from the previous fetch.
	restartRequested = false
	result = selfUpdate("missing", repo, "missing-branch")
	if result.Status != "error" || restartRequested {
		t.Fatalf("failed fetch scheduled stale update: %+v", result)
	}
	if cfg := loadConfig(); cfg.Branch != "main" {
		t.Fatal("failed update changed saved source")
	}
	// A different requested repository must not reuse the cache's old origin.
	other, otherBinary := fixtureRelease(t)
	otherBinary = append(otherBinary, []byte("other repository")...)
	os.WriteFile(filepath.Join(other, "dist", "fleet-agent.exe"), otherBinary, 0600)
	testGit(t, other, "add", ".")
	testGit(t, other, "commit", "-m", "other source")
	result = selfUpdate("other", other, "main")
	if result.Status != "ok" {
		t.Fatal(result.Message)
	}
	staged, _ = os.ReadFile(exePath + ".new")
	if !bytes.Equal(staged, otherBinary) {
		t.Fatal("used the previous repository")
	}
	// Invalid artifacts cannot stage a restart even when Git fetch succeeds.
	restartRequested = false
	os.WriteFile(filepath.Join(other, "dist", "fleet-agent.exe"), []byte("<html>error</html>"), 0600)
	testGit(t, other, "add", ".")
	testGit(t, other, "commit", "-m", "invalid binary")
	result = selfUpdate("invalid", other, "main")
	if result.Status != "error" || restartRequested {
		t.Fatal("invalid binary accepted")
	}
}

func TestGitHubFallbackPinsRequestedBranchAndAllArtifacts(t *testing.T) {
	previousAPI, previousRaw := githubAPI, githubRaw
	defer func() { githubAPI, githubRaw = previousAPI, previousRaw }()
	sha := strings.Repeat("a", 40)
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.EscapedPath())
		if r.Header.Get("Cache-Control") != "no-cache" {
			t.Error("missing cache bypass")
		}
		if strings.Contains(r.URL.Path, "/commits/") {
			fmt.Fprintf(w, `{"sha":%q}`, sha)
			return
		}
		if !strings.Contains(r.URL.Path, "/"+sha+"/dist/") {
			t.Error("download did not pin commit")
		}
		w.Write([]byte("test-artifact"))
	}))
	defer server.Close()
	githubAPI, githubRaw = server.URL, server.URL
	got, err := fetchGitHubRelease("https://github.com/owner/custom.git", "feature/fix", t.TempDir())
	if err != nil || got != sha {
		t.Fatalf("fallback: %s, %v", got, err)
	}
	if len(requested) != 4 || !strings.Contains(requested[0], "/owner/custom/commits/feature%2Ffix") {
		t.Fatalf("wrong source: %v", requested)
	}
}

func TestUpdateReceiptSurvivesFailedDelivery(t *testing.T) {
	oldDir := dataDir
	dataDir = t.TempDir()
	defer func() { dataDir = oldDir }()
	result := JobResult{JobID: "update", Status: "ok", Message: "Installed commit abc"}
	raw, _ := json.Marshal(result)
	os.WriteFile(filepath.Join(dataDir, "update-result.json"), append([]byte("\xef\xbb\xbf"), raw...), 0600)
	c := &Client{}
	c.addResult(*readUpdateReceipt())
	c.acknowledgeResults([]JobResult{{JobID: "update", Status: "ok", Message: "Staged commit abc"}})
	if readUpdateReceipt() == nil || len(c.pending) != 1 {
		t.Fatal("earlier acknowledgment removed final receipt")
	}
	c.acknowledgeResults([]JobResult{result})
	if readUpdateReceipt() != nil || len(c.pending) != 0 {
		t.Fatal("acknowledged result not cleared")
	}
}
