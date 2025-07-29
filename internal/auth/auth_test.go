package auth

import (
	"testing"

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
