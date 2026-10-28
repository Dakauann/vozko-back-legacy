package sitepublish

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func passwordHash(t *testing.T, password string) string {
	t.Helper()
	salt := []byte("0123456789abcdef")
	key, err := pbkdf2.Key(sha256.New, password, salt, minHashIterations, 32)
	if err != nil {
		t.Fatal(err)
	}
	return "pbkdf2-sha256$100000$" + base64.RawURLEncoding.EncodeToString(salt) + "$" + base64.StdEncoding.EncodeToString(key)
}

func TestPagePasswordChecksThePBKDF2Hash(t *testing.T) {
	hash := passwordHash(t, "sorriso123")
	if !VerifyPagePassword("sorriso123", hash) {
		t.Fatal("the right password opens the page")
	}
	for _, wrong := range []string{"", "sorriso124", strings.Repeat("a", MaxPagePasswordLength+1)} {
		if VerifyPagePassword(wrong, hash) {
			t.Fatalf("%q must be refused", wrong)
		}
	}
	for _, broken := range []string{"", "plain", "pbkdf2-sha256$1$c2FsdA$aGFzaA", "pbkdf2-sha256$100000$!!$!!", "pbkdf2-sha256$900000$c2FsdHNhbHRzYWx0$aGFzaA"} {
		if VerifyPagePassword("sorriso123", broken) {
			t.Fatalf("%q must never open the page", broken)
		}
	}
}

func TestPassTokenIsBoundToHostDeploymentAndHash(t *testing.T) {
	secret := []byte(strings.Repeat("k", 32))
	now := time.Unix(1_800_000_000, 0)
	settings := ServingSettings{DeploymentID: "d1", PasswordHash: passwordHash(t, "x")}
	token := PassToken(secret, "sorriso.paginas.test", settings, now)
	if !ValidPassToken(secret, "sorriso.paginas.test", settings, token, now.Add(time.Hour)) {
		t.Fatal("a fresh token passes")
	}
	if ValidPassToken(secret, "sorriso.paginas.test", settings, token, now.Add(PassTokenLifetime+time.Second)) {
		t.Fatal("an expired token fails")
	}
	if ValidPassToken(secret, "outra.paginas.test", settings, token, now) {
		t.Fatal("a token belongs to one host")
	}
	moved := settings
	moved.DeploymentID = "d2"
	if ValidPassToken(secret, "sorriso.paginas.test", moved, token, now) {
		t.Fatal("a new deployment asks again")
	}
	for _, forged := range []string{"", "123", "abc.def", strings.Split(token, ".")[0] + ".00"} {
		if ValidPassToken(secret, "sorriso.paginas.test", settings, forged, now) {
			t.Fatalf("%q must fail", forged)
		}
	}
}

func TestSafeNextStaysOnThePage(t *testing.T) {
	cases := map[string]string{"/promo?a=1": "/promo?a=1", "//evil.test": "/", "/\\evil.test": "/", "https://evil.test": "/", "": "/"}
	for raw, want := range cases {
		if got := SafeNext(raw); got != want {
			t.Fatalf("%q: got %q", raw, got)
		}
	}
	if got := SafeNext("/" + strings.Repeat("a", 600)); len(got) != maxNextLength {
		t.Fatalf("got %d", len(got))
	}
}
