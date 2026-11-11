package updates

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestSignedServiceConfig(t *testing.T) {
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c, err := SignServiceConfig(private.Seed(), "https://auth.example.com")
	if err != nil {
		t.Fatal(err)
	}
	sig, err := encoding.DecodeString(c.Signature)
	if err != nil || !ed25519.Verify(pub, c.message(), sig) {
		t.Fatal("service signature failed")
	}
	if _, err = SignServiceConfig(private.Seed(), "http://insecure.local"); err == nil {
		t.Fatal("insecure service allowed")
	}
	c.URL = "https://phishing.example.com"
	if ed25519.Verify(pub, c.message(), sig) {
		t.Fatal("tampered service accepted")
	}
}
