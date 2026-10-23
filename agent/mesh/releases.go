package mesh

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const MaxReleaseSize = 128 * 1024 * 1024

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ReleaseManifest struct {
	Version     string `json:"version"`
	Sequence    uint64 `json:"sequence"`
	Platform    string `json:"platform"`
	ProtocolMin int    `json:"protocolMin"`
	ProtocolMax int    `json:"protocolMax"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	URL         string `json:"url"`
	KeyID       string `json:"keyId"`
	Signature   []byte `json:"signature"`
}

func (m ReleaseManifest) SigningBytes() []byte { m.Signature = nil; return encode(m) }
func (m ReleaseManifest) ID() string           { return digest(encode(m)) }
func FileHash(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type ReleaseCache struct {
	Dir  string
	Keys map[string]ed25519.PublicKey
	mu   sync.Mutex
}

func OpenReleaseCache(dataDir string) (*ReleaseCache, error) {
	c := &ReleaseCache{Dir: filepath.Join(dataDir, "releases"), Keys: map[string]ed25519.PublicKey{}}
	if err := os.MkdirAll(c.Dir, 0700); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(dataDir, "release-keys.json"))
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	var keys []string
	if json.Unmarshal(b, &keys) != nil {
		return nil, errors.New("invalid release-keys.json")
	}
	for _, key := range keys {
		pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(key))
		if err != nil || len(pub) != 32 {
			return nil, errors.New("invalid release public key")
		}
		c.Keys[digest(pub)] = ed25519.PublicKey(pub)
	}
	return c, nil
}
func (c *ReleaseCache) Verify(m ReleaseManifest) error {
	pub := c.Keys[m.KeyID]
	if len(pub) != ed25519.PublicKeySize || !ed25519.Verify(pub, m.SigningBytes(), m.Signature) {
		return errors.New("release is not signed by an installed publisher key")
	}
	if m.Platform != "windows/amd64" || m.ProtocolMin > Protocol || m.ProtocolMax < Protocol || m.ProtocolMin < 1 || m.Size < 1024 || m.Size > MaxReleaseSize || !hashPattern.MatchString(m.SHA256) || m.Sequence == 0 || m.Version == "" {
		return errors.New("incompatible or invalid release manifest")
	}
	u, err := url.Parse(m.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return errors.New("release binary URL must be HTTPS")
	}
	return nil
}
func (c *ReleaseCache) Manifest(id string) (ReleaseManifest, error) {
	if !hashPattern.MatchString(id) {
		return ReleaseManifest{}, errors.New("release hash required")
	}
	b, err := os.ReadFile(filepath.Join(c.Dir, id+".json"))
	if err != nil {
		return ReleaseManifest{}, err
	}
	m, err := decode[ReleaseManifest](b)
	if err != nil {
		return m, err
	}
	if m.ID() != id {
		return m, errors.New("cached manifest hash mismatch")
	}
	return m, c.Verify(m)
}
func (c *ReleaseCache) Binary(id string) (string, error) {
	m, err := c.Manifest(id)
	if err != nil {
		return "", err
	}
	file := filepath.Join(c.Dir, m.SHA256+".exe")
	info, err := os.Stat(file)
	if err != nil {
		return "", err
	}
	if info.Size() != m.Size {
		return "", errors.New("cached binary size mismatch")
	}
	hash, err := FileHash(file)
	if err != nil || hash != m.SHA256 {
		return "", errors.New("cached binary checksum mismatch")
	}
	return file, nil
}

type ReleaseFetcher func(context.Context, string) (io.ReadCloser, error)

func fetchHTTPS(ctx context.Context, raw string) (io.ReadCloser, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, errors.New("HTTPS release URL required")
	}
	client := &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || req.URL.Scheme != "https" {
			return errors.New("invalid release redirect")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		return nil, fmt.Errorf("release HTTP %d", res.StatusCode)
	}
	return res.Body, nil
}
func (c *ReleaseCache) Fetch(ctx context.Context, reference string, peerFetch ReleaseFetcher) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.Keys) == 0 {
		return "", errors.New("install a publisher public key before updating")
	}
	if hashPattern.MatchString(reference) {
		if _, err := c.Binary(reference); err == nil {
			return reference, nil
		}
	}
	var raw []byte
	var err error
	if hashPattern.MatchString(reference) {
		if peerFetch == nil {
			return "", errors.New("release is not cached locally")
		}
		body, err := peerFetch(ctx, reference+"/manifest")
		if err != nil {
			return "", err
		}
		raw, err = io.ReadAll(io.LimitReader(body, 65537))
		body.Close()
		if err != nil {
			return "", err
		}
	} else {
		body, err := fetchHTTPS(ctx, reference)
		if err != nil {
			return "", err
		}
		raw, err = io.ReadAll(io.LimitReader(body, 65537))
		body.Close()
		if err != nil {
			return "", err
		}
	}
	if len(raw) > 65536 {
		return "", errors.New("release manifest exceeds limit")
	}
	m, err := decode[ReleaseManifest](raw)
	if err != nil {
		return "", err
	}
	if err = c.Verify(m); err != nil {
		return "", err
	}
	id := m.ID()
	if hashPattern.MatchString(reference) && id != reference {
		return "", errors.New("release manifest identity mismatch")
	}
	if _, err = c.Binary(id); err == nil {
		return id, nil
	}
	var binary io.ReadCloser
	if peerFetch != nil {
		binary, err = peerFetch(ctx, id+"/binary")
	}
	if binary == nil {
		binary, err = fetchHTTPS(ctx, m.URL)
	}
	if err != nil {
		return "", err
	}
	defer binary.Close()
	f, err := os.CreateTemp(c.Dir, "download-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	h := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(binary, m.Size+1))
	if copyErr == nil {
		copyErr = f.Sync()
	}
	closeErr := f.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if size != m.Size || hex.EncodeToString(h.Sum(nil)) != m.SHA256 {
		return "", errors.New("release checksum or size mismatch")
	}
	if err = os.Rename(f.Name(), filepath.Join(c.Dir, m.SHA256+".exe")); err != nil {
		return "", err
	}
	if err = atomicFile(filepath.Join(c.Dir, id+".json"), encode(m)); err != nil {
		return "", err
	}
	return id, nil
}

func (n *Node) FetchRelease(ctx context.Context, reference string) (string, error) {
	if n.Releases == nil {
		return "", errors.New("release cache unavailable")
	}
	return n.Releases.Fetch(ctx, reference, func(ctx context.Context, suffix string) (io.ReadCloser, error) {
		members, err := n.Store.Members()
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			if m.ID == n.Self.ID || m.Removed {
				continue
			}
			client := n.client(m.ID)
			req, err := http.NewRequestWithContext(ctx, "GET", m.PeerURL+"/peer/v2/releases/"+suffix, nil)
			if err != nil {
				continue
			}
			res, err := client.Do(req)
			if err == nil && res.StatusCode == 200 {
				return res.Body, nil
			}
			if res != nil {
				res.Body.Close()
			}
			client.CloseIdleConnections()
		}
		return nil, errors.New("no peer has this release")
	})
}
func (n *Node) serveRelease(w http.ResponseWriter, r *http.Request) {
	if n.Releases == nil || r.Method != "GET" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/peer/v2/releases/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	if parts[1] == "manifest" {
		m, err := n.Releases.Manifest(parts[0])
		if err != nil {
			failHTTP(w, 404, err)
			return
		}
		writeJSON(w, 200, m)
		return
	}
	if parts[1] == "binary" {
		file, err := n.Releases.Binary(parts[0])
		if err != nil {
			failHTTP(w, 404, err)
			return
		}
		http.ServeFile(w, r, file)
		return
	}
	http.NotFound(w, r)
}
