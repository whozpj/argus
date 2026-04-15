package auth_test

import (
	"strings"
	"testing"

	"github.com/whozpj/argus/server/internal/auth"
)

func TestJWT_RoundTrip(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-1234")
	tok, err := auth.IssueToken("user-123")
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	uid, err := auth.ValidateToken(tok)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if uid != "user-123" {
		t.Errorf("uid = %q, want user-123", uid)
	}
}

func TestJWT_InvalidToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-1234")
	_, err := auth.ValidateToken("not.a.token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestJWT_TamperedToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-1234")
	tok, _ := auth.IssueToken("user-456")
	// Target the first character of the signature section — it always carries
	// all 6 bits of data (no base64 padding bits), so swapping it always
	// produces a different HMAC and a guaranteed validation failure.
	parts := strings.SplitN(tok, ".", 3)
	sig := []byte(parts[2])
	if sig[0] == 'A' {
		sig[0] = 'B'
	} else {
		sig[0] = 'A'
	}
	tampered := parts[0] + "." + parts[1] + "." + string(sig)
	_, err := auth.ValidateToken(tampered)
	if err == nil {
		t.Fatal("expected error for tampered token")
	}
}
