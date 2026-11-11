package updates

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestSignatureAndTamper(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	m, err := Sign(priv.Seed(), "1.3.1", "https://example.org/releases/v1.3.1.exe", make([]byte, 1024))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyWithKey(m, pub); err != nil {
		t.Fatal(err)
	}
	m.SHA256 = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if VerifyWithKey(m, pub) == nil {
		t.Fatal("accepted altered hash")
	}
}
func TestVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{{"1.3.0", "1.2.9", 1}, {"1.3.0", "1.3.0", 0}, {"1.3.0", "1.4.0", -1}, {"2.0.0", "10.0.0", -1}}
	for _, tt := range cases {
		got, err := CompareVersion(tt.a, tt.b)
		if err != nil || got != tt.want {
			t.Fatalf("%v %v got %d %v", tt.a, tt.b, got, err)
		}
	}
}
func TestRejectInsecureTransport(t *testing.T) {
	if HTTPS("http://example.org/latest.json") == nil {
		t.Fatal("http allowed")
	}
}
