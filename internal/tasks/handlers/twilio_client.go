package handlers

import (
	"context"
	"fmt"

	"github.com/twilio/twilio-go"
	twilioApi "github.com/twilio/twilio-go/rest/api/v2010"
)

// RealTwilioClient implements TwilioClient using the actual Twilio API
type RealTwilioClient struct {
	client     *twilio.RestClient
	fromNumber string
}

// NewRealTwilioClient creates a new Twilio client
func NewRealTwilioClient(accountSID, authToken, fromNumber string) *RealTwilioClient {
	return &RealTwilioClient{
		client:     twilio.NewRestClientWithParams(twilio.ClientParams{Username: accountSID, Password: authToken}),
		fromNumber: fromNumber,
	}
}

func (c *RealTwilioClient) MakeCall(ctx context.Context, from, to, webhookURL string) (string, error) {
	params := &twilioApi.CreateCallParams{}
	params.SetFrom(from)
	params.SetTo(to)
	params.SetUrl(webhookURL)
	params.SetStatusCallback(webhookURL + "/status")

	resp, err := c.client.Api.CreateCall(params)
	if err != nil {
		return "", fmt.Errorf("failed to create call: %w", err)
	}

	return *resp.Sid, nil
}

func (c *RealTwilioClient) GetCallStatus(ctx context.Context, callSID string) (string, error) {
	params := &twilioApi.FetchCallParams{}
	resp, err := c.client.Api.FetchCall(callSID, params)
	if err != nil {
		return "", fmt.Errorf("failed to fetch call status: %w", err)
	}

	return *resp.Status, nil
}

func (c *RealTwilioClient) EndCall(ctx context.Context, callSID string) error {
	params := &twilioApi.UpdateCallParams{}
	params.SetStatus("completed")

	_, err := c.client.Api.UpdateCall(callSID, params)
	if err != nil {
		return fmt.Errorf("failed to end call: %w", err)
	}

	return nil
}
