package auth

import "testing"

func TestPasswordHashRoundTrip(t *testing.T) {
	encoded, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}

	valid, err := VerifyPassword("correct horse battery staple", encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("correct password did not verify")
	}

	valid, err = VerifyPassword("wrong password", encoded)
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Fatal("incorrect password unexpectedly verified")
	}
}

func TestSessionTokenIsStoredOnlyAsHash(t *testing.T) {
	plainText, hash, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if plainText == "" {
		t.Fatal("session token is empty")
	}
	if len(hash) != SessionTokenBytes {
		t.Fatalf("hash length = %d, want %d", len(hash), SessionTokenBytes)
	}
	if string(hash) == plainText {
		t.Fatal("plain session token was not hashed")
	}
}
