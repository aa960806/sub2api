//go:build unit

package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newRegistrationCooldownSQLClient(t *testing.T) (*dbent.Client, *sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	return client, db, mock
}

func TestParseRegistrationIPCooldownSecondsBoundsAndDefaults(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{raw: "", want: RegistrationIPCooldownSecondsDefault},
		{raw: "  bad ", want: RegistrationIPCooldownSecondsDefault},
		{raw: "0", want: RegistrationIPCooldownSecondsDefault},
		{raw: "-10", want: RegistrationIPCooldownSecondsDefault},
		{raw: "600", want: 600},
		{raw: "999999", want: RegistrationIPCooldownSecondsMax},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			require.Equal(t, tt.want, parseRegistrationIPCooldownSeconds(tt.raw))
		})
	}
}

func TestRegistrationIPCooldownSettingFailsClosedWhenMissingOrMalformed(t *testing.T) {
	cfg := testRegistrationCooldownConfig()
	setting := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyRegistrationIPCooldownEnabled: "TRUE",
		SettingKeyRegistrationIPCooldownSeconds: "not-a-number",
	}}, cfg)

	require.False(t, setting.IsRegistrationIPCooldownEnabled(context.Background()))
	require.Equal(t, RegistrationIPCooldownSecondsDefault, setting.GetRegistrationIPCooldownSeconds(context.Background()))
}

func TestClaimRegistrationIPCooldownRequiresTrustedClientIP(t *testing.T) {
	setting := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyRegistrationIPCooldownEnabled: "true",
	}}, testRegistrationCooldownConfig())
	svc := &AuthService{settingService: setting}

	_, err := svc.claimRegistrationIPCooldown(context.Background())
	require.ErrorIs(t, err, ErrServiceUnavailable)
}

func TestClaimRegistrationIPCooldownDisabledStatesDoNotTouchBusinessTable(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
		err    error
	}{
		{name: "missing", values: map[string]string{}},
		{name: "false", values: map[string]string{SettingKeyRegistrationIPCooldownEnabled: "false"}},
		{name: "malformed", values: map[string]string{SettingKeyRegistrationIPCooldownEnabled: "TRUE"}},
		{name: "read error", values: map[string]string{}, err: errors.New("settings unavailable")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, db, mock := newRegistrationCooldownSQLClient(t)
			defer db.Close()
			setting := NewSettingService(&settingRepoStub{values: tt.values, err: tt.err}, testRegistrationCooldownConfig())
			svc := &AuthService{entClient: client, settingService: setting}

			claim, err := svc.claimRegistrationIPCooldown(WithAffiliateSignupIP(context.Background(), "203.0.113.20"))
			if tt.name == "read error" {
				require.ErrorIs(t, err, ErrServiceUnavailable)
				require.Nil(t, claim)
			} else {
				require.NoError(t, err)
				require.Nil(t, claim)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestClaimRegistrationIPCooldownReservationDoesNotStartSuccessfulCooldown(t *testing.T) {
	client, db, mock := newRegistrationCooldownSQLClient(t)
	defer db.Close()

	setting := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyRegistrationIPCooldownEnabled: "true",
		SettingKeyRegistrationIPCooldownSeconds: "300",
	}}, testRegistrationCooldownConfig())
	mock.ExpectQuery(`INSERT INTO registration_ip_cooldowns \(ip_hash, reservation_token, reserved_until`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), 300, registrationIPCooldownReservationTTL).
		WillReturnRows(sqlmock.NewRows([]string{"reservation_token"}).AddRow("placeholder"))

	svc := &AuthService{entClient: client, settingService: setting}
	ctx := WithAffiliateSignupIP(context.Background(), "203.0.113.20")
	// The token is generated internally, so the mock deliberately returns a
	// different value. The guarded RETURNING contract must reject that result.
	claim, err := svc.claimRegistrationIPCooldown(ctx)
	require.ErrorIs(t, err, ErrServiceUnavailable)
	require.Nil(t, claim)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRegistrationIPCooldownFinalizeByUserUsesGuardedUpdate(t *testing.T) {
	client, db, mock := newRegistrationCooldownSQLClient(t)
	defer db.Close()

	mock.ExpectExec(`UPDATE registration_ip_cooldowns`).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	svc := &AuthService{entClient: client}
	svc.finalizeRegistrationIPCooldownForUser(context.Background(), 42)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRegistrationIPCooldownReleaseClaimUsesTokenGuard(t *testing.T) {
	client, db, mock := newRegistrationCooldownSQLClient(t)
	defer db.Close()

	mock.ExpectExec(`UPDATE registration_ip_cooldowns`).
		WithArgs("hash", "token").
		WillReturnResult(sqlmock.NewResult(0, 1))

	svc := &AuthService{entClient: client}
	svc.releaseRegistrationIPCooldownClaim(context.Background(), &registrationIPCooldownClaim{
		ipHash: "hash",
		token:  "token",
	})
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRegistrationIPCooldownTokenGuardedUpdatesTreatZeroRowsAsFailure(t *testing.T) {
	client, db, mock := newRegistrationCooldownSQLClient(t)
	defer db.Close()
	svc := &AuthService{entClient: client}
	claim := &registrationIPCooldownClaim{ipHash: "hash", token: "token"}

	mock.ExpectExec(`UPDATE registration_ip_cooldowns`).
		WithArgs("hash", "token", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	require.ErrorIs(t, svc.attachRegistrationIPCooldownClaim(context.Background(), claim, 7), ErrServiceUnavailable)

	mock.ExpectExec(`UPDATE registration_ip_cooldowns`).
		WithArgs("hash", "token", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	require.ErrorIs(t, svc.finalizeRegistrationIPCooldown(context.Background(), claim, 7), ErrServiceUnavailable)

	mock.ExpectExec(`UPDATE registration_ip_cooldowns`).
		WithArgs("hash", "token").
		WillReturnResult(sqlmock.NewResult(0, 0))
	require.ErrorIs(t, svc.releaseRegistrationIPCooldownClaim(context.Background(), claim), ErrServiceUnavailable)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRegistrationIPCooldownUserScopedUpdatesAllowMissingReservation(t *testing.T) {
	client, db, mock := newRegistrationCooldownSQLClient(t)
	defer db.Close()
	svc := &AuthService{entClient: client}

	mock.ExpectExec(`UPDATE registration_ip_cooldowns`).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	require.NoError(t, svc.finalizeRegistrationIPCooldownForUser(context.Background(), 7))

	mock.ExpectExec(`UPDATE registration_ip_cooldowns`).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	require.NoError(t, svc.releaseRegistrationIPCooldownForUser(context.Background(), 7))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRegistrationIPCooldownAttachAndFinalizeClaimUseTokenAndUserGuards(t *testing.T) {
	client, db, mock := newRegistrationCooldownSQLClient(t)
	defer db.Close()

	mock.ExpectExec(`UPDATE registration_ip_cooldowns`).
		WithArgs("hash", "token", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE registration_ip_cooldowns`).
		WithArgs("hash", "token", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	svc := &AuthService{entClient: client}
	claim := &registrationIPCooldownClaim{ipHash: "hash", token: "token"}
	svc.attachRegistrationIPCooldownClaim(context.Background(), claim, 7)
	svc.finalizeRegistrationIPCooldown(context.Background(), claim, 7)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRollbackOAuthEmailAccountCreationReleasesUserReservation(t *testing.T) {
	client, db, mock := newRegistrationCooldownSQLClient(t)
	defer db.Close()

	mock.ExpectExec(`UPDATE registration_ip_cooldowns`).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	settings := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyInvitationCodeEnabled:         "false",
		SettingKeyRegistrationIPCooldownEnabled: "true",
	}}, testRegistrationCooldownConfig())
	userRepo := &userRepoStub{}
	svc := &AuthService{entClient: client, userRepo: userRepo, settingService: settings}

	require.NoError(t, svc.RollbackOAuthEmailAccountCreation(context.Background(), 42, ""))
	require.Equal(t, []int64{42}, userRepo.deletedIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func testRegistrationCooldownConfig() *config.Config {
	return &config.Config{JWT: config.JWTConfig{Secret: "test-secret"}}
}
