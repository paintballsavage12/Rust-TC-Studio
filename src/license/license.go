// Package license creates and verifies offline, signed access codes.
// The distributed application only carries the owner's PUBLIC verification key.
package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	Prefix             = "TC1-" // compatible with already-issued 1.2/1.3 player codes
	PrefixV2           = "TC2-"
	RolePlayer         = "player"
	RoleAdmin          = "admin"
	versionV1     byte = 1
	versionV2     byte = 2
	nonceBytes         = 12
	maxCodeLength      = 256
)

var encode = base64.RawURLEncoding

type Claim struct {
	Username   string
	Identifier string
	Role       string
}

func ValidUsername(name string) bool {
	name = strings.TrimSpace(name)
	if len([]rune(name)) < 2 || len([]rune(name)) > 24 || !utf8.ValidString(name) || len([]byte(name)) > 90 {
		return false
	}
	for _, r := range name {
		if r < ' ' || r == 127 || r == '\n' || r == '\r' || r == '\t' {
			return false
		}
	}
	return true
}

// Sign maintains the legacy v1 player format for existing issuer software.
func Sign(privateKey ed25519.PrivateKey, name string) (string, Claim, error) {
	return sign(privateKey, name, RolePlayer, false)
}

// SignRole makes a v2 code whose player/admin privilege is authenticated by
// the owner's Ed25519 signature, not chosen by the client or stored in config.
func SignRole(privateKey ed25519.PrivateKey, name, role string) (string, Claim, error) {
	return sign(privateKey, name, role, true)
}

func sign(privateKey ed25519.PrivateKey, name, role string, v2 bool) (string, Claim, error) {
	name = strings.TrimSpace(name)
	if !ValidUsername(name) {
		return "", Claim{}, errors.New("username must be 2-24 visible characters")
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		return "", Claim{}, errors.New("invalid issuer signing key")
	}
	if role != RolePlayer && role != RoleAdmin {
		return "", Claim{}, errors.New("role must be player or admin")
	}
	if !v2 && role != RolePlayer {
		return "", Claim{}, errors.New("v1 codes cannot be admins")
	}
	nameBytes := []byte(name)
	header := 2
	prefix := Prefix
	if v2 {
		header = 3
		prefix = PrefixV2
	}
	payload := make([]byte, header+len(nameBytes)+nonceBytes)
	if v2 {
		payload[0] = versionV2
		if role == RoleAdmin {
			payload[1] = 1
		}
		payload[2] = byte(len(nameBytes))
	} else {
		payload[0] = versionV1
		payload[1] = byte(len(nameBytes))
	}
	copy(payload[header:], nameBytes)
	if _, err := rand.Read(payload[header+len(nameBytes):]); err != nil {
		return "", Claim{}, err
	}
	signature := ed25519.Sign(privateKey, payload)
	full := append(payload, signature...)
	return prefix + encode.EncodeToString(full), Claim{Username: name, Identifier: fmt.Sprintf("%X", payload[header+len(nameBytes):]), Role: role}, nil
}

func Verify(code string) (Claim, error) {
	publicBytes, err := encode.DecodeString(publicKeyEncoded)
	if err != nil || len(publicBytes) != ed25519.PublicKeySize {
		return Claim{}, errors.New("client verifier not configured")
	}
	return verifyWithKey(code, ed25519.PublicKey(publicBytes))
}

func verifyWithKey(code string, publicKey ed25519.PublicKey) (Claim, error) {
	code = strings.TrimSpace(code)
	if len(code) < 90 || len(code) > maxCodeLength {
		return Claim{}, errors.New("invalid access code format")
	}
	isV2 := strings.HasPrefix(code, PrefixV2)
	if !isV2 && !strings.HasPrefix(code, Prefix) {
		return Claim{}, errors.New("invalid code prefix")
	}
	prefix := Prefix
	if isV2 {
		prefix = PrefixV2
	}
	data, err := encode.DecodeString(strings.TrimPrefix(code, prefix))
	if err != nil {
		return Claim{}, errors.New("invalid code characters")
	}
	header := 2
	if isV2 {
		header = 3
	}
	if len(data) < header+2+nonceBytes+ed25519.SignatureSize {
		return Claim{}, errors.New("access code incomplete")
	}
	payload := data[:len(data)-ed25519.SignatureSize]
	signature := data[len(data)-ed25519.SignatureSize:]
	// Authenticate the complete claim before using the role or display name.
	if !ed25519.Verify(publicKey, payload, signature) {
		return Claim{}, errors.New("code signature mismatch")
	}
	role := RolePlayer
	var nameLen int
	if isV2 {
		if payload[0] != versionV2 || payload[1] > 1 {
			return Claim{}, errors.New("unsupported signed access code")
		}
		if payload[1] == 1 {
			role = RoleAdmin
		}
		nameLen = int(payload[2])
	} else {
		if payload[0] != versionV1 {
			return Claim{}, errors.New("unsupported signed access code")
		}
		nameLen = int(payload[1])
	}
	if nameLen != len(payload)-header-nonceBytes {
		return Claim{}, errors.New("malformed access code length")
	}
	username := string(payload[header : header+nameLen])
	if !ValidUsername(username) {
		return Claim{}, errors.New("invalid username in code")
	}
	return Claim{Username: username, Identifier: fmt.Sprintf("%X", payload[header+nameLen:]), Role: role}, nil
}

func ReadPrivateKey(text string) (ed25519.PrivateKey, error) {
	buf, err := encode.DecodeString(strings.TrimSpace(text))
	if err != nil || len(buf) != ed25519.SeedSize {
		return nil, errors.New("invalid owner private key file")
	}
	key := ed25519.NewKeyFromSeed(buf)
	expected, err := encode.DecodeString(publicKeyEncoded)
	if err != nil || !ed25519.PublicKey(expected).Equal(key.Public()) {
		return nil, errors.New("owner key does not match this player client")
	}
	return key, nil
}
