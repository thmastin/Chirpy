package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Test HashPassword properly hashes a password
func TestHashPassword(t *testing.T) {
	password := "testpassword"
	hashedPassword, err := HashPassword(password)
	if err != nil {
		t.Errorf("Test failed: Hashing password failed: %v", err)
	}
	err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	if err != nil {
		t.Errorf("Expected password to match, but got error: %v", err)
	}

}

func TestComparePasswordHash(t *testing.T) {
	password := "testpassword"
	invalidPassword := "wrongpassword"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		t.Errorf("Test failed: Hashing password failed: %v", err)
	}

	// Test matching passwoword
	err = CheckPasswordHash(password, string(hashedPassword))
	if err != nil {
		t.Errorf("Test failed: Valid password: %s, does not match hashedPassword: %v", password, err)
	}

	// Test mismtached password
	err = CheckPasswordHash(invalidPassword, string(hashedPassword))
	if err == nil {
		t.Errorf("Tset failes: invalid password: %s, expected error and got nil", invalidPassword)
	}
}

func TestJWTRoundTrip(t *testing.T) {
	userID := uuid.New()
	secret := "test-secret"
	expiresIn := time.Hour

	// Create the JWT
	tokenString, err := MakeJWT(userID, secret, expiresIn)
	if err != nil {
		t.Fatalf("Failed to create JWT: %v", err)
	}

	// Validate the JWT
	parsedUserID, err := ValidateJWT(tokenString, secret)
	if err != nil {
		t.Fatalf("Failed to validate JWT: %v", err)
	}

	// Check that we got back the same user ID
	if parsedUserID != userID {
		t.Errorf("Expected user ID %v, got %v", userID, parsedUserID)
	}
}

func TestExpiredJWT(t *testing.T) {
	userID := uuid.New()
	secret := "test-secret"
	expiresIn := time.Millisecond * 1 // Very short expiration

	tokenString, err := MakeJWT(userID, secret, expiresIn)
	if err != nil {
		t.Fatalf("Failed to create JWT: %v", err)
	}

	// Wait for token to expire
	time.Sleep(time.Millisecond * 10)

	// Try to validate expired token
	_, err = ValidateJWT(tokenString, secret)
	if err == nil {
		t.Error("Expected error for expired token, but got none")
	}
}

func TestWrongSecret(t *testing.T) {
	userID := uuid.New()
	correctSecret := "correct-secret"
	wrongSecret := "wrong-secret"
	expiresIn := time.Hour

	tokenString, err := MakeJWT(userID, correctSecret, expiresIn)
	if err != nil {
		t.Fatalf("Failed to create JWT: %v", err)
	}

	// Try to validate with wrong secret
	_, err = ValidateJWT(tokenString, wrongSecret)
	if err == nil {
		t.Error("Expected error for wrong secret, but got none")
	}
}
