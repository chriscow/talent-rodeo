package internal

import (
	"log/slog"
	"talent-rodeo/internal/smtp"

	"github.com/getsentry/sentry-go"
	"github.com/pusher/pusher-http-go/v5"
)

// ServiceDeps contains all dependencies required for post-processing completion flows
// This struct is used to avoid global state and argument bloat when calling
// transcript.ProcessCompletedCall from both chat and phone call completion flows
type ServiceDeps struct {
	// Core dependencies
	Hub    *sentry.Hub
	FS     *FirestoreClient
	Pusher *pusher.Client
	Logger *slog.Logger

	// Background processing dependencies
	Queue    SearchQueue
	Dispatch *JobEventDispatch

	// Email configuration
	SMTPConfig *smtp.Config
}

// NewServiceDeps creates a new ServiceDeps instance with all required dependencies
func NewServiceDeps(
	hub *sentry.Hub,
	fs *FirestoreClient,
	pusher *pusher.Client,
	logger *slog.Logger,
	queue SearchQueue,
	dispatch *JobEventDispatch,
	smtpConfig *smtp.Config,
) *ServiceDeps {
	return &ServiceDeps{
		Hub:        hub,
		FS:         fs,
		Pusher:     pusher,
		Logger:     logger,
		Queue:      queue,
		Dispatch:   dispatch,
		SMTPConfig: smtpConfig,
	}
}