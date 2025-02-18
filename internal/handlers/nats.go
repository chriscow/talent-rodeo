package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	// StreamName is the name of our single Jetstream
	StreamName = "rodeo"
)

// NATSConfig holds configuration for NATS server
type NATSConfig struct {
	Port      int
	JetStream bool
}

// DefaultNATSConfig returns default NATS configuration
func DefaultNATSConfig() NATSConfig {
	return NATSConfig{
		Port:      42223,
		JetStream: true,
	}
}

// StartNATSServer creates and starts a NATS server with the given configuration
func StartNATSServer(ctx context.Context, cfg NATSConfig) (*server.Server, error) {
	svr, err := server.NewServer(&server.Options{
		Port:      cfg.Port,
		JetStream: cfg.JetStream,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create NATS server: %w", err)
	}

	svr.Start()

	// Wait for server to be ready
	if !svr.ReadyForConnections(5 * time.Second) {
		svr.Shutdown()
		return nil, fmt.Errorf("NATS server failed to start")
	}

	return svr, nil
}

// CreateStream creates a NATS stream (publisher)
func CreateStream(ctx context.Context, natsURL string, logger *slog.Logger) (jetstream.Stream, error) {
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		logger.Error("failed to connect to NATS", "error", err)
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		logger.Error("failed to create JetStream context", "error", err)
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	config := jetstream.StreamConfig{
		Name:      rodeoStream,
		Retention: jetstream.InterestPolicy,
		Subjects:  []string{"rodeo.>"},
	}

	stream, err := js.Stream(ctx, rodeoStream)
	if errors.Is(err, jetstream.ErrStreamNotFound) {
		stream, err = js.CreateStream(ctx, config)
		if err != nil {
			logger.Error("failed to create stream", "error", err)
			return nil, fmt.Errorf("failed to create stream: %w", err)
		}
	}

	return stream, nil
}

// CreateConsumer creates a NATS consumer (subscriber)
func CreateConsumer(ctx context.Context, stream jetstream.Stream, name string, logger *slog.Logger) (jetstream.Consumer, error) {

	cfg := jetstream.ConsumerConfig{
		Durable:       name,
		DeliverPolicy: jetstream.DeliverAllPolicy,
	}

	consumer, err := stream.CreateConsumer(ctx, cfg)
	if err != nil {
		logger.Error("failed to create consumer", "error", err)
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	return consumer, nil
}
