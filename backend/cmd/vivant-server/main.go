// Commande vivant-server : fournisseur d'identité Vivant.
//
// Variables d'environnement :
//
//	VIVANT_ADDR          adresse d'écoute (défaut :8080)
//	VIVANT_ISSUER        URL publique du serveur (défaut http://localhost:8080)
//	VIVANT_DATA_DIR      clés et données persistantes (défaut data-local)
//	VIVANT_CLIENTS_FILE  services autorisés (défaut config/clients.json)
//	VIVANT_APP_NAME      nom affiché (défaut Vivant)
//	VIVANT_DEV=1         renvoie les codes SMS dans l'API (développement uniquement)
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"vivant/internal/jwt"
	"vivant/internal/notify"
	"vivant/internal/registry"
	"vivant/internal/server"
	"vivant/internal/store"
	"vivant/internal/verification"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("arrêt du serveur", "err", err)
		os.Exit(1)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func run(logger *slog.Logger) error {
	addr := env("VIVANT_ADDR", ":8080")
	issuer := strings.TrimRight(env("VIVANT_ISSUER", "http://localhost:8080"), "/")
	dataDir := env("VIVANT_DATA_DIR", "data-local")
	devMode := os.Getenv("VIVANT_DEV") == "1"

	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	key, err := jwt.LoadOrGenerateKey(filepath.Join(dataDir, "signing-key.pem"))
	if err != nil {
		return err
	}
	pairwise, err := loadOrCreateSecret(filepath.Join(dataDir, "pairwise.key"))
	if err != nil {
		return err
	}
	clients, err := loadClients(env("VIVANT_CLIENTS_FILE", "config/clients.json"))
	if err != nil {
		return err
	}
	st, err := store.NewMemory(filepath.Join(dataDir, "state.json"))
	if err != nil {
		return err
	}
	reg, err := registry.NewMock()
	if err != nil {
		return err
	}

	srv, err := server.New(server.Config{
		Issuer:         issuer,
		AppName:        env("VIVANT_APP_NAME", "Vivant"),
		DevMode:        devMode,
		Clients:        clients,
		PairwiseSecret: pairwise,
	}, server.Deps{
		Store:    st,
		Registry: reg,
		Faces:    verification.MockFaceVerifier{},
		SMS:      notify.LogSMS{Logger: logger},
		Signer:   jwt.NewSigner(key),
		Logger:   logger,
	})
	if err != nil {
		return err
	}

	logger.Warn("registre national, SMS et vérification faciale SIMULÉS : prototype uniquement")
	if devMode {
		logger.Warn("MODE DÉVELOPPEMENT : les codes SMS sont renvoyés par l'API. Ne jamais activer en production.")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				st.Sweep(now)
			}
		}
	}()

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("Vivant démarré", "adresse", addr, "issuer", issuer, "services", len(clients))
		errCh <- httpSrv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdownCtx)
}

func loadOrCreateSecret(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return hex.DecodeString(strings.TrimSpace(string(data)))
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	return secret, os.WriteFile(path, []byte(hex.EncodeToString(secret)), 0o600)
}

func loadClients(path string) ([]server.Client, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var clients []server.Client
	if err := json.Unmarshal(data, &clients); err != nil {
		return nil, err
	}
	return clients, nil
}
