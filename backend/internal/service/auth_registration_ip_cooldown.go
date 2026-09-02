package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	RegistrationIPCooldownSecondsDefault = 300
	RegistrationIPCooldownSecondsMax     = 86400
	registrationIPCooldownReservationTTL = 120
)

var ErrRegistrationIPCooldown = infraerrors.TooManyRequests(
	"REGISTRATION_IP_COOLDOWN",
	"registration from this IP is cooling down",
)

type registrationIPCooldownClaim struct {
	ipHash string
	token  string
}

// IsRegistrationIPCooldownEnabled reports the opt-in switch. Any missing or
// unreadable setting fails closed so a partially migrated database cannot
// unexpectedly enable a balance-independent registration restriction.
func (s *SettingService) IsRegistrationIPCooldownEnabled(ctx context.Context) bool {
	if s == nil || s.settingRepo == nil {
		return false
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationIPCooldownEnabled)
	return err == nil && raw == "true"
}

// GetRegistrationIPCooldownSeconds returns a bounded value even when the
// persisted setting is absent or malformed.
func (s *SettingService) GetRegistrationIPCooldownSeconds(ctx context.Context) int {
	if s == nil || s.settingRepo == nil {
		return RegistrationIPCooldownSecondsDefault
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationIPCooldownSeconds)
	if err != nil {
		return RegistrationIPCooldownSecondsDefault
	}
	return parseRegistrationIPCooldownSeconds(raw)
}

func parseRegistrationIPCooldownSeconds(raw string) int {
	seconds, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || seconds <= 0 {
		return RegistrationIPCooldownSecondsDefault
	}
	if seconds > RegistrationIPCooldownSecondsMax {
		return RegistrationIPCooldownSecondsMax
	}
	return seconds
}

// registrationIPCooldownClient respects an Ent transaction carried by ctx.
// This matters for pending OAuth finalization, where the user/identity binding
// and the cooldown state must observe the same database transaction.
func (s *AuthService) registrationIPCooldownClient(ctx context.Context) *dbent.Client {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	if s == nil {
		return nil
	}
	return s.entClient
}

// claimRegistrationIPCooldown reserves a trusted client IP for one new user.
// A nil claim means the feature is disabled. Database failures deliberately
// fail closed while the feature is enabled.
func (s *AuthService) claimRegistrationIPCooldown(ctx context.Context) (*registrationIPCooldownClaim, error) {
	if s == nil || s.settingService == nil || !s.settingService.IsRegistrationIPCooldownEnabled(ctx) {
		return nil, nil
	}
	client := s.registrationIPCooldownClient(ctx)
	if client == nil {
		return nil, ErrServiceUnavailable
	}
	cooldownSeconds := s.settingService.GetRegistrationIPCooldownSeconds(ctx)
	clientIP := affiliateSignupIPFromContext(ctx)
	if strings.TrimSpace(clientIP) == "" {
		logger.LegacyPrintf("service.auth", "%s", "[Auth] registration IP cooldown enabled but trusted client IP is unavailable")
		return nil, ErrServiceUnavailable
	}
	token, err := randomHexString(32)
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	ipHash := s.hashRegistrationIP(clientIP)
	rows, err := client.QueryContext(ctx, `
INSERT INTO registration_ip_cooldowns (ip_hash, reservation_token, reserved_until, created_at, updated_at)
VALUES ($1, $2, NOW() + ($4::int * INTERVAL '1 second'), NOW(), NOW())
ON CONFLICT (ip_hash) DO UPDATE
SET reservation_token = EXCLUDED.reservation_token,
    reserved_until = EXCLUDED.reserved_until,
    last_user_id = NULL,
    updated_at = NOW()
WHERE (registration_ip_cooldowns.last_registered_at IS NULL
       OR registration_ip_cooldowns.last_registered_at <= NOW() - ($3::int * INTERVAL '1 second'))
  AND (registration_ip_cooldowns.reserved_until IS NULL
       OR registration_ip_cooldowns.reserved_until <= NOW())
RETURNING reservation_token`, ipHash, token, cooldownSeconds, registrationIPCooldownReservationTTL)
	if err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to claim registration IP cooldown: %v", err)
		return nil, ErrServiceUnavailable
	}
	defer rows.Close()
	if rows.Next() {
		var returnedToken string
		if err := rows.Scan(&returnedToken); err != nil || strings.TrimSpace(returnedToken) != token {
			logger.LegacyPrintf("service.auth", "%s", "[Auth] Failed to read registration IP cooldown claim")
			return nil, ErrServiceUnavailable
		}
		return &registrationIPCooldownClaim{ipHash: ipHash, token: token}, nil
	}
	if err := rows.Err(); err != nil {
		return nil, ErrServiceUnavailable
	}
	return nil, s.registrationIPCooldownError(ctx, ipHash, cooldownSeconds)
}

func (s *AuthService) attachRegistrationIPCooldownClaim(ctx context.Context, claim *registrationIPCooldownClaim, userID int64) error {
	if claim == nil || userID <= 0 {
		return nil
	}
	client := s.registrationIPCooldownClient(ctx)
	if client == nil {
		return ErrServiceUnavailable
	}
	result, err := client.ExecContext(ctx, `
UPDATE registration_ip_cooldowns
SET last_user_id = $3, updated_at = NOW()
WHERE ip_hash = $1 AND reservation_token = $2`, claim.ipHash, claim.token, userID)
	if err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to attach registration IP cooldown claim for user %d: %v", userID, err)
		return ErrServiceUnavailable
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		logger.LegacyPrintf("service.auth", "[Auth] Registration IP cooldown attach affected %d rows for user %d", affected, userID)
		return ErrServiceUnavailable
	}
	return nil
}

func (s *AuthService) finalizeRegistrationIPCooldown(ctx context.Context, claim *registrationIPCooldownClaim, userID int64) error {
	if claim == nil || userID <= 0 {
		return nil
	}
	client := s.registrationIPCooldownClient(ctx)
	if client == nil {
		return ErrServiceUnavailable
	}
	result, err := client.ExecContext(ctx, `
UPDATE registration_ip_cooldowns
SET last_registered_at = NOW(), last_user_id = $3,
    reservation_token = NULL, reserved_until = NULL, updated_at = NOW()
WHERE ip_hash = $1 AND reservation_token = $2`, claim.ipHash, claim.token, userID)
	if err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to finalize registration IP cooldown for user %d: %v", userID, err)
		return ErrServiceUnavailable
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		logger.LegacyPrintf("service.auth", "[Auth] Registration IP cooldown finalize affected %d rows for user %d", affected, userID)
		return ErrServiceUnavailable
	}
	return nil
}

func (s *AuthService) finalizeRegistrationIPCooldownForUser(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return nil
	}
	client := s.registrationIPCooldownClient(ctx)
	if client == nil {
		return ErrServiceUnavailable
	}
	result, err := client.ExecContext(ctx, `
UPDATE registration_ip_cooldowns
SET last_registered_at = NOW(), reservation_token = NULL,
    reserved_until = NULL, updated_at = NOW()
WHERE last_user_id = $1 AND reservation_token IS NOT NULL`, userID)
	if err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to finalize registration IP cooldown by user %d: %v", userID, err)
		return ErrServiceUnavailable
	}
	if affected, err := result.RowsAffected(); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Registration IP cooldown finalize-by-user affected %d rows for user %d", affected, userID)
		return ErrServiceUnavailable
	} else if affected > 1 {
		logger.LegacyPrintf("service.auth", "[Auth] Registration IP cooldown finalize-by-user affected %d rows for user %d", affected, userID)
		return ErrServiceUnavailable
	}
	return nil
}

func (s *AuthService) releaseRegistrationIPCooldownClaim(ctx context.Context, claim *registrationIPCooldownClaim) error {
	if claim == nil {
		return nil
	}
	client := s.registrationIPCooldownClient(ctx)
	if client == nil {
		return ErrServiceUnavailable
	}
	result, err := client.ExecContext(ctx, `
UPDATE registration_ip_cooldowns
SET last_user_id = NULL, reservation_token = NULL,
    reserved_until = NULL, updated_at = NOW()
WHERE ip_hash = $1 AND reservation_token = $2`, claim.ipHash, claim.token)
	if err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to release registration IP cooldown claim: %v", err)
		return ErrServiceUnavailable
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		logger.LegacyPrintf("service.auth", "[Auth] Registration IP cooldown release affected %d rows", affected)
		return ErrServiceUnavailable
	}
	return nil
}

func (s *AuthService) releaseRegistrationIPCooldownForUser(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return nil
	}
	client := s.registrationIPCooldownClient(ctx)
	if client == nil {
		return ErrServiceUnavailable
	}
	// Keep last_registered_at intact. Rollback cleanup can fail after the user
	// row was created; retaining the timestamp prevents an early re-registration
	// while the database is in an uncertain state. A later explicit claim can
	// safely replace the expired reservation once the normal cooldown elapses.
	result, err := client.ExecContext(ctx, `
UPDATE registration_ip_cooldowns
SET last_user_id = NULL,
    reservation_token = NULL, reserved_until = NULL, updated_at = NOW()
WHERE last_user_id = $1`, userID)
	if err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Failed to release registration IP cooldown for rolled back user %d: %v", userID, err)
		return ErrServiceUnavailable
	}
	if affected, err := result.RowsAffected(); err != nil || affected > 1 {
		logger.LegacyPrintf("service.auth", "[Auth] Registration IP cooldown rollback release affected %d rows for user %d", affected, userID)
		return ErrServiceUnavailable
	}
	return nil
}

func (s *AuthService) registrationIPCooldownError(ctx context.Context, ipHash string, cooldownSeconds int) error {
	remaining := cooldownSeconds
	client := s.registrationIPCooldownClient(ctx)
	if client != nil && ipHash != "" {
		rows, err := client.QueryContext(ctx, `
SELECT GREATEST(0, EXTRACT(EPOCH FROM (
    GREATEST(
        COALESCE(last_registered_at + ($2::int * INTERVAL '1 second'), NOW()),
        COALESCE(reserved_until, NOW())
    ) - NOW())))
FROM registration_ip_cooldowns WHERE ip_hash = $1`, ipHash, cooldownSeconds)
		if err == nil {
			defer rows.Close()
			if rows.Next() {
				var seconds float64
				if scanErr := rows.Scan(&seconds); scanErr == nil && seconds >= 0 {
					remaining = int(math.Ceil(seconds))
				}
			}
		}
	}
	if remaining < 0 {
		remaining = 0
	}
	return ErrRegistrationIPCooldown.WithMetadata(map[string]string{
		"retry_after_seconds": strconv.Itoa(remaining),
	})
}

func (s *AuthService) hashRegistrationIP(clientIP string) string {
	secret := ""
	if s != nil && s.cfg != nil {
		secret = strings.TrimSpace(s.cfg.JWT.Secret)
	}
	sum := sha256.Sum256([]byte(secret + "\x00" + strings.TrimSpace(clientIP)))
	return hex.EncodeToString(sum[:])
}

// joinRegistrationCleanupErrors preserves the original application error
// while making failed compensating actions observable to callers. It is used
// after a user row has been created but a later registration step failed.
func joinRegistrationCleanupErrors(primary error, cleanupErrs ...error) error {
	filtered := make([]error, 0, len(cleanupErrs)+1)
	if primary != nil {
		filtered = append(filtered, primary)
	}
	for _, err := range cleanupErrs {
		if err != nil {
			filtered = append(filtered, err)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return errors.Join(filtered...)
}

// releaseRegistrationIPCooldownClaimBestEffort is intentionally limited to
// defer paths whose return values cannot be changed. Explicit failure paths
// use releaseRegistrationIPCooldownClaim directly and propagate its error.
func (s *AuthService) releaseRegistrationIPCooldownClaimBestEffort(ctx context.Context, claim *registrationIPCooldownClaim, operation string) {
	if err := s.releaseRegistrationIPCooldownClaim(ctx, claim); err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] registration IP cooldown %s cleanup failed: %v", operation, err)
	}
}

func wrapRegistrationCleanupError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func (s *AuthService) registrationIPCooldownEnabled(ctx context.Context) bool {
	return s != nil && s.settingService != nil && s.settingService.IsRegistrationIPCooldownEnabled(ctx)
}
