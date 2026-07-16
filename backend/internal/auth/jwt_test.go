package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTRoundTrip(t *testing.T) {
	j := NewJWT("test-secret", time.Minute)
	const userID = "8a5075bb-7a92-4907-bd8c-73923a4ac9f5"

	token, err := j.Issue(userID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	got, err := j.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got != userID {
		t.Fatalf("Verify returned %q, want %q", got, userID)
	}
}

func TestJWTWrongSecretRejected(t *testing.T) {
	token, err := NewJWT("secret-a", time.Minute).Issue("user-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := NewJWT("secret-b", time.Minute).Verify(token); err == nil {
		t.Fatal("token signed with a different secret was accepted")
	}
}

func TestJWTExpiredRejected(t *testing.T) {
	token, err := NewJWT("test-secret", -time.Minute).Issue("user-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := NewJWT("test-secret", time.Minute).Verify(token); err == nil {
		t.Fatal("expired token was accepted")
	}
}

func TestJWTGarbageRejected(t *testing.T) {
	j := NewJWT("test-secret", time.Minute)
	for _, tok := range []string{"", "not-a-jwt", "a.b.c"} {
		if _, err := j.Verify(tok); err == nil {
			t.Fatalf("garbage token %q was accepted", tok)
		}
	}
}

func TestJWTWrongAlgorithmRejected(t *testing.T) {
	// alg=none tokens must never validate.
	claims := jwt.RegisteredClaims{
		Subject:   "user-1",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	if _, err := NewJWT("test-secret", time.Minute).Verify(unsigned); err == nil {
		t.Fatal("alg=none token was accepted")
	}
}

func TestJWTMissingSubjectRejected(t *testing.T) {
	claims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := NewJWT("test-secret", time.Minute).Verify(token); err == nil {
		t.Fatal("token without sub was accepted")
	}
}
