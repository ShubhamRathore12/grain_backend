package handlers

import (
	"crypto/subtle"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// -----------------------------------------------------------------------------
// Password storage (S-03, S-04)
//
// Passwords were compared in SQL against a plaintext column. Three stored forms
// now exist, and the prefix carries the rotation requirement so no schema change
// is needed:
//
//	$2a$... / $2b$... / $2y$...   bcrypt hash the user chose      -> no rotation
//	INIT:$2a$...                  bcrypt hash an admin issued     -> must rotate
//	anything else                 legacy plaintext                -> must rotate,
//	                                                                 rehashed on
//	                                                                 next login
//
// That makes "the credential someone else generated for you" a distinguishable
// state, which is what mandatory first-login rotation needs.
// -----------------------------------------------------------------------------

// initialPasswordPrefix marks a hash created by an administrator rather than the
// account owner.
const initialPasswordPrefix = "INIT:"

// bcryptCost of 12 is a deliberate step above the library default of 10: login
// is infrequent here, so the extra work costs users nothing and costs an
// offline cracker a lot.
const bcryptCost = 12

// minPasswordLength is the floor from the review's password-policy
// recommendation. Length beats composition rules, so this is set high rather
// than adding symbol requirements.
const minPasswordLength = 12

// commonPasswords is a small local screen for the passwords that appear at the
// top of every breach corpus. It is not a substitute for a breached-password
// API; see SECURITY_REMEDIATION_BACKEND.md.
var commonPasswords = map[string]bool{
	"password": true, "password1": true, "password123": true, "passw0rd": true,
	"123456": true, "1234567890": true, "12345678": true, "qwerty": true,
	"qwertyuiop": true, "letmein": true, "welcome": true, "welcome123": true,
	"admin": true, "admin123": true, "administrator": true, "root": true,
	"iloveyou": true, "abc123": true, "changeme": true, "changeme123": true,
	"grain": true, "graintechnik": true, "grain123": true, "chiller": true,
	"secret": true, "monkey": true, "dragon": true, "sunshine": true,
}

// HashPassword produces a bcrypt hash of a password the user chose themselves.
func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// HashInitialPassword produces a hash flagged as admin-issued, so the account
// cannot use the API until the owner replaces it.
func HashInitialPassword(plain string) (string, error) {
	hash, err := HashPassword(plain)
	if err != nil {
		return "", err
	}
	return initialPasswordPrefix + hash, nil
}

func isBcryptHash(stored string) bool {
	return strings.HasPrefix(stored, "$2a$") ||
		strings.HasPrefix(stored, "$2b$") ||
		strings.HasPrefix(stored, "$2y$")
}

// passwordCheck reports the outcome of verifying a submitted password.
type passwordCheck struct {
	Valid bool
	// MustRotate is true when the credential was issued by an administrator or
	// is still in the legacy plaintext column.
	MustRotate bool
	// Upgraded carries a new stored value when the record needs rewriting
	// (plaintext -> hashed). Empty when no rewrite is required.
	Upgraded string
}

// VerifyPassword checks a submitted password against any of the three stored
// forms. bcrypt.CompareHashAndPassword is already constant-time; the plaintext
// branch uses subtle.ConstantTimeCompare so the legacy path does not leak
// length or content through timing.
func VerifyPassword(stored, submitted string) passwordCheck {
	switch {
	case strings.HasPrefix(stored, initialPasswordPrefix):
		hash := strings.TrimPrefix(stored, initialPasswordPrefix)
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(submitted)) != nil {
			return passwordCheck{}
		}
		return passwordCheck{Valid: true, MustRotate: true}

	case isBcryptHash(stored):
		if bcrypt.CompareHashAndPassword([]byte(stored), []byte(submitted)) != nil {
			return passwordCheck{}
		}
		return passwordCheck{Valid: true}

	default:
		if stored == "" {
			return passwordCheck{}
		}
		if subtle.ConstantTimeCompare([]byte(stored), []byte(submitted)) != 1 {
			return passwordCheck{}
		}
		// Correct, but stored in the clear. Re-store it hashed and require the
		// owner to pick a new one.
		upgraded, err := HashInitialPassword(submitted)
		if err != nil {
			upgraded = ""
		}
		return passwordCheck{Valid: true, MustRotate: true, Upgraded: upgraded}
	}
}

// ValidatePasswordPolicy enforces length, character variety, reuse of the
// username, and the local common-password screen. The returned message is shown
// to the user, so it states what to fix.
func ValidatePasswordPolicy(password, username string) error {
	if len([]rune(password)) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters long", minPasswordLength)
	}
	if len(password) > 200 {
		// bcrypt only considers the first 72 bytes; cap the input rather than
		// silently ignoring the tail.
		return fmt.Errorf("password must be 200 characters or fewer")
	}

	var hasLetter, hasDigit bool
	for _, c := range password {
		switch {
		case unicode.IsLetter(c):
			hasLetter = true
		case unicode.IsDigit(c):
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return fmt.Errorf("password must contain at least one letter and one number")
	}

	lower := strings.ToLower(password)
	if commonPasswords[lower] {
		return fmt.Errorf("password is too common; choose something unique")
	}
	if username != "" && strings.Contains(lower, strings.ToLower(strings.TrimSpace(username))) {
		return fmt.Errorf("password must not contain your username")
	}

	return nil
}
