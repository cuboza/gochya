package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gochya/gochya/server/internal/activity"
	"github.com/gochya/gochya/server/internal/auth"
	"github.com/gochya/gochya/server/internal/battle"
	"github.com/gochya/gochya/server/internal/breeding"
	"github.com/gochya/gochya/server/internal/corebridge"
	"github.com/gochya/gochya/server/internal/device"
	"github.com/gochya/gochya/server/internal/dojo"
	"github.com/gochya/gochya/server/internal/inventory"
	"github.com/gochya/gochya/server/internal/profile"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	startupTimeout  = 10 * time.Second
	shutdownTimeout = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("API stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	config, err := loadAPIConfig(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	startupCtx, cancelStartup := context.WithTimeout(
		context.Background(),
		startupTimeout,
	)
	defer cancelStartup()
	poolConfig, err := pgxpool.ParseConfig(config.DatabaseURL)
	if err != nil {
		return fmt.Errorf("parse PostgreSQL configuration: %w", err)
	}
	poolConfig.ConnConfig.RuntimeParams["application_name"] = "gochya-api"
	pool, err := pgxpool.NewWithConfig(startupCtx, poolConfig)
	if err != nil {
		return fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(startupCtx); err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	if err := validateDatabaseSchema(startupCtx, pool); err != nil {
		return fmt.Errorf("validate PostgreSQL schema: %w", err)
	}

	core := corebridge.NativeEngine{}
	if _, err := core.ValidateHeart(startupCtx, corebridge.HeartEvidence{}); err != nil {
		return fmt.Errorf("probe native Gochya Core: %w", err)
	}
	store, err := dojo.NewPostgresStore(dojo.PostgresStoreConfig{Pool: pool})
	if err != nil {
		return fmt.Errorf("create Dojo store: %w", err)
	}
	authenticator, err := dojo.NewJWTAuthenticator(dojo.JWTAuthenticatorConfig{
		Issuer:     config.JWTIssuer,
		Audience:   config.JWTAudience,
		PublicKeys: config.JWTPublicKeys,
	})
	if err != nil {
		return fmt.Errorf("create JWT authenticator: %w", err)
	}
	refreshStore, err := auth.NewPostgresRefreshTokenStore(pool)
	if err != nil {
		return fmt.Errorf("create refresh-token store: %w", err)
	}
	identityStore, err := auth.NewPostgresIdentityStore(pool)
	if err != nil {
		return fmt.Errorf("create identity store: %w", err)
	}
	loginNonceStore, err := auth.NewPostgresLoginNonceStore(pool)
	if err != nil {
		return fmt.Errorf("create login-nonce store: %w", err)
	}
	sessions, err := auth.NewService(auth.ServiceConfig{
		Store:      refreshStore,
		KeyID:      config.JWTSigningKeyID,
		PrivateKey: config.JWTSigningPrivateKey,
		Issuer:     config.JWTIssuer,
		Audience:   config.JWTAudience,
	})
	if err != nil {
		return fmt.Errorf("create session service: %w", err)
	}
	googleTokenValidator, err := auth.NewGoogleAPIIDTokenValidator(nil)
	if err != nil {
		return fmt.Errorf("create Google token validator: %w", err)
	}
	googleVerifier, err := auth.NewGoogleVerifier(auth.GoogleVerifierConfig{
		Validator: googleTokenValidator,
		Audiences: config.GoogleClientIDs,
	})
	if err != nil {
		return fmt.Errorf("create Google identity verifier: %w", err)
	}
	googleExchange, err := auth.NewGoogleExchangeService(
		auth.GoogleExchangeServiceConfig{
			Verifier: googleVerifier,
			Players:  identityStore,
			Sessions: sessions,
		},
	)
	if err != nil {
		return fmt.Errorf("create Google exchange: %w", err)
	}
	appleKeys, err := auth.NewAppleHTTPKeySet(auth.AppleHTTPKeySetConfig{})
	if err != nil {
		return fmt.Errorf("create Apple signing-key cache: %w", err)
	}
	appleVerifier, err := auth.NewAppleVerifier(auth.AppleVerifierConfig{
		Keys:      appleKeys,
		Audiences: config.AppleClientIDs,
	})
	if err != nil {
		return fmt.Errorf("create Apple identity verifier: %w", err)
	}
	appleExchange, err := auth.NewAppleExchangeService(
		auth.AppleExchangeServiceConfig{
			Verifier: appleVerifier,
			Nonces:   loginNonceStore,
			Players:  identityStore,
			Sessions: sessions,
		},
	)
	if err != nil {
		return fmt.Errorf("create Apple exchange: %w", err)
	}
	samsungKeys, err := auth.NewSamsungHTTPKeySet(auth.HTTPRSAKeySetConfig{})
	if err != nil {
		return fmt.Errorf("create Samsung signing-key cache: %w", err)
	}
	samsungTokenClient, err := auth.NewSamsungOIDCTokenClient(
		auth.SamsungOIDCTokenClientConfig{
			ClientID:     config.SamsungOIDCClientID,
			ClientSecret: config.SamsungOIDCClientSecret,
		},
	)
	if err != nil {
		return fmt.Errorf("create Samsung OIDC token client: %w", err)
	}
	samsungVerifier, err := auth.NewSamsungVerifier(
		auth.SamsungVerifierConfig{
			Keys:     samsungKeys,
			ClientID: config.SamsungOIDCClientID,
		},
	)
	if err != nil {
		return fmt.Errorf("create Samsung identity verifier: %w", err)
	}
	samsungExchange, err := auth.NewSamsungExchangeService(
		auth.SamsungExchangeServiceConfig{
			ClientID:     config.SamsungOIDCClientID,
			RedirectURIs: config.SamsungRedirectURIs,
			Tokens:       samsungTokenClient,
			Verifier:     samsungVerifier,
			Nonces:       loginNonceStore,
			Players:      identityStore,
			Sessions:     sessions,
		},
	)
	if err != nil {
		return fmt.Errorf("create Samsung exchange: %w", err)
	}
	authAPI, err := auth.NewHTTPHandlerWithProviders(auth.HTTPHandlerConfig{
		Sessions: sessions,
		Google:   googleExchange,
		Apple:    appleExchange,
		Samsung:  samsungExchange,
	})
	if err != nil {
		return fmt.Errorf("create auth HTTP API: %w", err)
	}
	accessTokens, err := dojo.NewGoogleDefaultPlayIntegrityAccessTokenSource(
		context.Background(),
	)
	if err != nil {
		return fmt.Errorf("create Play Integrity credentials: %w", err)
	}
	playDecoder, err := dojo.NewHTTPPlayIntegrityDecoder(
		dojo.HTTPPlayIntegrityDecoderConfig{AccessTokens: accessTokens},
	)
	if err != nil {
		return fmt.Errorf("create Play Integrity decoder: %w", err)
	}
	attestation, err := dojo.NewPlayIntegrityVerifier(
		dojo.PlayIntegrityVerifierConfig{
			Decoder:                  playDecoder,
			PackageName:              config.PlayPackageName,
			CertificateSHA256Digests: config.PlayCertificateDigests,
			RequiredDeviceVerdicts:   config.PlayRequiredVerdicts,
			AllowUnlicensed:          config.PlayAllowUnlicensed,
			AllowTestingResponses:    config.PlayAllowTestResponses,
		},
	)
	if err != nil {
		return fmt.Errorf("create Play Integrity verifier: %w", err)
	}
	deviceStore, err := device.NewPostgresStore(pool)
	if err != nil {
		return fmt.Errorf("create device enrollment store: %w", err)
	}
	deviceService, err := device.NewService(device.ServiceConfig{
		Store:            deviceStore,
		Attestation:      attestation,
		AllowedAppBuilds: config.AllowedAppBuilds,
	})
	if err != nil {
		return fmt.Errorf("create device enrollment service: %w", err)
	}
	deviceAPI, err := device.NewHTTPHandler(deviceService, authenticator, nil)
	if err != nil {
		return fmt.Errorf("create device enrollment HTTP API: %w", err)
	}
	inventoryStore, err := inventory.NewPostgresStore(pool)
	if err != nil {
		return fmt.Errorf("create inventory store: %w", err)
	}
	inventoryService, err := inventory.NewService(inventoryStore)
	if err != nil {
		return fmt.Errorf("create inventory service: %w", err)
	}
	inventoryAPI, err := inventory.NewHTTPHandler(inventoryService, authenticator, nil)
	if err != nil {
		return fmt.Errorf("create inventory HTTP API: %w", err)
	}
	profileStore, err := profile.NewPostgresStore(pool)
	if err != nil {
		return fmt.Errorf("create profile store: %w", err)
	}
	profileService, err := profile.NewService(profile.ServiceConfig{
		Store: profileStore,
	})
	if err != nil {
		return fmt.Errorf("create profile service: %w", err)
	}
	profileAPI, err := profile.NewHTTPHandler(profileService, authenticator, nil)
	if err != nil {
		return fmt.Errorf("create profile HTTP API: %w", err)
	}
	battleStore, err := battle.NewPostgresStore(pool)
	if err != nil {
		return fmt.Errorf("create battle store: %w", err)
	}
	battleService, err := battle.NewService(battle.ServiceConfig{
		Store: battleStore,
		Core:  core,
	})
	if err != nil {
		return fmt.Errorf("create battle service: %w", err)
	}
	battleAPI, err := battle.NewHTTPHandler(battleService, authenticator, nil)
	if err != nil {
		return fmt.Errorf("create battle HTTP API: %w", err)
	}
	activityStore, err := activity.NewPostgresStore(pool)
	if err != nil {
		return fmt.Errorf("create activity store: %w", err)
	}
	activityService, err := activity.NewService(activity.ServiceConfig{
		Store: activityStore,
		Core:  core,
	})
	if err != nil {
		return fmt.Errorf("create activity service: %w", err)
	}
	activityAPI, err := activity.NewHTTPHandler(
		activityService,
		authenticator,
		nil,
	)
	if err != nil {
		return fmt.Errorf("create activity HTTP API: %w", err)
	}
	breedingStore, err := breeding.NewPostgresStore(pool)
	if err != nil {
		return fmt.Errorf("create breeding store: %w", err)
	}
	breedingService, err := breeding.NewService(breeding.ServiceConfig{
		Store: breedingStore,
		Core:  core,
	})
	if err != nil {
		return fmt.Errorf("create breeding service: %w", err)
	}
	breedingAPI, err := breeding.NewHTTPHandler(
		breedingService,
		authenticator,
		nil,
	)
	if err != nil {
		return fmt.Errorf("create breeding HTTP API: %w", err)
	}
	service, err := dojo.NewService(dojo.ServiceConfig{
		Store:                     store,
		Core:                      core,
		Attestation:               attestation,
		AllowedAppBuilds:          config.AllowedAppBuilds,
		AllowedClassifierVersions: config.AllowedClassifierBuilds,
	})
	if err != nil {
		return fmt.Errorf("create Dojo service: %w", err)
	}
	api, err := dojo.NewHTTPHandler(service, authenticator, nil)
	if err != nil {
		return fmt.Errorf("create HTTP API: %w", err)
	}
	application := http.NewServeMux()
	application.Handle("/v1/auth/", authAPI.Routes())
	application.Handle("/v1/devices/", deviceAPI.Routes())
	inventoryRoutes := inventoryAPI.Routes()
	application.Handle("/v1/me/techniques", inventoryRoutes)
	application.Handle("/v1/me/techniques/equip", inventoryRoutes)
	application.Handle("/v1/me/loadout", inventoryRoutes)
	profileRoutes := profileAPI.Routes()
	application.Handle("/v1/me", profileRoutes)
	application.Handle("/v1/me/pets", profileRoutes)
	application.Handle("/v1/me/pets/", profileRoutes)
	battleRoutes := battleAPI.Routes()
	application.Handle("/v1/matchmaking/queue", battleRoutes)
	application.Handle("/v1/match/", battleRoutes)
	application.Handle("/v1/me/matches/history", battleRoutes)
	activityRoutes := activityAPI.Routes()
	application.Handle("/v1/sync/activity", activityRoutes)
	application.Handle("/v1/me/activity/week", activityRoutes)
	application.Handle("/v1/me/activity/reward", activityRoutes)
	breedingRoutes := breedingAPI.Routes()
	application.Handle("/v1/breeding/breed", breedingRoutes)
	application.Handle("/v1/me/eggs", breedingRoutes)
	application.Handle("/v1/me/eggs/", breedingRoutes)
	application.Handle("/", api.Routes())

	server := &http.Server{
		Addr:              config.HTTPAddress,
		Handler:           newRootHandler(application, pool),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	signalCtx, stopSignals := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopSignals()
	serverError := make(chan error, 1)
	go func() {
		serverError <- server.ListenAndServe()
	}()
	slog.Info("API listening", "address", config.HTTPAddress)

	select {
	case err := <-serverError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-signalCtx.Done():
		slog.Info("API shutting down")
		shutdownCtx, cancelShutdown := context.WithTimeout(
			context.Background(),
			shutdownTimeout,
		)
		defer cancelShutdown()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		err := <-serverError
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP during shutdown: %w", err)
		}
		return nil
	}
}
