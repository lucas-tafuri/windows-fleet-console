//go:build windows

package main

import (
	"errors"
	"github.com/cursor/fleet-agent/mesh"
	"os"
	"path/filepath"
	"testing"
)

func TestMeshUpdateRollbackAfterFailedHealth(t *testing.T) {
	dir := t.TempDir()
	installed := filepath.Join(dir, "node.exe")
	staged := filepath.Join(dir, "new.exe")
	if err := os.WriteFile(installed, []byte("old binary"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("new binary"), 0600); err != nil {
		t.Fatal(err)
	}
	hash, _ := mesh.FileHash(staged)
	stops, starts := 0, 0
	rolled, err := meshSwapBinary(staged, installed, hash, func() error { stops++; return nil }, func() error { starts++; return nil }, func() error { return errors.New("new service unhealthy") })
	if err == nil || !rolled || stops != 2 || starts != 2 {
		t.Fatal(rolled, err, stops, starts)
	}
	b, _ := os.ReadFile(installed)
	if string(b) != "old binary" {
		t.Fatal("old binary not restored")
	}
}
func TestMeshUpdateDoesNotReplaceRunningBinary(t *testing.T) {
	dir := t.TempDir()
	installed := filepath.Join(dir, "node.exe")
	staged := filepath.Join(dir, "new.exe")
	_ = os.WriteFile(installed, []byte("old binary"), 0600)
	_ = os.WriteFile(staged, []byte("new binary"), 0600)
	hash, _ := mesh.FileHash(staged)
	_, err := meshSwapBinary(staged, installed, hash, func() error { return errors.New("cannot stop") }, func() error { t.Fatal("unexpected restart"); return nil }, func() error { t.Fatal("unexpected health"); return nil })
	if err == nil {
		t.Fatal("expected failure")
	}
	b, _ := os.ReadFile(installed)
	if string(b) != "old binary" {
		t.Fatal("modified running binary")
	}
}
func TestMeshUpdateChecksumFailureRollsBack(t *testing.T) {
	dir := t.TempDir()
	installed := filepath.Join(dir, "node.exe")
	staged := filepath.Join(dir, "new.exe")
	_ = os.WriteFile(installed, []byte("old binary"), 0600)
	_ = os.WriteFile(staged, []byte("tampered binary"), 0600)
	rolled, err := meshSwapBinary(staged, installed, "invalid hash", func() error { return nil }, func() error { return nil }, func() error { t.Fatal("invalid release reached health check"); return nil })
	if err == nil || !rolled {
		t.Fatal(rolled, err)
	}
	b, _ := os.ReadFile(installed)
	if string(b) != "old binary" {
		t.Fatal("old binary not restored")
	}
}
