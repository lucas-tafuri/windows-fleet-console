//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

var updateLock sync.Mutex
var updateHTTP = &http.Client{Timeout: 120 * time.Second}
var githubAPI = "https://api.github.com"
var githubRaw = "https://raw.githubusercontent.com"

var releaseFiles = []string{"dist/fleet-agent.exe", "dist/install.ps1", "dist/install.cmd"}

func readUpdateReceipt() *JobResult {
	raw, err := os.ReadFile(filepath.Join(dataDir, "update-result.json"))
	if err != nil {
		return nil
	}
	raw = []byte(strings.TrimPrefix(string(raw), "\ufeff"))
	var result JobResult
	if json.Unmarshal(raw, &result) != nil || result.JobID == "" {
		return nil
	}
	return &result
}

func acknowledgeUpdateReceipt(sent JobResult) {
	if receipt := readUpdateReceipt(); receipt != nil && receipt.JobID == sent.JobID && receipt.Message == sent.Message {
		_ = os.Remove(filepath.Join(dataDir, "update-result.json"))
	}
}

type updateRequest struct {
	JobID    string `json:"jobId"`
	PID      int    `json:"pid"`
	Root     string `json:"root"`
	Revision string `json:"revision"`
	SHA256   string `json:"sha256"`
	Repo     string `json:"repo"`
}

func updateGit(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git: %s", strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, err
	}
	return out, nil
}

func fetchGitRelease(repo, branch, stage string) (string, error) {
	// A private bare cache avoids modifying an operator's checkout and fetching
	// from the wrong origin. Never read old FETCH_HEAD after a failed fetch.
	cache := filepath.Join(dataDir, "update-cache.git")
	if _, err := os.Stat(filepath.Join(cache, "HEAD")); os.IsNotExist(err) {
		if _, err := updateGit("init", "--bare", cache); err != nil {
			return "", err
		}
	}
	if _, err := updateGit("check-ref-format", "refs/heads/"+branch); err != nil {
		return "", fmt.Errorf("invalid branch: %w", err)
	}
	if _, err := updateGit("-C", cache, "fetch", "--no-tags", "--depth=1", "--", repo, "refs/heads/"+branch); err != nil {
		return "", err
	}
	head, err := updateGit("-C", cache, "rev-parse", "--verify", "FETCH_HEAD^{commit}")
	if err != nil {
		return "", err
	}
	revision := strings.TrimSpace(string(head))
	for _, file := range releaseFiles {
		contents, err := updateGit("-C", cache, "show", revision+":"+file)
		if err != nil {
			return "", fmt.Errorf("release missing %s: %w", file, err)
		}
		if err := writeUpdateFile(filepath.Join(stage, filepath.Base(file)), contents); err != nil {
			return "", err
		}
	}
	return revision, nil
}

func githubRepository(repo string) (string, error) {
	if strings.HasPrefix(repo, "git@github.com:") {
		repo = "https://github.com/" + strings.TrimPrefix(repo, "git@github.com:")
	}
	u, err := url.Parse(repo)
	if err != nil || u.Host != "github.com" || u.Scheme != "https" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("GitHub fallback is unavailable for this repository")
	}
	name := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	parts := strings.Split(name, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("invalid GitHub repository")
	}
	return name, nil
}

func updateGet(address string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "PrettyDamnFleet-Updater")
	res, err := updateHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update download: HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 64*1024*1024+1))
	if len(data) > 64*1024*1024 {
		return nil, fmt.Errorf("update artifact exceeds 64 MiB")
	}
	return data, err
}

func fetchGitHubRelease(repo, branch, stage string) (string, error) {
	name, err := githubRepository(repo)
	if err != nil {
		return "", err
	}
	data, err := updateGet(githubAPI + "/repos/" + name + "/commits/" + url.PathEscape(branch) + fmt.Sprintf("?fleet_update=%d", time.Now().UnixNano()))
	if err != nil {
		return "", err
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(data, &commit); err != nil {
		return "", err
	}
	decoded, err := hex.DecodeString(commit.SHA)
	if err != nil || len(decoded) != 20 {
		return "", fmt.Errorf("GitHub returned an invalid commit")
	}
	// Download all artifacts from the same immutable commit, never raw/main.
	for _, file := range releaseFiles {
		contents, err := updateGet(githubRaw + "/" + name + "/" + commit.SHA + "/" + file)
		if err != nil {
			return "", fmt.Errorf("%s: %w", file, err)
		}
		if err := writeUpdateFile(filepath.Join(stage, filepath.Base(file)), contents); err != nil {
			return "", err
		}
	}
	return commit.SHA, nil
}

func writeUpdateFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}

func validateUpdateBinary(path string) error {
	f, err := pe.Open(path)
	if err != nil {
		return fmt.Errorf("download is not a Windows executable: %w", err)
	}
	defer f.Close()
	want := uint16(pe.IMAGE_FILE_MACHINE_AMD64)
	if runtime.GOARCH == "386" {
		want = pe.IMAGE_FILE_MACHINE_I386
	}
	if runtime.GOARCH == "arm64" {
		want = pe.IMAGE_FILE_MACHINE_ARM64
	}
	if f.Machine != want || f.Characteristics&pe.IMAGE_FILE_EXECUTABLE_IMAGE == 0 || f.Characteristics&pe.IMAGE_FILE_DLL != 0 || f.OptionalHeader == nil {
		return fmt.Errorf("release binary is not an executable for %s", runtime.GOARCH)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	for _, section := range f.Sections {
		if uint64(section.Offset)+uint64(section.Size) > uint64(info.Size()) {
			return fmt.Errorf("release executable is truncated")
		}
	}
	return nil
}

func selfUpdate(jobID, repo, branch string) JobResult {
	updateLock.Lock()
	defer updateLock.Unlock()
	if restartRequested {
		return driveError(jobID, fmt.Errorf("an update is already staged for restart"))
	}
	if repo == "" {
		repo = agentCfg.Repo
	}
	if repo == "" {
		repo = defaultRepo
	}
	if branch == "" {
		branch = agentCfg.Branch
	}
	if branch == "" {
		branch = defaultBranch
	}
	stage, err := os.MkdirTemp(dataDir, "update-")
	if err != nil {
		return driveError(jobID, err)
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(stage)
		}
	}() // Exact MkdirTemp child of dataDir.
	if background {
		if err := ensureSessionPrivileges(); err != nil {
			return driveError(jobID, err)
		}
		sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
		if err != nil {
			return driveError(jobID, err)
		}
		acl, _, err := sd.DACL()
		if err != nil {
			return driveError(jobID, err)
		}
		if err := windows.SetNamedSecurityInfo(stage, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
			return driveError(jobID, err)
		}
	}
	revision, gitErr := fetchGitRelease(repo, branch, stage)
	via := "git fetch"
	if gitErr != nil {
		revision, err = fetchGitHubRelease(repo, branch, stage)
		via = "GitHub commit download"
		if err != nil {
			return driveError(jobID, fmt.Errorf("update failed; git: %v; fallback: %v", gitErr, err))
		}
	}
	binary := filepath.Join(stage, "fleet-agent.exe")
	if err := validateUpdateBinary(binary); err != nil {
		return driveError(jobID, err)
	}
	contents, err := os.ReadFile(binary)
	if err != nil {
		return driveError(jobID, err)
	}
	hash := sha256.Sum256(contents)
	request := updateRequest{jobID, os.Getpid(), dataDir, revision, hex.EncodeToString(hash[:]), repo}
	raw, _ := json.Marshal(request)
	if err := writeUpdateFile(filepath.Join(stage, "request.json"), raw); err != nil {
		return driveError(jobID, err)
	}
	if err := writeUpdateFile(filepath.Join(stage, "apply.ps1"), []byte(applyUpdateScript)); err != nil {
		return driveError(jobID, err)
	}
	stagedBinary := ""
	if !background {
		dest := filepath.Join(dataDir, "fleet-agent.exe")
		if exePath != "" {
			dest = exePath
		}
		stagedBinary = dest + ".new"
		if err := copyFile(binary, stagedBinary); err != nil {
			return driveError(jobID, err)
		}
	} else {
		// An independent one-shot SYSTEM task survives stopping the resident
		// agent's supervisor and reinstalls executable, startup, and tray together.
		if err := armUpdateTask(stage); err != nil {
			return driveError(jobID, err)
		}
	}
	// Only persist a source after all staging/scheduling steps succeeded.
	next := agentCfg
	next.Repo, next.Branch = repo, branch
	if err := saveConfig(next); err != nil {
		if stagedBinary != "" {
			_ = os.Remove(stagedBinary)
		}
		return driveError(jobID, fmt.Errorf("save update source: %w", err))
	}
	agentCfg = next
	keep = true
	restartRequested = true
	return JobResult{JobID: jobID, Status: "ok", Via: via, Message: "Staged commit " + revision + "; restarting to apply update", Output: "Binary SHA256: " + request.SHA256}
}

func armUpdateTask(stage string) error {
	code := `$ErrorActionPreference='Stop'; $stage=$env:FLEET_UPDATE_STAGE; $action=New-ScheduledTaskAction -Execute ($env:SystemRoot+'\System32\WindowsPowerShell\v1.0\powershell.exe') -Argument ('-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$stage+'\apply.ps1"'); $principal=New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest; $settings=New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit (New-TimeSpan -Minutes 15); Register-ScheduledTask -TaskName 'Fleet Console Update' -Action $action -Principal $principal -Settings $settings -Force | Out-Null`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", code)
	cmd.Env = append(os.Environ(), "FLEET_UPDATE_STAGE="+stage)
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("schedule update: %w; %s", err, out)
	}
	return nil
}

const applyUpdateScript = `$ErrorActionPreference = 'Stop'
function Get-UpdateHash($path) {
  $stream=[IO.File]::OpenRead($path)
  $sha=[Security.Cryptography.SHA256]::Create()
  try { return ([BitConverter]::ToString($sha.ComputeHash($stream))).Replace('-','').ToLowerInvariant() }
  finally { $stream.Dispose(); $sha.Dispose() }
}
$request = Get-Content -Raw -LiteralPath (Join-Path $PSScriptRoot 'request.json') | ConvertFrom-Json
$receipt = Join-Path $request.root 'update-result.json'
$result = @{jobId=$request.jobId; status='error'; via='update installer'; message='Update did not complete'}
try {
  try { Start-Transcript -Path (Join-Path $request.root 'update.log') -Append -Force | Out-Null } catch {}
  $deadline = (Get-Date).AddMinutes(3)
  while (Get-Process -Id $request.pid -ErrorAction SilentlyContinue) {
    if ((Get-Date) -gt $deadline) { throw 'Agent did not exit for update' }
    Start-Sleep -Milliseconds 250
  }
  $binary = Join-Path $PSScriptRoot 'fleet-agent.exe'
  if ((Get-UpdateHash $binary) -ne $request.sha256) { throw 'Staged binary checksum changed' }
  $oldBinary=Join-Path $request.root 'fleet-agent.exe'
  $backup=Join-Path $PSScriptRoot 'previous-agent.exe'
  if (Test-Path -LiteralPath $oldBinary) { Copy-Item -LiteralPath $oldBinary -Destination $backup }
  $installer = Join-Path $PSScriptRoot 'install.ps1'
  $argsForInstall = @('-NoPause', '-Repo', $request.repo)
  $scriptTokens=$null; $scriptErrors=$null
  $ast=[Management.Automation.Language.Parser]::ParseFile($installer,[ref]$scriptTokens,[ref]$scriptErrors)
  if ($scriptErrors.Count) { throw 'Staged installer has syntax errors' }
  $parameters=@($ast.ParamBlock.Parameters | ForEach-Object {$_.Name.VariablePath.UserPath})
  if ($parameters -contains 'DataDir') { $argsForInstall += @('-DataDir', $request.root) }
  elseif ($request.root -ne (Join-Path $env:ProgramData 'FleetConsole')) { throw 'Release installer does not support this data directory' }
  if ($parameters -contains 'SkipRepoSync') { $argsForInstall += '-SkipRepoSync' }
  # Feed the exact saved pairing through the child environment, avoiding stale
  # machine environment values and exposing no token in the command line.
  $pairing=Get-Content -Raw -LiteralPath (Join-Path $request.root 'config.json') | ConvertFrom-Json
  $env:FLEET_SERVER=[string]$pairing.server
  $env:FLEET_TOKEN=[string]$pairing.token
  & ($env:SystemRoot+'\System32\WindowsPowerShell\v1.0\powershell.exe') -NoProfile -NonInteractive -ExecutionPolicy Bypass -File $installer @argsForInstall
  if ($LASTEXITCODE -ne 0) { throw ('Installer exited with code '+$LASTEXITCODE) }
  if ((Get-UpdateHash (Join-Path $request.root 'fleet-agent.exe')) -ne $request.sha256) { throw 'Installed binary does not match the fetched commit' }
  $result.status='ok'
  $result.message='Installed commit '+$request.revision+'; startup and tray refreshed'
  $result.output='Binary SHA256: '+$request.sha256
} catch {
  $result.message='Update failed: '+$_.Exception.Message
  if ($backup -and (Test-Path -LiteralPath $backup)) {
    Stop-ScheduledTask -TaskName 'Fleet Console Agent' -ErrorAction SilentlyContinue
    Get-CimInstance Win32_Process -Filter "Name='fleet-agent.exe'" | Where-Object { $_.ExecutablePath -eq $oldBinary } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
    Copy-Item -LiteralPath $backup -Destination $oldBinary -Force
  }
  Start-ScheduledTask -TaskName 'Fleet Console Agent' -ErrorAction SilentlyContinue
} finally {
  try { Stop-Transcript | Out-Null } catch {}
  $result | ConvertTo-Json -Compress | Set-Content -LiteralPath ($receipt+'.tmp') -Encoding UTF8
  Move-Item -LiteralPath ($receipt+'.tmp') -Destination $receipt -Force
}
`
