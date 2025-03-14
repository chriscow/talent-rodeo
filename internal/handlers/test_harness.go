package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// SendAndReceiveMessage is a generic test harness function that publishes an input message of type T
// to a given subject and waits for a response message of type R from an existing consumer in the specified stream.
// It marshals the input to JSON, publishes it, then fetches a single message from the consumer,
// unmarshalling it into type R.
// Debug logging is provided via t.Logf calls.
func SendAndReceiveMessage[T any, R any](t *testing.T, ctx context.Context, js jetstream.JetStream, streamName string, publishSubject, responseSubject string, input T, fetchTimeout time.Duration, consumerName string) (R, error) {
	var zero R

	// Create a new consumer each time with a DeliverPolicy that ensures we get responses
	// even if they were published before the consumer was created.
	_, err := js.CreateOrUpdateConsumer(ctx, streamName, jetstream.ConsumerConfig{
		Name:          consumerName,
		FilterSubject: responseSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverNewPolicy,
	})
	if err != nil {
		t.Fatalf("SendAndReceiveMessage: Failed to create consumer: %v", err)
	}

	// Marshal input to JSON.
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return zero, fmt.Errorf("SendAndReceiveMessage: failed to marshal input: %w", err)
	}
	t.Logf("SendAndReceiveMessage: Publishing to subject %q: %s", publishSubject, string(inputJSON))

	// Publish the input message.
	_, err = js.Publish(ctx, publishSubject, inputJSON)
	if err != nil {
		return zero, fmt.Errorf("SendAndReceiveMessage: failed to publish message: %w", err)
	}
	t.Log("SendAndReceiveMessage: Message published, waiting for response...")

	// Retrieve the configured consumer.
	consumer, err := js.Consumer(ctx, streamName, consumerName)
	if err != nil {
		return zero, fmt.Errorf("SendAndReceiveMessage: failed to get consumer %q: %w", consumerName, err)
	}
	t.Logf("SendAndReceiveMessage: Using consumer %q from stream %q", consumerName, streamName)

	// Fetch one message using the provided fetch timeout.
	msgs, err := consumer.Fetch(1, jetstream.FetchMaxWait(fetchTimeout))
	if err != nil {
		return zero, fmt.Errorf("SendAndReceiveMessage: failed to fetch messages: %w", err)
	}
	msg := <-msgs.Messages()
	if msg == nil {
		return zero, fmt.Errorf("SendAndReceiveMessage: no message received")
	}
	t.Logf("SendAndReceiveMessage: Received message: %s", string(msg.Data()))

	// Unmarshal the JSON payload into the expected type.
	var result R
	if err := json.Unmarshal(msg.Data(), &result); err != nil {
		return zero, fmt.Errorf("SendAndReceiveMessage: failed to unmarshal message: %w", err)
	}
	// Acknowledge the message.
	msg.Ack()
	t.Log("SendAndReceiveMessage: Message acknowledged, returning result.")
	return result, nil
}
