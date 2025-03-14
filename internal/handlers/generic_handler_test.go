package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/chriscow/minds"
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

	// Create handler with mock LLM
	handler := NewGenericHandler(env.js, env.logger, env.llm)

	// Register message handler
	err := handler.Register(env.ctx, rodeoStream, testSubject, "TEST-GENERIC-HANDLER", DefaultMaxAckPending, func(ctx context.Context, msg jetstream.Msg) error {
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
		context.Background(),
		env.js,
		rodeoStream,
		testSubject,
		testResponse,
		input,
		5*time.Second,
		"generic-test",
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
	handler := NewGenericHandler(env.js, env.logger, env.llm)
	ctx, cancel := context.WithTimeout(env.ctx, 1*time.Second)
	defer cancel()

	processed := make(chan bool, 1)

	err := handler.Register(ctx, rodeoStream, testSubject, "TEST-CANCELLATION", DefaultMaxAckPending, func(ctx context.Context, msg jetstream.Msg) error {
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

func TestGenericHandler_LLMProcessing(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	const (
		testSubject  = "rodeo.test.llm.input"
		testResponse = "rodeo.test.llm.response"
	)

	// Set up mock LLM response
	mockResponseData := TestResponse{
		Processed: true,
		Content:   "processed content",
	}
	mockResponseJSON, err := json.Marshal(mockResponseData)
	if err != nil {
		t.Fatalf("Failed to marshal mock response: %v", err)
	}

	// Set up the response in the LLM's response map
	env.llm.responses = map[string]string{
		"Test system prompt": string(mockResponseJSON),
	}

	// Create handler with mock LLM
	handler := NewGenericHandler(env.js, env.logger, env.llm)

	// Register LLM handler
	err = handler.RegisterLLMHandler(env.ctx, rodeoStream, testSubject, "TEST-LLM-HANDLER", &testProcessor{
		outputTopic: testResponse,
	}, DefaultMaxAckPending)
	if err != nil {
		t.Fatalf("Failed to register LLM handler: %v", err)
	}

	// Create test input
	input := TestMessage{
		Content: "test message for LLM processing",
	}

	// Use the generic harness to send and receive messages
	got, err := SendAndReceiveMessage[TestMessage, TestResponse](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		testSubject,
		testResponse,
		input,
		2*time.Second,
		"generic-test-llm",
	)
	if err != nil {
		t.Fatalf("Failed to send and receive message: %v", err)
	}

	// Verify response
	if !got.Processed {
		t.Error("Expected message to be processed")
	}
	if got.Content != "processed content" {
		t.Errorf("Expected response content %q, got %q", "processed content", got.Content)
	}
}

func TestGenericHandler_LLMProcessing_NoLLM(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Create handler without LLM
	handler := NewGenericHandler(env.js, env.logger, nil)

	// Attempt to register LLM handler
	err := handler.RegisterLLMHandler(env.ctx, rodeoStream, "test.subject", "TEST-LLM-HANDLER", &testProcessor{
		outputTopic: "test.response",
	}, DefaultMaxAckPending)
	if err == nil {
		t.Error("Expected error when registering LLM handler without LLM, got nil")
	}
}

func TestGenericHandler_LLMProcessing_Error(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	const (
		testSubject  = "rodeo.test.llm.error"
		testResponse = "rodeo.test.llm.error.response"
	)

	// Set up mock LLM with error by having no responses
	env.llm.responses = nil

	// Create handler
	handler := NewGenericHandler(env.js, env.logger, env.llm)

	// Register LLM handler
	err := handler.RegisterLLMHandler(env.ctx, rodeoStream, testSubject, "TEST-LLM-ERROR", &testProcessor{
		outputTopic: testResponse,
	}, DefaultMaxAckPending)
	if err != nil {
		t.Fatalf("Failed to register LLM handler: %v", err)
	}

	// Create test input
	input := TestMessage{
		Content: "test message that should fail",
	}

	// Send message and expect error
	_, err = SendAndReceiveMessage[TestMessage, TestResponse](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		testSubject,
		testResponse,
		input,
		2*time.Second,
		"generic-test-error",
	)
	if err == nil {
		t.Error("Expected error from LLM processing, got nil")
	}
}

// testProcessor implements LLMProcessor for testing
type testProcessor struct {
	outputTopic string
}

func (p *testProcessor) GetSystemPrompt() string {
	return "Test system prompt"
}

func (p *testProcessor) GetResponseSchema() (*minds.ResponseSchema, error) {
	return minds.NewResponseSchema("TestSchema", "Test schema for LLM processing", TestResponse{})
}

func (p *testProcessor) GetOutputTopic() string {
	return p.outputTopic
}
