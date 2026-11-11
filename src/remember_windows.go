//go:build windows

package main

import (
	"crypto/subtle"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"rusttcstudio/license"
)

type dataBlob struct {
	Length uint32
	Data   *byte
}

var crypt32 = syscall.NewLazyDLL("crypt32.dll")
var protectData = crypt32.NewProc("CryptProtectData")
var unprotectData = crypt32.NewProc("CryptUnprotectData")
var localFree = syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree")

const ownerSession = "owner-local-v1"

func rememberPath() string {
	return filepath.Join(filepath.Dir(configFilename()), "remembered_session.dat")
}

func dpapi(input []byte, encrypt bool) ([]byte, error) {
	if len(input) == 0 {
		return nil, errors.New("empty input")
	}
	src := dataBlob{uint32(len(input)), &input[0]}
	dst := dataBlob{}
	var r uintptr
	var err error
	if encrypt {
		r, _, err = protectData.Call(uintptr(unsafe.Pointer(&src)), 0, 0, 0, 0, 0x1, uintptr(unsafe.Pointer(&dst)))
	} else {
		r, _, err = unprotectData.Call(uintptr(unsafe.Pointer(&src)), 0, 0, 0, 0, 0x1, uintptr(unsafe.Pointer(&dst)))
	}
	if r == 0 {
		return nil, err
	}
	defer localFree.Call(uintptr(unsafe.Pointer(dst.Data)))
	if dst.Length > 4096 {
		return nil, errors.New("stored session too large")
	}
	output := make([]byte, dst.Length)
	copy(output, unsafe.Slice(dst.Data, int(dst.Length)))
	return output, nil
}

func rememberCredential(code string) error {
	// No plaintext codes are written to the config or stored in the GitHub repo.
	encrypted, err := dpapi([]byte(code), true)
	if err != nil {
		return err
	}
	dir := filepath.Dir(rememberPath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(rememberPath(), encrypted, 0600)
}
func clearRememberedCredential() { _ = os.Remove(rememberPath()) }
func restoreRememberedCredential() bool {
	raw, err := os.ReadFile(rememberPath())
	if err != nil || len(raw) > 8192 {
		return false
	}
	decoded, err := dpapi(raw, false)
	if err != nil {
		return false
	}
	code := string(decoded)
	if strings.HasPrefix(code, "pin-session-v1:") {
		restorePINSession(strings.TrimPrefix(code, "pin-session-v1:"))
		return false
	}
	if subtle.ConstantTimeCompare([]byte(code), []byte(ownerSession)) == 1 {
		if !hasPrivateOwnerKey() {
			return false
		}
		a.isAdmin, a.ownerLocal = true, true
		a.cfg.User = "Owner:Unknown"
		a.licenseID = "LOCAL-OWNER"
	} else {
		claim, err := license.Verify(code)
		if err != nil {
			return false
		}
		a.isAdmin = claim.Role == license.RoleAdmin
		a.ownerLocal = false
		a.cfg.User = claim.Username
		a.licenseID = claim.Identifier
	}
	a.authed = true
	a.rememberLogin = true
	a.page = "dashboard"
	addLog("Restored saved Windows session")
	return true
}
