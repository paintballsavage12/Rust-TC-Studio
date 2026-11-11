package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"
)

func TestLegacyAndRoleCodes(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{RolePlayer, RoleAdmin} {
		code, issued, err := SignRole(priv, "PurpleUser", role)
		if err != nil {
			t.Fatal(err)
		}
		got, err := verifyWithKey(code, priv.Public().(ed25519.PublicKey))
		if err != nil {
			t.Fatal(err)
		}
		if got.Username != "PurpleUser" || got.Role != role || got.Identifier != issued.Identifier {
			t.Fatalf("wrong claim %+v", got)
		}
		// The role, username, and nonce are signature-covered.
		raw, _ := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(code, PrefixV2))
		raw[1] ^= 1
		changed := PrefixV2 + base64.RawURLEncoding.EncodeToString(raw)
		if _, err := verifyWithKey(changed, priv.Public().(ed25519.PublicKey)); err == nil {
			t.Fatal("tampered role accepted")
		}
		if _, err := Verify(code); err == nil {
			t.Fatal("untrusted issuer accepted")
		}
	}
	legacy, _, err := Sign(priv, "Old Player")
	if err != nil {
		t.Fatal(err)
	}
	got, err := verifyWithKey(legacy, priv.Public().(ed25519.PublicKey))
	if err != nil || got.Role != RolePlayer || got.Username != "Old Player" {
		t.Fatal("legacy compatibility broken", got, err)
	}
}
func TestInvalidCodes(t *testing.T) {
	for _, c := range []string{"", "TC1-invalid", "TC2-invalid", "TC2-" + strings.Repeat("A", 500)} {
		if _, err := Verify(c); err == nil {
			t.Fatal("accepted", c)
		}
	}
}
func TestUsernameAndRoles(t *testing.T) {
	for _, n := range []string{"", "x", strings.Repeat("x", 25), "bad\nname"} {
		if ValidUsername(n) {
			t.Fatalf("accepted %q", n)
		}
	}
	_, priv, _ := ed25519.GenerateKey(nil)
	if _, _, err := SignRole(priv, "Valid User", "super-admin"); err == nil {
		t.Fatal("unknown role")
	}
}
