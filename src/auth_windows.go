//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"rusttcstudio/authclient"
	"rusttcstudio/license"
	"strings"
	"time"
)

type authEvent struct {
	gen     int64
	kind    string
	session authclient.Session
	issued  authclient.Issue
	err     error
}

var authEvents = make(chan authEvent, 8)

func onlyDigits(s string) bool {
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return s != ""
}
func startAuthClient() (authclient.Client, error) { return authclient.New(a.cfg.AuthURL) }
func beginPINLogin(pin string) {
	c, err := startAuthClient()
	if err != nil {
		a.authNotice = "Online PIN server is not configured. Signed access codes still work."
		toast(a.authNotice)
		return
	}
	a.authGen++
	gen := a.authGen
	a.authPending = true
	a.authNotice = "Checking 6-digit PIN securely..."
	keep := a.rememberLogin
	refreshUI()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		s, err := c.Login(ctx, pin, keep)
		authEvents <- authEvent{gen: gen, kind: "login", session: s, err: err}
	}()
}
func restorePINSession(token string) {
	c, err := startAuthClient()
	if err != nil {
		a.authNotice = "Saved PIN session needs a configured auth server"
		return
	}
	a.authGen++
	gen := a.authGen
	a.authPending = true
	a.authNotice = "Restoring your saved online session..."
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		s, err := c.Restore(ctx, token)
		s.Token = token
		authEvents <- authEvent{gen: gen, kind: "restore", session: s, err: err}
	}()
}
func privateOwnerAPIKey() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "OWNER_AUTH_TOKEN.txt"))
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(b))
	if len(v) < 32 {
		return "", fmt.Errorf("owner API token too short")
	}
	return v, nil
}
func issueOnlinePIN() {
	if !a.authed || !a.ownerLocal || a.adminPINPending {
		toast("Online player PINs require your private owner installation.")
		return
	}
	name := strings.TrimSpace(editValue(a.adminNameEdit))
	if !license.ValidUsername(name) {
		toast("Enter a valid username (2 to 24 characters).")
		return
	}
	c, err := startAuthClient()
	if err != nil {
		a.adminMessage = "6-digit PINs require a live HTTPS PIN server. Configure signed auth.json first; use Legacy Signed Player Code meanwhile."
		addLog("6-digit PIN unavailable: auth server not configured")
		refreshUI()
		return
	}
	key, err := privateOwnerAPIKey()
	if err != nil {
		toast("OWNER_AUTH_TOKEN.txt missing beside owner EXE.")
		return
	}
	a.adminPINPending = true
	a.adminMessage = "Generating secure PIN..."
	gen := a.authGen
	refreshUI()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		v, err := c.Issue(ctx, name, key)
		authEvents <- authEvent{gen: gen, kind: "issue", issued: v, err: err}
	}()
}
func verifyOnlineSessionInBackground() {
	if !a.authed || !a.onlinePIN || a.activeSessionToken == "" || a.sessionChecking || time.Since(a.lastSessionCheck) < 10*time.Minute {
		return
	}
	c, err := startAuthClient()
	if err != nil {
		return
	}
	a.sessionChecking = true
	a.lastSessionCheck = time.Now()
	gen := a.authGen
	token := a.activeSessionToken
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		v, err := c.Restore(ctx, token)
		authEvents <- authEvent{gen: gen, kind: "heartbeat", session: v, err: err}
	}()
}
func revokeOnlinePIN() {
	if !a.authed || !a.ownerLocal || a.adminPINPending {
		toast("Online player management requires your private owner installation.")
		return
	}
	name := strings.TrimSpace(editValue(a.adminNameEdit))
	if !license.ValidUsername(name) {
		toast("Enter the username you want to revoke.")
		return
	}
	c, err := startAuthClient()
	if err != nil {
		toast("PIN service is not configured.")
		return
	}
	key, err := privateOwnerAPIKey()
	if err != nil {
		toast("Private owner API token is missing.")
		return
	}
	if showMessage("Revoke the online PIN for "+name+"?\n\nThis also disables their saved sessions.", "CONFIRM PLAYER REVOCATION", MB_YESNO|MB_ICONWARNING) != IDYES {
		return
	}
	a.adminPINPending = true
	a.adminMessage = "Revoking player online access..."
	gen := a.authGen
	refreshUI()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		err := c.Revoke(ctx, name, key)
		authEvents <- authEvent{gen: gen, kind: "revoke", issued: authclient.Issue{Username: name}, err: err}
	}()
}
func pollAuthEvents() {
	for {
		select {
		case ev := <-authEvents:
			if ev.gen != a.authGen {
				continue
			}
			if ev.kind == "heartbeat" {
				a.sessionChecking = false
				if ev.err != nil && strings.Contains(ev.err.Error(), "HTTP 401") {
					signOut()
					toast("Your player PIN was revoked or your session expired.")
				}
				continue
			}
			if ev.kind == "revoke" {
				a.adminPINPending = false
				if ev.err != nil {
					a.adminMessage = "Revoke failed: " + ev.err.Error()
				} else {
					a.adminMessage = "Revoked online PIN and saved sessions for " + ev.issued.Username
					addLog("Online PIN revoked for " + ev.issued.Username)
				}
				refreshUI()
				continue
			}
			if ev.kind == "issue" {
				a.adminPINPending = false
				if ev.err != nil {
					a.adminMessage = "PIN generation failed: " + ev.err.Error()
					addLog("Online PIN issuance failed")
				} else {
					a.adminIssuedCode = ev.issued.PIN
					a.adminMessage = "Six-digit player PIN issued for " + ev.issued.Username
					if copyClipboard(ev.issued.PIN) {
						a.adminMessage += "; copied to clipboard"
					}
					addLog("Online PIN issued for " + ev.issued.Username)
				}
				refreshUI()
				continue
			}
			a.authPending = false
			if ev.err != nil {
				a.authNotice = "Login failed: " + ev.err.Error()
				if ev.kind == "restore" {
					clearRememberedCredential()
				}
				toast(a.authNotice)
				continue
			}
			a.onlinePIN = true
			a.activeSessionToken = ev.session.Token
			a.lastSessionCheck = time.Now()
			a.ownerLocal = false
			a.isAdmin = false
			a.licenseID = "ONLINE-PIN"
			a.cfg.User = ev.session.Username
			a.authed = true
			a.page = "dashboard"
			a.cfg.Salt, a.cfg.Hash = "", ""
			saveConfig()
			if a.rememberLogin || ev.kind == "restore" {
				a.rememberLogin = true
				if err := rememberCredential("pin-session-v1:" + ev.session.Token); err != nil {
					a.authNotice = "Windows could not save this session"
				}
			} else {
				clearRememberedCredential()
			}
			setEdit(a.passEdit, "")
			showEdits()
			SetFocus.Call(a.hwnd)
			registerHotkeys()
			addLog("Online PIN login: " + a.cfg.User)
			refreshUI()
		default:
			return
		}
	}
}
