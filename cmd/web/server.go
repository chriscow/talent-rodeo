package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"talent-rodeo/internal"
	"time"

	"github.com/getsentry/sentry-go"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

const (
	defaultIdleTimeout    = time.Minute
	defaultReadTimeout    = 5 * time.Second
	defaultWriteTimeout   = 10 * time.Second
	defaultShutdownPeriod = 30 * time.Second
)

const (
	letsEncryptStagingCA    = "https://acme-staging-v02.api.letsencrypt.org/directory"
	letsEncryptProductionCA = "https://acme-v02.api.letsencrypt.org/directory"
)

func (app *application) serveHTTP() error {
	app.logger.Info(internal.BGGreen("Starting HTTP server"), "port", app.config.httpPort)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", app.config.httpPort),
		Handler:      app.routes(),
		ErrorLog:     slog.NewLogLogger(app.logger.Handler(), slog.LevelWarn),
		IdleTimeout:  defaultIdleTimeout,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
	}

	err := app.serve(srv)
	if err != nil {
		internal.SentryReportError(context.Background(), err, sentry.LevelError, map[string]string{
			"action": "failed_to_serve_http",
		})
		return fmt.Errorf("app.serve(srv): %w", err)
	}

	app.wg.Wait()
	return nil
}

func (app *application) serveAutoHTTPS() error {
	if app.config.autoHTTPS.domain == "localhost" || strings.HasPrefix(app.config.autoHTTPS.domain, "localhost:") {
		return errors.New("auto HTTPS domain must be publicly accessible (not localhost)")
	}

	var directoryURL string

	if app.config.autoHTTPS.staging {
		directoryURL = letsEncryptStagingCA
	} else {
		directoryURL = letsEncryptProductionCA
	}

	certManager := autocert.Manager{
		Email:      app.config.autoHTTPS.email,
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache("./certs"),
		HostPolicy: autocert.HostWhitelist(app.config.autoHTTPS.domain),
		Client: &acme.Client{
			DirectoryURL: directoryURL,
		},
	}

	serverErrorChan := make(chan error)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()

		tlsConfig := certManager.TLSConfig()
		tlsConfig.MinVersion = tls.VersionTLS12
		tlsConfig.CurvePreferences = []tls.CurveID{tls.X25519, tls.CurveP256}

		srv := &http.Server{
			Addr:         ":443",
			Handler:      app.routes(),
			ErrorLog:     slog.NewLogLogger(app.logger.Handler(), slog.LevelWarn),
			IdleTimeout:  defaultIdleTimeout,
			ReadTimeout:  defaultReadTimeout,
			WriteTimeout: defaultWriteTimeout,
			TLSConfig:    tlsConfig,
		}

		serverErrorChan <- app.serve(srv)
	}()

	go func() {
		defer wg.Done()

		srv := &http.Server{
			Addr:         ":80",
			Handler:      certManager.HTTPHandler(nil),
			ErrorLog:     slog.NewLogLogger(app.logger.Handler(), slog.LevelWarn),
			IdleTimeout:  defaultIdleTimeout,
			ReadTimeout:  defaultReadTimeout,
			WriteTimeout: defaultWriteTimeout,
		}

		serverErrorChan <- app.serve(srv)
	}()

	go func() {
		wg.Wait()
		close(serverErrorChan)
	}()

	for err := range serverErrorChan {
		if err != nil {
			internal.SentryReportError(context.Background(), err, sentry.LevelError, map[string]string{
				"action": "failed_to_serve_http",
			})
			return err
		}
	}

	app.wg.Wait()
	return nil
}

func (app *application) serve(srv *http.Server) error {
	shutdownErrorChan := make(chan error)
	serverErrorChan := make(chan error, 1)

	go func() {
		app.logger.Info("app: starting app server", slog.Group("server", "addr", srv.Addr))
		if srv.TLSConfig != nil {
			serverErrorChan <- srv.ListenAndServeTLS("", "")
		} else {
			serverErrorChan <- srv.ListenAndServe()
		}
	}()

	go func() {
		quitChan := make(chan os.Signal, 1)
		signal.Notify(quitChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-quitChan
		app.logger.Info("app: received shutdown signal", "signal", sig.String())

		// First, initiate server shutdown
		ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownPeriod)
		defer cancel()

		app.logger.Info("app: attempting graceful shutdown")

		// Stop the search service first
		app.logger.Info("app: stopping search service")
		if err := app.searchScheduler.Close(ctx); err != nil {
			app.logger.Error("error stopping search scheduler", "error", err)
		}
		if err := app.searchProvider.Close(ctx); err != nil {
			app.logger.Error("error stopping search provider", "error", err)
		}
		app.logger.Info("app: search service stopped")

		app.logger.Info("app: stopping search result processor")
		app.searchResultProcessor.Stop()
		app.logger.Info("app: search result processor stopped")

		// Stop the task processor first
		app.logger.Info("app: stopping task processor")
		app.processor.Stop()
		app.logger.Info("app: task processor stopped")

		err := srv.Shutdown(ctx)
		if err != nil {
			app.logger.Error("error during server shutdown", "error", err)
		}

		// Wait for NATS to shut down
		if app.natsShutdown != nil {
			app.logger.Info("app: waiting for NATS shutdown")
			select {
			case <-app.natsShutdown:
				app.logger.Info("app: NATS shutdown complete")
			case <-time.After(5 * time.Second):
				app.logger.Error("timeout waiting for NATS shutdown")
			}
		}

		shutdownErrorChan <- err
	}()

	// Wait for either server error or shutdown
	select {
	case err := <-serverErrorChan:
		if !errors.Is(err, http.ErrServerClosed) {
			app.logger.Error("server error", "error", err)
			internal.SentryReportError(context.Background(), err, sentry.LevelError, map[string]string{
				"action": "server_error",
			})
			return err
		}
	case err := <-shutdownErrorChan:
		if err != nil {
			app.logger.Error("shutdown error", "error", err)
			internal.SentryReportError(context.Background(), err, sentry.LevelError, map[string]string{
				"action": "shutdown_error",
			})
			return err
		}
	}

	app.logger.Info("app: stopped server", slog.Group("server", "addr", srv.Addr))
	return nil
}

// func (app *application) getKV(config jetstream.KeyValueConfig) (jetstream.KeyValue, error) {
// 	/* KEEP THIS
// 	jetstream.KeyValueConfig{
// 		Bucket:      "todos",
// 		Description: "Datastar Todos",
// 		Compression: true,
// 		TTL:         time.Hour,
// 		MaxBytes:    16 * 1024 * 1024,
// 	}
// 	*/
// 	kv, err := app.js.CreateOrUpdateKeyValue(context.Background(), config)

// 	if err != nil {
// 		return nil, fmt.Errorf("error creating key value: %w", err)
// 	}

// 	return kv, nil
// }
