package db

import (
	"context"
	"encoding/base32"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/xiron902224/norix/control-plane/internal/secrets"
)

type TOTPEnrollment struct {
	Secret     string
	OTPAUTHURL string
}

// ValidateTOTPCode checks a 6-digit code against a base32 secret using a ±1 step window.
func ValidateTOTPCode(secret, code string, at time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	ok, err := totp.ValidateCustom(code, secret, at, totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}

func encryptTOTPSecret(plain string) (enc string, nonce []byte, err error) {
	key, keyErr := secrets.KeyFromEnv()
	if keyErr != nil {
		// Dev fallback when NORIX_BUILD_SECRETS_KEY is unset (matches network secrets).
		return plain, []byte("dev-nonce"), nil
	}
	ciphertext, nonce, err := secrets.Encrypt(key, []byte(plain))
	if err != nil {
		return "", nil, err
	}
	return base32.StdEncoding.EncodeToString(ciphertext), nonce, nil
}

func decryptTOTPSecret(enc string, nonce []byte) (string, error) {
	if string(nonce) == "dev-nonce" {
		return enc, nil
	}
	key, err := secrets.KeyFromEnv()
	if err != nil {
		return "", fmt.Errorf("decrypt totp secret: %w", err)
	}
	ciphertext, err := base32.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", fmt.Errorf("decode totp ciphertext: %w", err)
	}
	plain, err := secrets.Decrypt(key, ciphertext, nonce)
	if err != nil {
		return "", fmt.Errorf("decrypt totp secret: %w", err)
	}
	return string(plain), nil
}

func EnableTOTP(ctx context.Context, pool *pgxpool.Pool, organizationID, userID, email string) (TOTPEnrollment, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Norix",
		AccountName: email,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
		SecretSize:  20,
	})
	if err != nil {
		return TOTPEnrollment{}, fmt.Errorf("generate totp secret: %w", err)
	}
	secret := key.Secret()
	enc, nonce, err := encryptTOTPSecret(secret)
	if err != nil {
		return TOTPEnrollment{}, fmt.Errorf("encrypt totp secret: %w", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return TOTPEnrollment{}, fmt.Errorf("begin enable totp: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return TOTPEnrollment{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_totp_secrets (organization_id, user_id, secret_enc, secret_nonce, enabled)
		VALUES ($1::uuid, $2::uuid, $3, $4, FALSE)
		ON CONFLICT (user_id) DO UPDATE SET
			secret_enc = EXCLUDED.secret_enc,
			secret_nonce = EXCLUDED.secret_nonce,
			enabled = FALSE,
			verified_at = NULL`,
		organizationID, userID, enc, nonce); err != nil {
		return TOTPEnrollment{}, fmt.Errorf("store totp secret: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return TOTPEnrollment{}, fmt.Errorf("commit totp: %w", err)
	}
	return TOTPEnrollment{Secret: secret, OTPAUTHURL: key.URL()}, nil
}

func loadTOTPSecret(ctx context.Context, tx pgx.Tx, organizationID, userID string) (secret string, enabled bool, err error) {
	var enc string
	var nonce []byte
	err = tx.QueryRow(ctx, `
		SELECT secret_enc, secret_nonce, enabled FROM user_totp_secrets
		WHERE organization_id = $1::uuid AND user_id = $2::uuid`, organizationID, userID).Scan(&enc, &nonce, &enabled)
	if err != nil {
		return "", false, err
	}
	if enc == "" {
		return "", enabled, fmt.Errorf("totp secret is empty; re-enroll required")
	}
	secret, err = decryptTOTPSecret(enc, nonce)
	if err != nil {
		return "", false, err
	}
	return secret, enabled, nil
}

func VerifyTOTP(ctx context.Context, pool *pgxpool.Pool, organizationID, userID, code string) (bool, error) {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false, fmt.Errorf("verification code is required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin verify totp: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return false, err
	}
	secret, _, err := loadTOTPSecret(ctx, tx, organizationID, userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, fmt.Errorf("totp is not enabled for this user")
		}
		return false, fmt.Errorf("load totp secret: %w", err)
	}
	if !ValidateTOTPCode(secret, code, time.Now()) {
		return false, fmt.Errorf("invalid verification code")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE user_totp_secrets
		SET enabled = TRUE, verified_at = $3
		WHERE organization_id = $1::uuid AND user_id = $2::uuid`,
		organizationID, userID, time.Now()); err != nil {
		return false, fmt.Errorf("mark totp verified: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit totp verify: %w", err)
	}
	return true, nil
}

// ValidateUserTOTP verifies a login-time code without changing enrollment state.
func ValidateUserTOTP(ctx context.Context, pool *pgxpool.Pool, organizationID, userID, code string) (bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return false, err
	}
	secret, enabled, err := loadTOTPSecret(ctx, tx, organizationID, userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, fmt.Errorf("totp is not configured for this user")
		}
		return false, err
	}
	if !enabled {
		return false, fmt.Errorf("totp is not verified for this user")
	}
	return ValidateTOTPCode(secret, code, time.Now()), nil
}

func UserTOTPEnabled(ctx context.Context, pool *pgxpool.Pool, organizationID, userID string) (bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return false, err
	}
	var enabled bool
	err = tx.QueryRow(ctx, `
		SELECT enabled FROM user_totp_secrets
		WHERE organization_id = $1::uuid AND user_id = $2::uuid`, organizationID, userID).Scan(&enabled)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return enabled, nil
}

func OrganizationMandatory2FA(ctx context.Context, pool *pgxpool.Pool, organizationID string) (bool, error) {
	var mandatory bool
	err := pool.QueryRow(ctx, `SELECT mandatory_2fa FROM organizations WHERE id = $1::uuid`, organizationID).Scan(&mandatory)
	if err != nil {
		return false, fmt.Errorf("mandatory 2fa flag: %w", err)
	}
	return mandatory, nil
}
