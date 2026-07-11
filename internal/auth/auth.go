// Package auth holds the primitives for Hivedock's single-admin authentication:
// password hashing (bcrypt) and cryptographically random opaque tokens for
// session and CSRF cookies. Storage lives in package store; HTTP wiring
// (cookies, middleware, handlers) lives in package server.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// MinPasswordLen is the minimum accepted admin password length. Kept modest —
// this is a single-user homelab tool behind the LAN, not a public service.
const MinPasswordLen = 8

// HashPassword returns a bcrypt hash of password at the default cost.
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(h), nil
}

// CheckPassword reports whether password matches the stored bcrypt hash. It is
// constant-time with respect to the hash (bcrypt's own comparison).
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// NewToken returns a URL-safe, cryptographically random 256-bit token, used for
// both session and CSRF cookie values.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
