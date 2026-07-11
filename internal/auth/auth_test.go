package auth

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("hash equals plaintext")
	}
	if !CheckPassword(hash, "correct horse battery staple") {
		t.Error("CheckPassword rejected the correct password")
	}
	if CheckPassword(hash, "wrong") {
		t.Error("CheckPassword accepted a wrong password")
	}
}

func TestNewTokenUniqueAndNonEmpty(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		tok, err := NewToken()
		if err != nil {
			t.Fatalf("NewToken: %v", err)
		}
		if tok == "" {
			t.Fatal("empty token")
		}
		if seen[tok] {
			t.Fatalf("duplicate token %q", tok)
		}
		seen[tok] = true
	}
}
