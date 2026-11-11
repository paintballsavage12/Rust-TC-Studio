// Package updates implements publisher-signed, HTTPS-only update manifests.
// Player executables contain the public key, never the update signing seed.
package updates

import (
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
	"regexp"
	"strconv"
	"strings"
	"time"
)

const MaxFileSize int64 = 50 << 20
const MaxManifestSize int64 = 16 << 10

var versionRe = regexp.MustCompile(`^[0-9]{1,5}\.[0-9]{1,5}\.[0-9]{1,5}$`)
var hashRe = regexp.MustCompile(`^[a-f0-9]{64}$`)
var encoding = base64.RawURLEncoding

type Manifest struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
}

func HTTPS(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || len(raw) > 2048 {
		return errors.New("an HTTPS URL is required")
	}
	return nil
}
func (m Manifest) Validate() error {
	if !versionRe.MatchString(m.Version) {
		return errors.New("invalid semantic version")
	}
	if err := HTTPS(m.URL); err != nil {
		return err
	}
	if m.Size < 1024 || m.Size > MaxFileSize {
		return errors.New("update size is invalid")
	}
	if !hashRe.MatchString(m.SHA256) {
		return errors.New("SHA-256 must be 64 lowercase hex characters")
	}
	return nil
}
func (m Manifest) message() []byte {
	return []byte("RustTCStudio-update-v1\n" + m.Version + "\n" + m.URL + "\n" + strconv.FormatInt(m.Size, 10) + "\n" + m.SHA256 + "\n")
}
func Sign(seed []byte, version, assetURL string, binary []byte) (Manifest, error) {
	if len(seed) != ed25519.SeedSize {
		return Manifest{}, errors.New("update signing seed must be 32 bytes")
	}
	hash := sha256.Sum256(binary)
	m := Manifest{Version: version, URL: assetURL, Size: int64(len(binary)), SHA256: hex.EncodeToString(hash[:])}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	m.Signature = encoding.EncodeToString(ed25519.Sign(ed25519.NewKeyFromSeed(seed), m.message()))
	return m, nil
}
func VerifyWithKey(m Manifest, pub ed25519.PublicKey) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if len(pub) != ed25519.PublicKeySize {
		return errors.New("invalid update public key")
	}
	sig, err := encoding.DecodeString(m.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize || !ed25519.Verify(pub, m.message(), sig) {
		return errors.New("update signature invalid")
	}
	return nil
}
func Verify(m Manifest) error {
	pub, err := encoding.DecodeString(publicKeyEncoded)
	if err != nil {
		return errors.New("update verifier not configured")
	}
	return VerifyWithKey(m, pub)
}
func ReadSeed(data []byte) ([]byte, error) {
	seed, err := encoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("invalid update signing key")
	}
	return seed, nil
}
func CompareVersion(a, b string) (int, error) {
	if !versionRe.MatchString(a) || !versionRe.MatchString(b) {
		return 0, errors.New("invalid version format")
	}
	var x, y [3]int
	fmt.Sscanf(a, "%d.%d.%d", &x[0], &x[1], &x[2])
	fmt.Sscanf(b, "%d.%d.%d", &y[0], &y[1], &y[2])
	for i := 0; i < 3; i++ {
		if x[i] > y[i] {
			return 1, nil
		}
		if x[i] < y[i] {
			return -1, nil
		}
	}
	return 0, nil
}
func Client(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return HTTPS(req.URL.String())
	}}
}
func Check(client *http.Client, feedURL string) (Manifest, error) {
	if err := HTTPS(feedURL); err != nil {
		return Manifest{}, err
	}
	resp, err := client.Get(feedURL)
	if err != nil {
		return Manifest{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Manifest{}, fmt.Errorf("feed HTTP %d", resp.StatusCode)
	}
	var m Manifest
	dec := json.NewDecoder(io.LimitReader(resp.Body, MaxManifestSize+1))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("invalid update feed: %w", err)
	}
	// Enforce body size even if the JSON document is valid before the limit.
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return Manifest{}, errors.New("unexpected trailing content in feed")
	}
	if err := Verify(m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}
func Download(client *http.Client, m Manifest, path string) error {
	if err := Verify(m); err != nil {
		return err
	}
	resp, err := client.Get(m.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > MaxFileSize || (resp.ContentLength > 0 && resp.ContentLength != m.Size) {
		return errors.New("download size mismatch")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(path)
		}
	}()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, m.Size+1))
	if err != nil {
		return err
	}
	if n != m.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), m.SHA256) {
		return errors.New("download rejected: size or SHA-256 mismatch")
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}
