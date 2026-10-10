package main

import (
	"context"
	"encoding/json"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"log/slog"
	"net/http"
	"os"
	"time"

	"talent-rodeo/internal/inbox"
)

type HealthStatus struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Services  map[string]string `json:"services"`
	Version   string            `json:"version,omitempty"`
}

// health returns a basic health check
func (app *application) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	status := HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Services: map[string]string{
			"database": "healthy",
		},
	}

	// Check database connection
	if app.fs != nil {
		// Simple ping to Firestore
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		// Try to access a collection (this will verify connection)
		_, err := app.fs.Collection("health_check").Doc("ping").Get(ctx)
		if err != nil {
			// Document not found is OK - means connection works
			if grpcstatus.Code(err) != codes.NotFound {
				status.Services["database"] = "unhealthy: " + err.Error()
				status.Status = "degraded"
			}
		}
	} else {
		status.Services["database"] = "unhealthy: client not initialized"
		status.Status = "degraded"
	}

	if status.Status == "healthy" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(status)
}

// healthDetailed returns detailed health check including Gmail
func (app *application) healthDetailed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	status := HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Services: map[string]string{
			"database": "healthy",
			"gmail":    "not_configured",
		},
	}

	// Check database connection (same as basic health)
	if app.fs != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		_, err := app.fs.Collection("health_check").Doc("ping").Get(ctx)
		if err != nil {
			if grpcstatus.Code(err) != codes.NotFound {
				status.Services["database"] = "unhealthy: " + err.Error()
				status.Status = "degraded"
			}
		}
	} else {
		status.Services["database"] = "unhealthy: client not initialized"
		status.Status = "degraded"
	}

	// Check Gmail connection
	if app.mode.Personal {
		status.Services["gmail"] = "disabled"
	} else if os.Getenv("GMAIL_READER_CREDENTIALS") != "" {
		status.Services["gmail"] = app.checkGmailHealth(r.Context())
		if status.Services["gmail"] != "healthy" && status.Status == "healthy" {
			status.Status = "degraded"
		}
	}

	// Set overall status
	if status.Status == "healthy" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(status)
}

// checkGmailHealth verifies Gmail connectivity
func (app *application) checkGmailHealth(ctx context.Context) string {
	// Create a silent logger for health checks
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError, // Only log errors
	}))

	config := inbox.InboxConfig{
		TokenPath: getGmailTokenPath(),
	}

	// Set a short timeout for health checks
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	reader, err := inbox.NewGmailReader(config, logger)
	if err != nil {
		if err.Error() == "no existing token found, requesting new authorization" {
			return "not_authorized"
		}
		return "unhealthy: " + err.Error()
	}
	defer reader.Close()

	// Try to fetch 1 email as a connectivity test
	_, err = reader.GetUnreadEmails(ctx, 1)
	if err != nil {
		return "unhealthy: " + err.Error()
	}

	return "healthy"
}

// gmailHealth is a dedicated Gmail health check endpoint
func (app *application) gmailHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	gmailStatus := map[string]interface{}{
		"service":   "gmail",
		"timestamp": time.Now(),
	}

	if os.Getenv("GMAIL_READER_CREDENTIALS") == "" {
		gmailStatus["status"] = "not_configured"
		gmailStatus["message"] = "GMAIL_READER_CREDENTIALS not set"
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		status := app.checkGmailHealth(r.Context())
		gmailStatus["status"] = status

		if status == "healthy" {
			gmailStatus["message"] = "Gmail connection successful"
			w.WriteHeader(http.StatusOK)
		} else if status == "not_authorized" {
			gmailStatus["message"] = "Gmail OAuth not authorized - run gmail setup"
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			gmailStatus["message"] = "Gmail connection failed"
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}

	json.NewEncoder(w).Encode(gmailStatus)
}
