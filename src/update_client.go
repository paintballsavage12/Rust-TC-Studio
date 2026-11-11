//go:build windows

package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"rusttcstudio/updates"
)

//go:embed updater_windows_amd64.exe
var embeddedUpdater []byte

// The signed manifest will be hosted in the owner's Rust-TC-Studio GitHub
// repository once it is published. The current updater reports a missing feed
// rather than treating an unpublished repo as a successful update check.
// Can be overridden at build time using -ldflags -X or in the Updates screen.
var defaultUpdateURL = "https://raw.githubusercontent.com/paintballsavage12/Rust-TC-Studio/main/update.json"

type updateResult struct {
	kind       string
	feedURL    string
	manifest   updates.Manifest
	stagedPath string
	err        error
}

var updateEvents = make(chan updateResult, 4)

func loadUpdateFeed() {
	if a.cfg.UpdateURL != "" {
		return
	}
	if err := updates.HTTPS(defaultUpdateURL); err == nil {
		a.cfg.UpdateURL = defaultUpdateURL
		saveConfig()
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "update_feed.txt"))
	if err != nil {
		return
	}
	feed := strings.TrimSpace(string(b))
	if updates.HTTPS(feed) == nil {
		a.cfg.UpdateURL = feed
		saveConfig()
	}
}
func checkForUpdate(manual bool) {
	if a.updateChecking || a.updateDownloading {
		return
	}
	feed := strings.TrimSpace(a.cfg.UpdateURL)
	if err := updates.HTTPS(feed); err != nil {
		a.updateStatus = "No update feed configured yet."
		a.updateError = "Enter the HTTPS release-feed link provided by the owner."
		if manual {
			toast("Set an HTTPS update feed URL first.")
		}
		refreshUI()
		return
	}
	a.updateChecking = true
	a.updateError = ""
	a.updateStatus = "Checking for signed releases..."
	a.lastUpdateCheck = time.Now()
	refreshUI()
	go func() {
		m, err := updates.Check(updates.Client(14*time.Second), feed)
		updateEvents <- updateResult{kind: "check", feedURL: feed, manifest: m, err: err}
	}()
}
func startDownloadUpdate() {
	if a.updateChecking || a.updateDownloading || !a.updateAvailable {
		return
	}
	m := a.updateManifest
	if updates.Verify(m) != nil {
		return
	}
	if showMessage("Install Rust TC Studio "+m.Version+" now?\n\nThe app will close and restart after installation. Rust crafting does NOT pause when this app closes.\n\nYour local settings and owner-issued username configuration are preserved. Saved login sessions are preserved if you enabled Keep Me Logged In.", "INSTALL VERIFIED UPDATE", MB_YESNO|MB_ICONQUESTION) != IDYES {
		return
	}
	stopStack()
	a.updateDownloading = true
	a.updateStatus = "Downloading and verifying new EXE..."
	a.updateError = ""
	refreshUI()
	go func() {
		exe, err := os.Executable()
		var stage string
		if err == nil {
			rnd := make([]byte, 8)
			if _, err = rand.Read(rnd); err == nil {
				stage = filepath.Join(filepath.Dir(exe), ".RustTCStudio-"+m.Version+"-"+hex.EncodeToString(rnd)+".new.exe")
				err = updates.Download(updates.Client(3*time.Minute), m, stage)
			}
		}
		updateEvents <- updateResult{kind: "download", stagedPath: stage, manifest: m, err: err}
	}()
}
func pollUpdateEvents() {
	for {
		select {
		case e := <-updateEvents:
			if e.kind == "check" {
				a.updateChecking = false
				if e.feedURL != a.cfg.UpdateURL {
					continue
				} // stale feed result
				if e.err != nil {
					a.updateStatus = "Update check failed"
					a.updateError = shortUpdateError(e.err)
					addLog("Update check failed")
				} else {
					n, err := updates.CompareVersion(e.manifest.Version, appVersion)
					if err != nil {
						a.updateStatus = "Invalid version in release"
						a.updateError = err.Error()
					} else if n > 0 {
						a.updateManifest = e.manifest
						a.updateAvailable = true
						a.updateStatus = "New signed release is available"
						a.updateError = ""
						addLog("New update found: v" + e.manifest.Version)
					} else {
						a.updateAvailable = false
						a.updateStatus = "You're up to date"
						a.updateError = ""
						addLog("No newer update available")
					}
				}
			} else if e.kind == "download" {
				a.updateDownloading = false
				if e.err != nil {
					a.updateStatus = "Download failed; no files were replaced"
					a.updateError = shortUpdateError(e.err)
					addLog("Update download rejected")
				} else if err := launchVerifiedUpdater(e.stagedPath, e.manifest.SHA256); err != nil {
					a.updateStatus = "Updater could not launch"
					a.updateError = shortUpdateError(err)
					_ = os.Remove(e.stagedPath)
				} else {
					a.updateStatus = "Applying update and restarting..."
					addLog("Verified v" + e.manifest.Version + "; restarting for install")
					// UI-owned window closes only after the separate helper has started.
					SendMessage.Call(a.hwnd, WM_CLOSE, 0, 0)
				}
			}
			refreshUI()
		default:
			return
		}
	}
}
func shortUpdateError(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 150 {
		s = s[:147] + "..."
	}
	return s
}
func launchVerifiedUpdater(stage, expectedSHA256 string) error {
	if len(embeddedUpdater) < 1024 {
		return errors.New("updater helper unavailable")
	}
	target, err := os.Executable()
	if err != nil {
		return err
	}
	if stage == "" || filepath.Dir(stage) != filepath.Dir(target) {
		return errors.New("invalid download location")
	}
	dir := filepath.Dir(a.configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	helper, err := os.CreateTemp(dir, "RustTCStudio-Updater-*.exe")
	if err != nil {
		return err
	}
	path := helper.Name()
	if _, err = helper.Write(embeddedUpdater); err != nil {
		helper.Close()
		os.Remove(path)
		return err
	}
	if err = helper.Close(); err != nil {
		os.Remove(path)
		return err
	}
	cmd := exec.Command(path, "-target", target, "-staged", stage, "-sha256", expectedSHA256, "-pid", fmt.Sprint(os.Getpid()))
	if err := cmd.Start(); err != nil {
		os.Remove(path)
		return err
	}
	return cmd.Process.Release()
}
