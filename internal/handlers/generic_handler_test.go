package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type TestMessage struct {
	Content string `json:"content"`
}

type TestResponse struct {
	Processed bool   `json:"processed"`
	Content   string `json:"content"`
}

func TestGenericHandler_MessageHandling(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	const (
		testSubject  = "rodeo.test.input"
		testResponse = "rodeo.test.response"
	)

	// Create test input
	input := TestMessage{
		Content: "test message content",
	}

	// Create a channel to track message processing
	processed := make(chan bool, 1)

	// Create handler
	handler := NewGenericHandler(env.js, env.logger)

	// Register message handler
	err := handler.Register(env.ctx, rodeoStream, testSubject, "TEST-GENERIC-HANDLER", func(ctx context.Context, msg jetstream.Msg) error {
		var receivedMsg TestMessage
		if err := json.Unmarshal(msg.Data(), &receivedMsg); err != nil {
			return err
		}

		// Verify received message content
		if receivedMsg.Content != input.Content {
			t.Errorf("Expected message content %q, got %q", input.Content, receivedMsg.Content)
		}

		// Create and publish response
		response := TestResponse{
			Processed: true,
			Content:   receivedMsg.Content,
		}
		responseData, err := json.Marshal(response)
		if err != nil {
			return err
		}

		_, err = env.js.Publish(ctx, testResponse, responseData)
		if err != nil {
			return err
		}

		processed <- true
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to register handler: %v", err)
	}

	// Use the generic harness to send and receive messages
	got, err := SendAndReceiveMessage[TestMessage, TestResponse](
		t,
		env.ctx,
		env.js,
		rodeoStream,
		testSubject,
		testResponse,
		input,
		2*time.Second,
	)
	if err != nil {
		t.Fatalf("Failed to send and receive message: %v", err)
	}

	// Verify response
	if !got.Processed {
		t.Error("Expected message to be processed")
	}
	if got.Content != input.Content {
		t.Errorf("Expected response content %q, got %q", input.Content, got.Content)
	}
}

func TestGenericHandler_ContextCancellation(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	const testSubject = "rodeo.test.cancellation"

	// Create handler with short-lived context
	handler := NewGenericHandler(env.js, env.logger)
	ctx, cancel := context.WithTimeout(env.ctx, 1*time.Second)
	defer cancel()

	processed := make(chan bool, 1)

	err := handler.Register(ctx, rodeoStream, testSubject, "TEST-CANCELLATION", func(ctx context.Context, msg jetstream.Msg) error {
		processed <- true
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to register handler: %v", err)
	}

	// Wait for context to be cancelled
	<-ctx.Done()

	// Verify no messages are processed after cancellation
	input := TestMessage{Content: "should not be processed"}
	inputData, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal input: %v", err)
	}

	_, err = env.js.Publish(env.ctx, testSubject, inputData)
	if err != nil {
		t.Fatalf("Failed to publish message: %v", err)
	}

	select {
	case <-processed:
		t.Error("Message was processed after context cancellation")
	case <-time.After(2 * time.Second):
		// Success - no message processed after cancellation
	}
}
