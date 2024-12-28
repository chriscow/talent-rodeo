package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"talent-rodeo/cmd/web/templates/pages"

	"github.com/google/uuid"
	"github.com/justinas/nosurf"
	"github.com/nats-io/nats.go/jetstream"
	datastar "github.com/starfederation/datastar/sdk/go"
)

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	token := nosurf.Token(r)
	user := contextGetAuthenticatedUser(r)

	if user != nil {
		app.logger.Debug("home", "user name", user.Name)
	} else {
		app.logger.Warn("home", "auth-state", "no user")
	}

	pages.Index(token, user).Render(r.Context(), w)
	// data := app.newTemplateData(r)

	// err := response.Page(w, http.StatusOK, data, "pages/home.tmpl")
	// if err != nil {
	// 	app.serverError(w, r, err)
	// }
}

func (app *application) watch(w http.ResponseWriter, r *http.Request) {
	sse := datastar.NewSSE(w, r)

	// Watch for updates
	ctx := r.Context()
	kv, err := app.getKV()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	session, err := app.sessionStore.Get(r, "session")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if session.ID == "" {
		session.ID = uuid.New().String()
		err = session.Save(r, w)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	watcher, err := kv.Watch(ctx, session.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case entry := <-watcher.Updates():
			if entry == nil {
				continue
			}
			// if err := json.Unmarshal(entry.Value(), mvc); err != nil {
			// 	http.Error(w, err.Error(), http.StatusInternalServerError)
			// 	return
			// }
			// sse.Send(msg.Value)
			sse.MergeFragmentf(`<div id="kv-container">%s</div>`, entry.Value())
		}
	}
}

func (app *application) getKV() (jetstream.KeyValue, error) {
	nc, err := app.ns.Client()
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
