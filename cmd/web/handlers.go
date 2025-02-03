package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"talent-rodeo/internal/response"

	"github.com/nats-io/nats.go/jetstream"
)

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)

	// If user is authenticated, check onboarding state
	if user := contextGetAuthenticatedUser(r); user != nil {
		state, err := app.db.GetOnboardingState(user.ID)
		if err != nil {
			app.serverError(w, r, fmt.Errorf("failed to get onboarding state: %w", err))
			return
		}

		// Redirect to profile if onboarding is not complete
		if !state.IsComplete() {
			http.Redirect(w, r, "/profile", http.StatusSeeOther)
			return
		}
	}

	if err := response.Page(w, http.StatusOK, data, "pages/dashboard.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getKV() (jetstream.KeyValue, error) {
	nc, err := app.nats.Client()
	if err != nil {
		return nil, fmt.Errorf("error creating nats client: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("error creating jetstream client: %w", err)
	}

	kv, err := js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket:      "todos",
		Description: "Datastar Todos",
		Compression: true,
		TTL:         time.Hour,
		MaxBytes:    16 * 1024 * 1024,
	})

	if err != nil {
		return nil, fmt.Errorf("error creating key value: %w", err)
	}

	return kv, nil
}
