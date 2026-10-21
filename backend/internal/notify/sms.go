// Package notify envoie les SMS (codes de vérification).
package notify

import (
	"context"
	"log/slog"
)

// SMSSender envoie un SMS. Implémentations réelles possibles : API SMS
// d'Orange Sonatel, Free, Expresso, ou un agrégateur.
type SMSSender interface {
	Send(ctx context.Context, to, message string) error
}

// LogSMS écrit les SMS dans les journaux au lieu de les envoyer.
type LogSMS struct {
	Logger *slog.Logger
}

func (l LogSMS) Send(_ context.Context, to, message string) error {
	l.Logger.Info("SMS (simulé)", "destinataire", to, "message", message)
	return nil
}

// Discard ignore les SMS (tests).
type Discard struct{}

func (Discard) Send(context.Context, string, string) error { return nil }
