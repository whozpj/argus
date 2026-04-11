package auth_test

import (
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
	// Flip the last character to produce an invalid signature.
	last := tok[len(tok)-1]
	var replacement byte
	if last == 'X' {
		replacement = 'Y'
	} else {
		replacement = 'X'
	}
	tampered := tok[:len(tok)-1] + string(replacement)
	_, err := auth.ValidateToken(tampered)
	if err == nil {
		t.Fatal("expected error for tampered token")
	}
}
