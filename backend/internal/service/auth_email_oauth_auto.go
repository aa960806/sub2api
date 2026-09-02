package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

type EmailOAuthIdentityInput struct {
	ProviderType     string
	ProviderKey      string
	ProviderSubject  string
	Email            string
	EmailVerified    bool
	Username         string
	DisplayName      string
	AvatarURL        string
	UpstreamMetadata map[string]any
}

func (s *AuthService) LoginOrRegisterVerifiedEmailOAuth(ctx context.Context, input EmailOAuthIdentityInput) (*TokenPair, *User, error) {
	return s.loginOrRegisterVerifiedEmailOAuth(ctx, input, "", "", "")
}

func (s *AuthService) LoginOrRegisterVerifiedEmailOAuthWithInvitation(
	ctx context.Context,
	input EmailOAuthIdentityInput,
	invitationCode string,
	affiliateCode string,
) (*TokenPair, *User, error) {
	return s.loginOrRegisterVerifiedEmailOAuth(ctx, input, invitationCode, affiliateCode, "")
}

func (s *AuthService) LoginOrRegisterVerifiedEmailOAuthWithSignupCodes(
	ctx context.Context,
	input EmailOAuthIdentityInput,
	invitationCode string,
	affiliateCode string,
	promoCode string,
) (*TokenPair, *User, error) {
	return s.loginOrRegisterVerifiedEmailOAuth(ctx, input, invitationCode, affiliateCode, promoCode)
}

func (s *AuthService) loginOrRegisterVerifiedEmailOAuth(
	ctx context.Context,
	input EmailOAuthIdentityInput,
	invitationCode string,
	affiliateCode string,
	promoCode string,
) (*TokenPair, *User, error) {
	if s == nil || s.userRepo == nil || s.entClient == nil {
		return nil, nil, ErrServiceUnavailable
	}

	providerType := normalizeOAuthSignupSource(input.ProviderType)
	if providerType != "github" && providerType != "google" && providerType != "oidc" {
		return nil, nil, infraerrors.BadRequest("OAUTH_PROVIDER_INVALID", "oauth provider is invalid")
	}
	providerKey := strings.TrimSpace(input.ProviderKey)
	if providerKey == "" {
		providerKey = providerType
	}
	providerSubject := strings.TrimSpace(input.ProviderSubject)
	if providerSubject == "" {
		return nil, nil, infraerrors.BadRequest("OAUTH_SUBJECT_MISSING", "oauth subject is missing")
	}
	if !input.EmailVerified {
		return nil, nil, infraerrors.Forbidden("OAUTH_EMAIL_NOT_VERIFIED", "oauth email is not verified")
	}

	email := strings.TrimSpace(strings.ToLower(input.Email))
	if email == "" || len(email) > 255 {
		return nil, nil, infraerrors.BadRequest("INVALID_EMAIL", "invalid email")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, nil, infraerrors.BadRequest("INVALID_EMAIL", "invalid email")
	}
	if isReservedEmail(email) {
		return nil, nil, ErrEmailReserved
	}
	if err := s.validateRegistrationEmailPolicy(ctx, email); err != nil {
		return nil, nil, err
	}

	identityUser, err := s.findEmailOAuthIdentityOwner(ctx, providerType, providerKey, providerSubject)
	if err != nil {
		return nil, nil, err
	}
	if identityUser != nil && !strings.EqualFold(strings.TrimSpace(identityUser.Email), email) {
		return nil, nil, infraerrors.Conflict("AUTH_IDENTITY_EMAIL_MISMATCH", "oauth identity belongs to a different email")
	}

	user := identityUser
	created := false
	if user == nil {
		user, err = s.userRepo.GetByEmail(ctx, email)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				user, created, err = s.createEmailOAuthUserWithStatus(ctx, email, input.Username, providerType, invitationCode, affiliateCode)
				if err != nil {
					return nil, nil, err
				}
			} else {
				logger.LegacyPrintf("service.auth", "[Auth] Database error during %s oauth login: %v", providerType, err)
				return nil, nil, ErrServiceUnavailable
			}
		}
	}

	if !user.IsActive() {
		return nil, nil, ErrUserNotActive
	}
	if err := s.ensureEmailOAuthIdentity(ctx, user.ID, EmailOAuthIdentityInput{
		ProviderType:     providerType,
		ProviderKey:      providerKey,
		ProviderSubject:  providerSubject,
		Email:            email,
		EmailVerified:    input.EmailVerified,
		Username:         input.Username,
		DisplayName:      input.DisplayName,
		AvatarURL:        input.AvatarURL,
		UpstreamMetadata: input.UpstreamMetadata,
	}); err != nil {
		return nil, nil, err
	}

	if user.Username == "" && strings.TrimSpace(input.Username) != "" {
		user.Username = strings.TrimSpace(input.Username)
		if err := s.userRepo.Update(ctx, user, UserUpdateFields{Username: true}); err != nil {
			logger.LegacyPrintf("service.auth", "[Auth] Failed to update username after %s oauth login: %v", providerType, err)
		}
	}
	if !created {
		if err := s.ApplyProviderDefaultSettingsOnFirstBind(ctx, user.ID, providerType); err != nil {
			logger.LegacyPrintf("service.auth", "[Auth] Failed to apply %s first bind defaults: %v", providerType, err)
		}
	} else {
		user = s.applyOAuthSignupPromoCode(ctx, user, promoCode)
	}
	s.RecordSuccessfulLogin(ctx, user.ID)

	tokenPair, err := s.GenerateTokenPair(ctx, user, "")
	if err != nil {
		return nil, nil, fmt.Errorf("generate token pair: %w", err)
	}
	return tokenPair, user, nil
}

// createEmailOAuthUser keeps the historical helper contract for callers that
// only need the resulting user. The status-aware implementation is used by
// the login flow so a create race cannot be mistaken for a newly-created
// account and subsequently deleted during cleanup.
func (s *AuthService) createEmailOAuthUser(ctx context.Context, email, username, providerType, invitationCode, affiliateCode string) (*User, error) {
	user, _, err := s.createEmailOAuthUserWithStatus(ctx, email, username, providerType, invitationCode, affiliateCode)
	return user, err
}

// createEmailOAuthUserWithStatus returns whether this invocation inserted the
// user. When another request wins the email uniqueness race, it returns the
// existing user with created=false after releasing this request's cooldown
// reservation.
func (s *AuthService) createEmailOAuthUserWithStatus(ctx context.Context, email, username, providerType, invitationCode, affiliateCode string) (*User, bool, error) {
	if s.settingService == nil || !s.settingService.IsRegistrationEnabled(ctx) {
		return nil, false, ErrRegDisabled
	}
	invitationRedeemCode, err := s.validateOAuthRegistrationInvitation(ctx, invitationCode)
	if err != nil {
		if errors.Is(err, ErrInvitationCodeRequired) {
			return nil, false, ErrOAuthInvitationRequired
		}
		return nil, false, err
	}

	registrationClaim, err := s.claimRegistrationIPCooldown(ctx)
	if err != nil {
		return nil, false, err
	}
	registrationFinalized := false
	defer func() {
		if registrationClaim != nil && !registrationFinalized {
			s.releaseRegistrationIPCooldownClaimBestEffort(ctx, registrationClaim, "verified OAuth auto registration")
		}
	}()

	randomPassword, err := randomHexString(32)
	if err != nil {
		return nil, false, ErrServiceUnavailable
	}
	hashedPassword, err := s.HashPassword(randomPassword)
	if err != nil {
		return nil, false, fmt.Errorf("hash password: %w", err)
	}
	grantPlan := s.resolveSignupGrantPlan(ctx, providerType)
	var defaultRPMLimit int
	if s.settingService != nil {
		defaultRPMLimit = s.settingService.GetDefaultUserRPMLimit(ctx)
	}
	user := &User{
		Email:        email,
		Username:     strings.TrimSpace(username),
		PasswordHash: hashedPassword,
		Role:         RoleUser,
		Balance:      grantPlan.Balance,
		Concurrency:  grantPlan.Concurrency,
		RPMLimit:     defaultRPMLimit,
		Status:       StatusActive,
		SignupSource: providerType,
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		if errors.Is(err, ErrEmailExists) {
			// Another request won the create race; this request did not create a
			// user and must not consume the IP cooldown reservation.
			releaseErr := s.releaseRegistrationIPCooldownClaim(ctx, registrationClaim)
			registrationClaim = nil
			if releaseErr != nil {
				return nil, false, wrapRegistrationCleanupError("release registration IP cooldown claim after create race", releaseErr)
			}
			existing, loadErr := s.userRepo.GetByEmail(ctx, email)
			if loadErr != nil {
				return nil, false, ErrServiceUnavailable
			}
			if existing == nil {
				return nil, false, ErrServiceUnavailable
			}
			return existing, false, nil
		}
		return nil, false, ErrServiceUnavailable
	}

	if err := s.attachRegistrationIPCooldownClaim(ctx, registrationClaim, user.ID); err != nil {
		cleanupErr := s.rollbackRegistrationAccountCreation(ctx, user.ID, invitationCode, registrationClaim, false, wrapRegistrationCleanupError("attach registration IP cooldown claim", err))
		registrationClaim = nil
		return nil, false, cleanupErr
	}

	s.postAuthUserBootstrap(ctx, user, providerType, false)
	s.assignSubscriptions(ctx, user.ID, grantPlan.Subscriptions, "auto assigned by signup defaults")
	// snapshot user × platform quota（fail-open）
	_ = s.snapshotPlatformQuotaDefaults(ctx, user.ID, &grantPlan)
	s.bindOAuthAffiliate(ctx, user.ID, affiliateCode)
	if invitationRedeemCode != nil {
		if err := s.useOAuthRegistrationInvitation(ctx, invitationRedeemCode.ID, user.ID); err != nil {
			cleanupErr := s.rollbackRegistrationAccountCreation(ctx, user.ID, invitationCode, registrationClaim, true, ErrInvitationCodeInvalid)
			registrationClaim = nil
			return nil, false, cleanupErr
		}
	}
	if err := s.finalizeRegistrationIPCooldown(ctx, registrationClaim, user.ID); err != nil {
		cleanupErr := s.rollbackRegistrationAccountCreation(ctx, user.ID, invitationCode, registrationClaim, true, wrapRegistrationCleanupError("finalize registration IP cooldown", err))
		registrationClaim = nil
		return nil, false, cleanupErr
	}
	registrationFinalized = registrationClaim != nil
	return user, true, nil
}

func (s *AuthService) findEmailOAuthIdentityOwner(ctx context.Context, providerType, providerKey, providerSubject string) (*User, error) {
	identity, err := s.entClient.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ(providerType),
			authidentity.ProviderKeyEQ(providerKey),
			authidentity.ProviderSubjectEQ(providerSubject),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, infraerrors.InternalServer("AUTH_IDENTITY_LOOKUP_FAILED", "failed to inspect auth identity ownership").WithCause(err)
	}
	user, err := s.userRepo.GetByID(ctx, identity.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, nil
		}
		return nil, ErrServiceUnavailable
	}
	return user, nil
}

func (s *AuthService) ensureEmailOAuthIdentity(ctx context.Context, userID int64, input EmailOAuthIdentityInput) error {
	metadata := map[string]any{
		"email":          strings.TrimSpace(strings.ToLower(input.Email)),
		"email_verified": input.EmailVerified,
	}
	for key, value := range input.UpstreamMetadata {
		metadata[key] = value
	}
	if strings.TrimSpace(input.Username) != "" {
		metadata["username"] = strings.TrimSpace(input.Username)
	}
	if strings.TrimSpace(input.DisplayName) != "" {
		metadata["display_name"] = strings.TrimSpace(input.DisplayName)
	}
	if strings.TrimSpace(input.AvatarURL) != "" {
		metadata["avatar_url"] = strings.TrimSpace(input.AvatarURL)
	}

	providerType := normalizeOAuthSignupSource(input.ProviderType)
	providerKey := strings.TrimSpace(input.ProviderKey)
	providerSubject := strings.TrimSpace(input.ProviderSubject)
	identity, err := s.entClient.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ(providerType),
			authidentity.ProviderKeyEQ(providerKey),
			authidentity.ProviderSubjectEQ(providerSubject),
		).
		Only(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return infraerrors.InternalServer("AUTH_IDENTITY_LOOKUP_FAILED", "failed to inspect auth identity ownership").WithCause(err)
	}
	if identity != nil {
		if identity.UserID != userID {
			return infraerrors.Conflict("AUTH_IDENTITY_OWNERSHIP_CONFLICT", "auth identity already belongs to another user")
		}
		_, err = s.entClient.AuthIdentity.UpdateOneID(identity.ID).
			SetMetadata(metadata).
			Save(ctx)
		return err
	}
	_, err = s.entClient.AuthIdentity.Create().
		SetUserID(userID).
		SetProviderType(providerType).
		SetProviderKey(providerKey).
		SetProviderSubject(providerSubject).
		SetMetadata(metadata).
		Save(ctx)
	if err == nil {
		return nil
	}
	if !dbent.IsConstraintError(err) {
		return infraerrors.InternalServer("AUTH_IDENTITY_SAVE_FAILED", "failed to save auth identity").WithCause(err)
	}

	// Two first-time OAuth requests can both observe no identity and race on
	// the unique provider tuple. Re-read after a uniqueness conflict. If the
	// winner belongs to this same user, the operation is idempotently complete;
	// only a different owner is a real security conflict. This prevents the
	// caller's compensating account cleanup from deleting an account that a
	// concurrent request has already bound successfully.
	winner, lookupErr := s.entClient.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ(providerType),
			authidentity.ProviderKeyEQ(providerKey),
			authidentity.ProviderSubjectEQ(providerSubject),
		).
		Only(ctx)
	if lookupErr != nil {
		return infraerrors.InternalServer("AUTH_IDENTITY_LOOKUP_FAILED", "failed to resolve concurrent auth identity creation").WithCause(lookupErr)
	}
	if winner.UserID != userID {
		return infraerrors.Conflict("AUTH_IDENTITY_OWNERSHIP_CONFLICT", "auth identity already belongs to another user")
	}
	if _, updateErr := s.entClient.AuthIdentity.UpdateOneID(winner.ID).
		SetMetadata(metadata).
		Save(ctx); updateErr != nil {
		return infraerrors.InternalServer("AUTH_IDENTITY_SAVE_FAILED", "failed to refresh auth identity").WithCause(updateErr)
	}
	return nil
}
