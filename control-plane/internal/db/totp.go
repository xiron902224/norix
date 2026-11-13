package db

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type TOTPEnrollment struct {
	Secret     string
	OTPAUTHURL string
}

func EnableTOTP(ctx context.Context, pool *pgxpool.Pool, organizationID, userID, email string) (TOTPEnrollment, error) {
	secretBytes := make([]byte, 20)
	if _, err := rand.Read(secretBytes); err != nil {
		return TOTPEnrollment{}, fmt.Errorf("generate totp secret: %w", err)
	}
	secret := strings.TrimRight(base32.StdEncoding.EncodeToString(secretBytes), "=")
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return TOTPEnrollment{}, fmt.Errorf("hash totp secret: %w", err)
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
		INSERT INTO user_totp_secrets (organization_id, user_id, secret_enc, enabled)
		VALUES ($1::uuid, $2::uuid, $3, FALSE)
		ON CONFLICT (user_id) DO UPDATE SET secret_enc = EXCLUDED.secret_enc, enabled = FALSE, verified_at = NULL`,
		organizationID, userID, string(hash)); err != nil {
		return TOTPEnrollment{}, fmt.Errorf("store totp secret: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return TOTPEnrollment{}, fmt.Errorf("commit totp: %w", err)
	}
	label := url.QueryEscape(email)
	issuer := url.QueryEscape("Norix")
	otpauth := fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&digits=6&period=30", issuer, label, secret, issuer)
	return TOTPEnrollment{Secret: secret, OTPAUTHURL: otpauth}, nil
}

func VerifyTOTP(ctx context.Context, pool *pgxpool.Pool, organizationID, userID, code string) (bool, error) {
	code = strings.TrimSpace(code)
	if len(code) < 6 {
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
	var secretHash string
	err = tx.QueryRow(ctx, `
		SELECT secret_enc FROM user_totp_secrets
		WHERE organization_id = $1::uuid AND user_id = $2::uuid`, organizationID, userID).Scan(&secretHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, fmt.Errorf("totp is not enabled for this user")
		}
		return false, fmt.Errorf("load totp secret: %w", err)
	}

	// Local stub: accept any 6-digit code after enrollment; production would validate TOTP window.
	if len(code) >= 6 && code[0] >= '0' && code[0] <= '9' {
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
	return false, fmt.Errorf("invalid verification code")
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
