package housekeeping

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupOnlyGeneratedFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Rust_TC_Studio.exe")
	now := time.Now()
	write := func(name string, age time.Duration) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("dummy"), 0600); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(p, now.Add(-age), now.Add(-age))
	}
	write("Rust_TC_Studio.exe", 0)
	write("Rust_TC_Studio.exe.previous-1763000000123456789.exe", time.Hour)
	write("Rust_TC_Studio.exe.previous-notanumber.exe", 48*time.Hour)
	write("RustPilot.exe.previous-1763000000123456789.exe", 48*time.Hour)
	write(".RustTCStudio-1.5.4-0123456789abcdef.new.exe", 48*time.Hour)
	write(".RustPilot-1.5.5-abcdef0123456789.new.exe", time.Minute)
	write("other-user-file.zip", 48*time.Hour)
	n, err := CleanupInstalled(target, now)
	if err != nil || n != 2 {
		t.Fatalf("removed %d, err %v", n, err)
	}
	for _, keep := range []string{"Rust_TC_Studio.exe", "Rust_TC_Studio.exe.previous-notanumber.exe", "RustPilot.exe.previous-1763000000123456789.exe", ".RustPilot-1.5.5-abcdef0123456789.new.exe", "other-user-file.zip"} {
		if _, e := os.Stat(filepath.Join(dir, keep)); e != nil {
			t.Fatalf("deleted %s", keep)
		}
	}
}
func TestHelperCleanup(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	for _, v := range []struct {
		name string
		age  time.Duration
	}{
		{"RustTCStudio-Updater-78942.exe", 48 * time.Hour}, {"RustTCStudio-Updater-00001.exe", time.Minute}, {"settings.json", 48 * time.Hour},
	} {
		p := filepath.Join(dir, v.name)
		os.WriteFile(p, []byte("x"), 0600)
		os.Chtimes(p, now.Add(-v.age), now.Add(-v.age))
	}
	if n := CleanupHelpers(dir, now); n != 1 {
		t.Fatalf("removed %d", n)
	}
}
