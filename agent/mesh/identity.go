package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/nacl/box"
)

type Identity struct {
	Signing    ed25519.PrivateKey `json:"signing"`
	BoxPrivate *[32]byte          `json:"boxPrivate"`
	BoxPublic  *[32]byte          `json:"boxPublic"`
}

func newIdentity() (*Identity, error) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	pub, secret, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Identity{key, secret, pub}, nil
}
func (i *Identity) public() ed25519.PublicKey { return i.Signing.Public().(ed25519.PublicKey) }
func (i *Identity) ID() string                { return digest(i.public()) }
func (i *Identity) Member(host, peer, dashboard string) Member {
	return Member{ID: i.ID(), PublicKey: i.public(), EncryptionKey: i.BoxPublic[:], Hostname: host, PeerURL: peer, DashboardURL: dashboard}
}

func loadIdentity(dir string, create bool) (*Identity, error) {
	file := filepath.Join(dir, "identity.dat")
	b, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) && create {
		i, err := newIdentity()
		if err != nil {
			return nil, err
		}
		b, err = protect(encode(i))
		if err != nil {
			return nil, err
		}
		if err = atomicFile(file, b); err != nil {
			return nil, err
		}
		return i, nil
	}
	if err != nil {
		return nil, fmt.Errorf("identity unavailable; restore or explicitly re-enroll: %w", err)
	}
	b, err = unprotect(b)
	if err != nil {
		return nil, fmt.Errorf("cannot decrypt identity: %w", err)
	}
	var i Identity
	if json.Unmarshal(b, &i) != nil || len(i.Signing) != ed25519.PrivateKeySize || i.BoxPrivate == nil || i.BoxPublic == nil {
		return nil, errors.New("invalid saved identity; left unchanged")
	}
	return &i, nil
}

func atomicFile(file string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(file), ".fleet-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), file)
}

func (i *Identity) certificate() (tls.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	t := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: i.ID()}, NotBefore: time.Now().Add(-24 * time.Hour), NotAfter: time.Now().AddDate(10, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, t, t, i.public(), i.Signing)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: i.Signing}, err
}

func peerIdentity(raw [][]byte) (string, error) {
	if len(raw) != 1 {
		return "", errors.New("one node certificate required")
	}
	c, err := x509.ParseCertificate(raw[0])
	if err != nil {
		return "", err
	}
	p, ok := c.PublicKey.(ed25519.PublicKey)
	if !ok {
		return "", errors.New("node certificate must use Ed25519")
	}
	if err = c.CheckSignature(c.SignatureAlgorithm, c.RawTBSCertificate, c.Signature); err != nil {
		return "", err
	}
	return digest(p), nil
}

func (e Event) signingBytes() []byte { e.ID = ""; e.Signature = nil; return encode(e) }
func (i *Identity) sign(e *Event) {
	e.PublicKey = i.public()
	e.Origin = i.ID()
	e.ID = digest(e.signingBytes())
	e.Signature = ed25519.Sign(i.Signing, e.signingBytes())
}
func (e Event) verify(fleet string) error {
	if e.Version != Protocol || e.FleetID != fleet || e.Sequence < 1 || e.Lamport < 1 || len(e.PublicKey) != ed25519.PublicKeySize || e.Origin != digest(e.PublicKey) || e.ID != digest(e.signingBytes()) || !ed25519.Verify(e.PublicKey, e.signingBytes(), e.Signature) {
		return errors.New("invalid event identity, fleet, version, or signature")
	}
	if len(e.Body) > MaxBody/2 || len(e.Dependencies) > 64 {
		return errors.New("event exceeds limits")
	}
	return nil
}

func seal(public []byte, secret string) ([]byte, error) {
	if len(public) != 32 {
		return nil, errors.New("invalid target encryption key")
	}
	var key [32]byte
	copy(key[:], public)
	return box.SealAnonymous(nil, []byte(secret), &key, rand.Reader)
}
func (i *Identity) open(cipher []byte) (string, error) {
	if len(cipher) == 0 {
		return "", nil
	}
	plain, ok := box.OpenAnonymous(nil, cipher, i.BoxPublic, i.BoxPrivate)
	if !ok {
		return "", errors.New("cannot decrypt target credential")
	}
	return string(plain), nil
}
