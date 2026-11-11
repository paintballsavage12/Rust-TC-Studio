package updates

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ServiceConfig is signed by the existing OWNER_UPDATE_SIGNING_KEY so the
// public GitHub repository can distribute a new auth backend URL securely.
type ServiceConfig struct {
	URL       string `json:"url"`
	Signature string `json:"signature"`
}

func (c ServiceConfig) message() []byte {
	return []byte("RustTCStudio-auth-service-v1\n" + c.URL + "\n")
}
func SignServiceConfig(seed []byte, serverURL string) (ServiceConfig, error) {
	if len(seed) != ed25519.SeedSize {
		return ServiceConfig{}, errors.New("signer key must contain 32 bytes")
	}
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if err := validateAuthBase(serverURL); err != nil {
		return ServiceConfig{}, err
	}
	c := ServiceConfig{URL: serverURL}
	c.Signature = encoding.EncodeToString(ed25519.Sign(ed25519.NewKeyFromSeed(seed), c.message()))
	return c, nil
}
func validateAuthBase(s string) error {
	if err := HTTPS(s); err != nil {
		return err
	}
	if !strings.HasPrefix(s, "https://") || strings.Contains(s[8:], "/") || strings.ContainsAny(s, "?#") {
		return errors.New("auth endpoint must be an HTTPS origin without a path")
	}
	return nil
}
func VerifyServiceConfig(c ServiceConfig) error {
	if err := validateAuthBase(c.URL); err != nil {
		return err
	}
	p, err := encoding.DecodeString(publicKeyEncoded)
	if err != nil || len(p) != ed25519.PublicKeySize {
		return errors.New("no update signing key")
	}
	sig, err := encoding.DecodeString(c.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize || !ed25519.Verify(ed25519.PublicKey(p), c.message(), sig) {
		return errors.New("service config signature rejected")
	}
	return nil
}
func CheckServiceConfig(client *http.Client, feedURL string) (ServiceConfig, error) {
	if err := HTTPS(feedURL); err != nil {
		return ServiceConfig{}, err
	}
	r, err := client.Get(feedURL)
	if err != nil {
		return ServiceConfig{}, err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return ServiceConfig{}, fmt.Errorf("auth config HTTP %d", r.StatusCode)
	}
	d := json.NewDecoder(io.LimitReader(r.Body, 4097))
	d.DisallowUnknownFields()
	var c ServiceConfig
	if err = d.Decode(&c); err != nil {
		return c, err
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return ServiceConfig{}, errors.New("auth config too large or malformed")
	}
	return c, VerifyServiceConfig(c)
}
