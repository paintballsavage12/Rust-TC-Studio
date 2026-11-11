//go:build windows

// This helper runs AFTER the player exits so Windows permits replacing its EXE.
// It is embedded into the player binary; users never download it separately.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const synchronize = 0x00100000
const waitObject0 = 0
const waitTimeout = 0x00000102

var k = syscall.NewLazyDLL("kernel32.dll")
var openProcess = k.NewProc("OpenProcess")
var waitForSingleObject = k.NewProc("WaitForSingleObject")
var closeHandle = k.NewProc("CloseHandle")

func main() {
	target := flag.String("target", "", "currently installed player executable")
	staged := flag.String("staged", "", "verified downloaded executable")
	wantedHash := flag.String("sha256", "", "expected SHA-256 of replacement")
	pid := flag.Int("pid", 0, "PID of player that must exit first")
	flag.Parse()
	if err := apply(*target, *staged, strings.ToLower(*wantedHash), *pid); err != nil {
		logError("Update failed: " + err.Error())
		os.Exit(1)
	}
}
func apply(target, staged, wantedHash string, pid int) error {
	if target == "" || staged == "" || pid <= 0 || len(wantedHash) != 64 {
		return errors.New("invalid updater parameters")
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	stagedAbs, err := filepath.Abs(staged)
	if err != nil {
		return err
	}
	if filepath.Dir(targetAbs) != filepath.Dir(stagedAbs) || targetAbs == stagedAbs {
		return errors.New("download must be staged alongside installed executable")
	}
	if err := waitExit(pid); err != nil {
		return err
	}
	f, err := os.Open(stagedAbs)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(h, io.LimitReader(f, (50<<20)+1))
	f.Close()
	if err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != wantedHash {
		return errors.New("staged EXE checksum mismatch")
	}
	backup := targetAbs + ".previous-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".exe"
	var moved bool
	for i := 0; i < 35; i++ {
		err = os.Rename(targetAbs, backup)
		if err == nil {
			moved = true
			break
		}
		time.Sleep(350 * time.Millisecond)
	}
	if !moved {
		return fmt.Errorf("could not move old EXE: %w", err)
	}
	if err = os.Rename(stagedAbs, targetAbs); err != nil {
		if recoverErr := os.Rename(backup, targetAbs); recoverErr != nil {
			return fmt.Errorf("install failed (%v); rollback failed (%v); old EXE at %s", err, recoverErr, backup)
		}
		return fmt.Errorf("install failed; original restored: %w", err)
	}
	cmd := exec.Command(targetAbs)
	cmd.Dir = filepath.Dir(targetAbs)
	if err = cmd.Start(); err != nil {
		failed := targetAbs + ".failed"
		_ = os.Rename(targetAbs, failed)
		_ = os.Rename(backup, targetAbs)
		return fmt.Errorf("relaunch failed; restore attempted: %w", err)
	}
	_ = cmd.Process.Release()
	// Keep one backup until the owner/Windows has confirmed the new app starts.
	// Cleanup can be done by the user; retaining it supports manual rollback.
	return nil
}
func waitExit(pid int) error {
	h, _, _ := openProcess.Call(synchronize, 0, uintptr(pid))
	if h == 0 {
		// If the original process has already ended, renames will succeed.
		time.Sleep(850 * time.Millisecond)
		return nil
	}
	defer closeHandle.Call(h)
	result, _, err := waitForSingleObject.Call(h, 120000)
	if result == waitObject0 {
		return nil
	}
	if result == waitTimeout {
		return errors.New("player did not exit within 2 minutes")
	}
	return fmt.Errorf("could not wait for player to close: %v", err)
}
func logError(s string) {
	folder := filepath.Join(os.Getenv("LOCALAPPDATA"), "RustTCStudio")
	_ = os.MkdirAll(folder, 0700)
	f, err := os.OpenFile(filepath.Join(folder, "updater.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, time.Now().Format(time.RFC3339), s)
}
