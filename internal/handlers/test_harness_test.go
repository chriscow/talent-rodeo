package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestSendAndReceiveMessageHarness(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Setup a subscription on "rodeo.request" to simulate an external responder.
	sub, err := env.nc.SubscribeSync("rodeo.request")
	if err != nil {
		t.Fatalf("TestSendAndReceiveMessageHarness: Failed to subscribe to 'rodeo.request': %v", err)
	}
	// Launch a goroutine that listens for a request and publishes a response.
	go func() {
		msg, err := sub.NextMsg(1 * time.Second)
		if err != nil {
			t.Logf("TestSendAndReceiveMessageHarness: No message received on 'rodeo.request': %v", err)
			return
		}
		t.Logf("TestSendAndReceiveMessageHarness: Received request: %s", string(msg.Data))
		// Create a response message.
		response := struct {
			Message string `json:"message"`
		}{
			Message: "hello world",
		}
		responseData, err := json.Marshal(response)
		if err != nil {
			t.Logf("TestSendAndReceiveMessageHarness: Failed to marshal response: %v", err)
			return
		}
		_, err = env.js.Publish(context.Background(), "rodeo.response", responseData)
		if err != nil {
			t.Logf("TestSendAndReceiveMessageHarness: Failed to publish response: %v", err)
			return
		}
		t.Log("TestSendAndReceiveMessageHarness: Response published on 'rodeo.response'")
	}()

	// Define the input message.
	input := struct {
		Request string `json:"request"`
	}{
		Request: "trigger",
	}

	// Use the harness to send the input and expect a response.
	type Response struct {
		Message string `json:"message"`
	}
	resp, err := SendAndReceiveMessage[struct {
		Request string `json:"request"`
	}, Response](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		"rodeo.request",
		"rodeo.response",
		input,
		2*time.Second,
	)
	if err != nil {
		t.Fatalf("TestSendAndReceiveMessageHarness: SendAndReceiveMessage failed: %v", err)
	}
	t.Logf("TestSendAndReceiveMessageHarness: Received response: %+v", resp)
	if resp.Message != "hello world" {
		t.Errorf("TestSendAndReceiveMessageHarness: got message %q, want %q", resp.Message, "hello world")
	}
}
