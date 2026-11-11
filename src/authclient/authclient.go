// Package authclient connects to the optional private PIN server over HTTPS.
package authclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	Base string
	HTTP *http.Client
}
type Session struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	Token    string `json:"token"`
	Expires  string `json:"expires"`
}
type Issue struct {
	Username string `json:"username"`
	PIN      string `json:"pin"`
	Role     string `json:"role"`
}

func HTTPS(base string) error {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || u.Path != "" || len(base) > 1024 {
		return errors.New("authentication server requires a clean HTTPS base URL")
	}
	return nil
}
func New(base string) (Client, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if err := HTTPS(base); err != nil {
		return Client{}, err
	}
	host, err := url.Parse(base)
	if err != nil {
		return Client{}, err
	}
	c := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if r.URL.Scheme != "https" || r.URL.Host != host.Host || len(via) > 2 {
			return errors.New("untrusted login redirect")
		}
		return nil
	}}
	return Client{Base: base, HTTP: c}, nil
}
func (c Client) call(ctx context.Context, path string, request any, adminSecret string, result any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.Base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if adminSecret != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(adminSecret))
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("login service HTTP %d", resp.StatusCode)
	}
	d := json.NewDecoder(io.LimitReader(resp.Body, 8192))
	if err = d.Decode(result); err != nil {
		return err
	}
	var junk any
	if err = d.Decode(&junk); err != io.EOF {
		return errors.New("invalid server response")
	}
	return nil
}
func (c Client) Login(ctx context.Context, pin string, remember bool) (Session, error) {
	if len(pin) != 6 {
		return Session{}, errors.New("player PIN must have six digits")
	}
	for _, v := range pin {
		if v < '0' || v > '9' {
			return Session{}, errors.New("PIN must be numeric")
		}
	}
	var s Session
	err := c.call(ctx, "/v1/login", map[string]any{"pin": pin, "remember": remember}, "", &s)
	if err != nil {
		return Session{}, err
	}
	if s.Role != "player" || len(s.Username) < 2 || len(s.Username) > 90 || len(s.Token) < 30 || len(s.Token) > 256 {
		return Session{}, errors.New("invalid login response")
	}
	return s, nil
}
func (c Client) Restore(ctx context.Context, token string) (Session, error) {
	var s Session
	if len(token) < 30 || len(token) > 256 {
		return s, errors.New("invalid session token")
	}
	err := c.call(ctx, "/v1/session", map[string]string{"token": token}, "", &s)
	if err != nil {
		return Session{}, err
	}
	if s.Role != "player" || len(s.Username) < 2 {
		return Session{}, errors.New("invalid saved session")
	}
	return s, nil
}
func (c Client) Issue(ctx context.Context, username, secret string) (Issue, error) {
	var i Issue
	if len(secret) < 32 {
		return i, errors.New("owner API token unavailable")
	}
	err := c.call(ctx, "/v1/admin/issue", map[string]string{"username": username}, secret, &i)
	if err != nil {
		return i, err
	}
	if i.Username != username || len(i.PIN) != 6 || i.Role != "player" {
		return i, errors.New("invalid PIN issued")
	}
	return i, nil
}
func (c Client) Revoke(ctx context.Context, username, secret string) error {
	var out map[string]any
	return c.call(ctx, "/v1/admin/revoke", map[string]string{"username": username}, secret, &out)
}
func (c Client) List(ctx context.Context, secret string) ([]map[string]any, error) {
	var out struct {
		Accounts []map[string]any `json:"accounts"`
	}
	err := c.call(ctx, "/v1/admin/list", map[string]any{}, secret, &out)
	return out.Accounts, err
}
