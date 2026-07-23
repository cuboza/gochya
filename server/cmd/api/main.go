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

	"github.com/gochya/gochya/server/internal/auth"
	"github.com/gochya/gochya/server/internal/corebridge"
	"github.com/gochya/gochya/server/internal/dojo"
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
	authAPI, err := auth.NewHTTPHandlerWithGoogle(
		sessions,
		googleExchange,
		nil,
	)
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
