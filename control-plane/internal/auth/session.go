package auth

import "time"

type SessionIdentity struct {
	UserID         string
	OrganizationID string
	Email          string
	Name           string
	Role           string
	ExpiresAt      time.Time
}
