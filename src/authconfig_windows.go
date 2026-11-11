//go:build windows

package main

import (
	"rusttcstudio/updates"
	"time"
)

var authConfigFeedURL = "https://raw.githubusercontent.com/paintballsavage12/Rust-TC-Studio/main/auth.json"

type authConfigEvent struct {
	config updates.ServiceConfig
	err    error
}

var authConfigEvents = make(chan authConfigEvent, 2)
var authConfigChecking bool
var lastAuthConfigCheck time.Time

func checkRemoteAuthConfig() {
	if a.authLocalPinned || authConfigChecking || time.Since(lastAuthConfigCheck) < 6*time.Hour {
		return
	}
	lastAuthConfigCheck = time.Now()
	authConfigChecking = true
	go func() {
		config, err := updates.CheckServiceConfig(updates.Client(12*time.Second), authConfigFeedURL)
		authConfigEvents <- authConfigEvent{config, err}
	}()
}
func pollAuthConfigEvents() {
	select {
	case ev := <-authConfigEvents:
		authConfigChecking = false
		if ev.err == nil && !a.authLocalPinned && a.cfg.AuthURL != ev.config.URL {
			a.cfg.AuthURL = ev.config.URL
			saveConfig()
			addLog("Verified remote PIN server configuration")
			if !a.authed && !a.authPending {
				_ = restoreRememberedCredential()
			}
		}
	default:
	}
	checkRemoteAuthConfig()
}
