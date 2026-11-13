// Package housekeeping removes only app-generated update leftovers after a
// successful application restart. It never touches saved settings or downloads.
package housekeeping

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var backupSuffix = regexp.MustCompile(`^\.previous-[0-9]{10,20}\.exe$`)
var stagedName = regexp.MustCompile(`^\.(?:RustTCStudio|RustPilot)-[0-9]+\.[0-9]+\.[0-9]+-[a-f0-9]{16}\.new\.exe$`)
var helperName = regexp.MustCompile(`^RustTCStudio-Updater-[0-9]+\.exe$`)

// CleanupInstalled removes the past-version backups belonging exactly to the
// executable that successfully started. It leaves unrelated EXEs, settings,
// manually downloaded files, and failed rollback artifacts alone.
// The caller must invoke this only after the application has started correctly.
func CleanupInstalled(executable string, now time.Time) (int, error) {
	abs, err := filepath.Abs(executable)
	if err != nil {
		return 0, err
	}
	dir := filepath.Dir(abs)
	base := filepath.Base(abs)
	if !strings.EqualFold(filepath.Ext(base), ".exe") {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, ent := range entries {
		n := ent.Name()
		isBackup := strings.HasPrefix(n, base) && backupSuffix.MatchString(strings.TrimPrefix(n, base))
		isStage := stagedName.MatchString(n)
		if !isBackup && !isStage {
			continue
		}
		path := filepath.Join(dir, n)
		info, e := os.Lstat(path)
		if e != nil || !info.Mode().IsRegular() {
			continue
		}
		// Avoid touching an updater's incomplete download. Backups need no
		// age check because we only run after the replacement has launched.
		if isStage && now.Sub(info.ModTime()) < 2*time.Hour {
			continue
		}
		if e = os.Remove(path); e == nil {
			count++
		}
	}
	return count, nil
}

// CleanupHelpers removes stale extracted update helpers in AppData. Recently
// launched helpers may still be working, so only delete files older than 24h.
func CleanupHelpers(configDir string, now time.Time) int {
	entries, err := os.ReadDir(configDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, ent := range entries {
		if !helperName.MatchString(ent.Name()) {
			continue
		}
		path := filepath.Join(configDir, ent.Name())
		st, e := os.Lstat(path)
		if e != nil || !st.Mode().IsRegular() || now.Sub(st.ModTime()) < 24*time.Hour {
			continue
		}
		if os.Remove(path) == nil {
			n++
		}
	}
	return n
}
