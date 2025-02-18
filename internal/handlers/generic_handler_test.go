package handlers

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// fakeJetStream is a minimal fake implementation of jetstream.JetStream.
type fakeJetStream struct {
	createOrUpdateConsumerFunc func(ctx context.Context, stream string, cfg jetstream.ConsumerConfig) (interface{}, error)
	pullSubscribeFunc          func(subject, durable string, opts ...jetstream.PullSubscribeOpt) (interface{}, error)
	consumerFunc               func(ctx context.Context, stream, durable string) (jetstream.Consumer, error)
}

func (f *fakeJetStream) CreateOrUpdateConsumer(ctx context.Context, stream string, cfg jetstream.ConsumerConfig) (interface{}, error) {
	return f.createOrUpdateConsumerFunc(ctx, stream, cfg)
}

func (f *fakeJetStream) PullSubscribe(subject, durable string, opts ...jetstream.PullSubscribeOpt) (interface{}, error) {
	return f.pullSubscribeFunc(subject, durable, opts...)
}

func (f *fakeJetStream) Consumer(ctx context.Context, stream, durable string) (jetstream.Consumer, error) {
	return f.consumerFunc(ctx, stream, durable)
}

// fakeMsg is a minimal fake implementation of jetstream.Msg.
type fakeMsg struct {
	data  []byte
	acked bool
	naked bool
}

func (m *fakeMsg) Data() []byte {
	return m.data
}

func (m *fakeMsg) Ack() error {
	m.acked = true
	return nil
}

func (m *fakeMsg) Nak() error {
	m.naked = true
	return nil
}

// fakeConsumer is a fake implementation of jetstream.Consumer.
type fakeConsumer struct {
	messages []*fakeMsg
}

func (c *fakeConsumer) Fetch(num int, opts ...jetstream.FetchOpt) ([]jetstream.Msg, error) {
	if len(c.messages) == 0 {
		return nil, errors.New("no messages")
	}
	msgs := make([]jetstream.Msg, len(c.messages))
	for i, m := range c.messages {
		msgs[i] = m
	}
	// Clear messages after they have been fetched once.
	c.messages = nil
	return msgs, nil
}

// TestGenericHandler_RegisterAndProcessMessage verifies that the callback is invoked,
// the message is processed, and the acknowledgment is sent correctly.
func TestGenericHandler_RegisterAndProcessMessage(t *testing.T) {
	var callbackInvoked int32

	// Prepare a fake message with known data.
	testData := "test message"
	fMsg := &fakeMsg{data: []byte(testData)}

	// fakeConsumer will initially return our fake message.
	fConsumer := &fakeConsumer{
		messages: []*fakeMsg{fMsg},
	}

	// Create a fake JetStream implementation.
	fJS := &fakeJetStream{
		createOrUpdateConsumerFunc: func(ctx context.Context, stream string, cfg jetstream.ConsumerConfig) (interface{}, error) {
			return nil, nil
		},
		pullSubscribeFunc: func(subject, durable string, opts ...jetstream.PullSubscribeOpt) (interface{}, error) {
			return nil, nil
		},
		consumerFunc: func(ctx context.Context, stream, durable string) (jetstream.Consumer, error) {
			return fConsumer, nil
		},
	}

	// Use a basic logger.
	logger := slog.New(slog.NewTextHandler(nil, nil))
	gh := NewGenericHandler(fJS, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subject := "test.subject"
	durable := "test-durable"
	stream := "test-stream"

	// Register the generic handler with a callback that validates the message.
	err := gh.Register(ctx, stream, subject, durable, func(ctx context.Context, msg jetstream.Msg) error {
		atomic.AddInt32(&callbackInvoked, 1)
		if string(msg.Data()) != testData {
			t.Errorf("callback: got message data %q, want %q", string(msg.Data()), testData)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	// Allow time for the processing goroutine to fetch and process the message.
	time.Sleep(2 * time.Second)
	cancel()

	if atomic.LoadInt32(&callbackInvoked) != 1 {
		t.Errorf("callback was invoked %d times, want 1", callbackInvoked)
	}

	if !fMsg.acked {
		t.Error("expected message to be acknowledged, but it was not")
	}
}
