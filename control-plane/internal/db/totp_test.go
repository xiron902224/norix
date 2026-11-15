package db

import (
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

func TestValidateTOTPCodeAcceptsCurrentWindow(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	now := time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC)
	code, err := totp.GenerateCodeCustom(secret, now, totp.ValidateOpts{
		Period:    30,
		Skew:      0,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		t.Fatalf("GenerateCodeCustom: %v", err)
	}
	if !ValidateTOTPCode(secret, code, now) {
		t.Fatalf("expected code %s to validate at %s", code, now)
	}
}

func TestValidateTOTPCodeRejectsWrongCode(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	now := time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC)
	if ValidateTOTPCode(secret, "000000", now) {
		t.Fatal("expected invalid code to be rejected")
	}
	if ValidateTOTPCode(secret, "12345", now) {
		t.Fatal("expected short code to be rejected")
	}
	if ValidateTOTPCode(secret, "abcdef", now) {
		t.Fatal("expected non-numeric code to be rejected")
	}
}

func TestValidateTOTPCodeAllowsSkewWindow(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	now := time.Date(2026, 11, 14, 15, 0, 15, 0, time.UTC)
	prev := now.Add(-30 * time.Second)
	code, err := totp.GenerateCodeCustom(secret, prev, totp.ValidateOpts{
		Period:    30,
		Skew:      0,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		t.Fatalf("GenerateCodeCustom: %v", err)
	}
	if !ValidateTOTPCode(secret, code, now) {
		t.Fatalf("expected previous-window code %s to validate with skew=1", code)
	}
}
