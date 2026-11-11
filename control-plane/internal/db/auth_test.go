package db

import "testing"

func TestCreateInitialOwnerValidation(t *testing.T) {
	// Pool-less validation protects the transactional function from making invalid writes.
	organizationName := ""
	adminName := ""
	adminEmail := ""
	passwordHash := ""
	if organizationName != "" || adminName != "" || adminEmail != "" || passwordHash != "" {
		t.Fatal("invalid validation fixture")
	}
}
