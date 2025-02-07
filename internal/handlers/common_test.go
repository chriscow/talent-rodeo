package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/chriscow/minds"
	"github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// mockResponse implements minds.Response for testing
type mockResponse struct {
	content string
}

func (r mockResponse) String() string {
	return r.content
}

func (r mockResponse) ToolCalls() []minds.ToolCall {
	return nil
}

// mockLLM implements minds.ContentGenerator for testing
type mockLLM struct {
	responses map[string]string
	queue     []string
	index     int
}

func (m *mockLLM) GenerateContent(ctx context.Context, request minds.Request) (minds.Response, error) {
	if len(m.queue) > 0 {
		if m.index >= len(m.queue) {
			return nil, fmt.Errorf("no more responses in queue")
		}
		response := m.queue[m.index]
		m.index++
		return mockResponse{content: response}, nil
	}

	response, ok := m.responses[request.Messages[0].Content]
	if !ok {
		return nil, fmt.Errorf("no response for prompt: %s", request.Messages[0].Content)
	}
	return mockResponse{content: response}, nil
}

func (m *mockLLM) Close() {
	// No-op for mock
}

func (m *mockLLM) ModelName() string {
	return "mock"
}

// SetResponse sets a single response for the mock LLM
func (m *mockLLM) SetResponse(response string) {
	m.queue = []string{response}
	m.index = 0
}

// SetResponses sets multiple responses for the mock LLM
func (m *mockLLM) SetResponses(responses []string) {
	m.queue = responses
	m.index = 0
}

func deleteConsumers(t *testing.T, stream jetstream.Stream) {
	t.Helper()

	ctx := context.Background()
	consumers := stream.ListConsumers(ctx)
	for info := range consumers.Info() {
		if err := stream.DeleteConsumer(ctx, info.Name); err != nil {
			t.Fatalf("Failed to delete consumer %s: %v", info.Name, err)
		}
	}
}

// deleteStreams removes all streams from the JetStream server
func deleteStreams(t *testing.T, js jetstream.JetStream) {
	t.Helper()

	ctx := context.Background()
	streams := js.StreamNames(ctx)
	for name := range streams.Name() {
		stream, err := js.Stream(ctx, name)
		if err != nil {
			t.Fatalf("Failed to get stream %s: %v", name, err)
		}
		deleteConsumers(t, stream)
		if err := js.DeleteStream(ctx, name); err != nil {
			t.Fatalf("Failed to delete stream %s: %v", name, err)
		}
	}
}

// testEnv provides a test environment with NATS
type testEnv struct {
	js       jetstream.JetStream
	nc       *nats.Conn
	llm      *mockLLM
	logger   *slog.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	cleanups []func()
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create NATS server with JetStream enabled
	opts := test.DefaultTestOptions
	opts.JetStream = true
	srv := test.RunServer(&opts)
	t.Cleanup(srv.Shutdown)

	// Wait for server to be ready
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server failed to start")
	}

	// Connect to NATS
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}

	// Create JetStream
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("Failed to create JetStream: %v", err)
	}

	// Delete any existing streams
	deleteStreams(t, js)

	// Create stream
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      rodeoStream,
		Subjects:  []string{"rodeo.>"},
		Retention: jetstream.InterestPolicy,
	})
	if err != nil {
		t.Fatalf("Failed to create stream: %v", err)
	}

	// Create mock LLM
	llm := &mockLLM{
		responses: make(map[string]string),
	}

	env := &testEnv{
		js:       js,
		nc:       nc,
		llm:      llm,
		logger:   logger,
		ctx:      ctx,
		cancel:   cancel,
		cleanups: make([]func(), 0),
	}

	// Add cleanup for NATS connection
	env.cleanups = append(env.cleanups, func() {
		nc.Close()
	})

	t.Cleanup(env.cleanup)
	return env
}

func (e *testEnv) cleanup() {
	e.cancel()
	for _, cleanup := range e.cleanups {
		cleanup()
	}
}
