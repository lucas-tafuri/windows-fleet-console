package mesh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignedPeerReleaseCacheAndTampering(t *testing.T) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = os.WriteFile(filepath.Join(dir, "release-keys.json"), encode([]string{base64.StdEncoding.EncodeToString(pub)}), 0600); err != nil {
		t.Fatal(err)
	}
	cache, err := OpenReleaseCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	binary := strings.Repeat("signed-binary", 1000)
	m := ReleaseManifest{Version: "2.0.1", Sequence: 1, Platform: "windows/amd64", ProtocolMin: 2, ProtocolMax: 2, SHA256: digest([]byte(binary)), Size: int64(len(binary)), URL: "https://example.invalid/fleet-node.exe", KeyID: digest(pub)}
	m.Signature = ed25519.Sign(key, m.SigningBytes())
	fetch := func(ctx context.Context, path string) (io.ReadCloser, error) {
		if strings.HasSuffix(path, "/manifest") {
			return io.NopCloser(strings.NewReader(string(encode(m)))), nil
		}
		return io.NopCloser(strings.NewReader(binary)), nil
	}
	id, err := cache.Fetch(context.Background(), m.ID(), fetch)
	if err != nil {
		t.Fatal(err)
	}
	path, err := cache.Binary(id)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = cache.Binary(id); err == nil {
		t.Fatal("tampered binary accepted")
	}
	forged := m
	forged.Version = "modified"
	if err = cache.Verify(forged); err == nil {
		t.Fatal("tampered manifest accepted")
	}
	other := &ReleaseCache{Keys: map[string]ed25519.PublicKey{}}
	if err = other.Verify(m); err == nil {
		t.Fatal("unknown publisher accepted")
	}
	incompatible := m
	incompatible.ProtocolMin = 3
	incompatible.Signature = ed25519.Sign(key, incompatible.SigningBytes())
	if err = cache.Verify(incompatible); err == nil {
		t.Fatal("unsupported protocol accepted")
	}
}
