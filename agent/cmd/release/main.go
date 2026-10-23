// Offline publisher utility. Never copy the private key to a fleet node.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"debug/pe"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/cursor/fleet-agent/mesh"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	newKey := flag.String("new-key", "", "Create a publisher private-key file (exclusive)")
	keyFile := flag.String("key", "", "Publisher private-key file")
	binary := flag.String("binary", "", "Windows x64 node executable")
	version := flag.String("version", "", "Release version")
	sequence := flag.Uint64("sequence", 0, "Strictly increasing release number")
	url := flag.String("url", "", "HTTPS URL of the exact executable")
	out := flag.String("out", "", "Manifest output path")
	flag.Parse()
	if *newKey != "" {
		pub, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(*newKey, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		if _, err = f.WriteString(base64.StdEncoding.EncodeToString(key)); err != nil {
			f.Close()
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
		fmt.Println(base64.StdEncoding.EncodeToString(pub))
		return nil
	}
	b, err := os.ReadFile(*keyFile)
	if err != nil {
		return err
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid publisher private key")
	}
	image, err := pe.Open(*binary)
	if err != nil {
		return err
	}
	defer image.Close()
	if image.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		return fmt.Errorf("Windows x64 executable required")
	}
	info, err := os.Stat(*binary)
	if err != nil {
		return err
	}
	hash, err := mesh.FileHash(*binary)
	if err != nil {
		return err
	}
	pub := ed25519.PrivateKey(key).Public().(ed25519.PublicKey)
	keyHash := sha256.Sum256(pub)
	m := mesh.ReleaseManifest{Version: *version, Sequence: *sequence, Platform: "windows/amd64", ProtocolMin: mesh.Protocol, ProtocolMax: mesh.Protocol, SHA256: hash, Size: info.Size(), URL: *url, KeyID: hex.EncodeToString(keyHash[:])}
	m.Signature = ed25519.Sign(ed25519.PrivateKey(key), m.SigningBytes())
	cache := mesh.ReleaseCache{Keys: map[string]ed25519.PublicKey{m.KeyID: pub}}
	if err = cache.Verify(m); err != nil {
		return err
	}
	b, err = json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(*out, b, 0644); err != nil {
		return err
	}
	fmt.Println(m.ID())
	return nil
}
