package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/twilio/twilio-go"
	twilioApi "github.com/twilio/twilio-go/rest/api/v2010"
)

// RealTwilioClient implements TwilioClient using the Twilio API
type RealTwilioClient struct {
	client     *twilio.RestClient
	logger     *slog.Logger
	fromNumber string
}

// NewRealTwilioClient creates a new Twilio client
func NewRealTwilioClient(accountSID, authToken, fromNumber string) *RealTwilioClient {
	client := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: accountSID,
		Password: authToken,
	})

	return &RealTwilioClient{
		client:     client,
		fromNumber: fromNumber,
		logger:     slog.Default(),
	}
}

// MakeCall initiates a phone call using Twilio
func (c *RealTwilioClient) MakeCall(ctx context.Context, from, to, twiml string) (string, error) {
	c.logger.Debug("Making Twilio call",
		"from", from,
		"to", to,
		"twiml", twiml,
	)

	params := &twilioApi.CreateCallParams{}
	params.SetTo(to)
	params.SetFrom(from)
	params.SetTwiml(twiml)

	resp, err := c.client.Api.CreateCall(params)
	if err != nil {
		return "", fmt.Errorf("failed to create call: %w", err)
	}

	c.logger.Info("Call initiated",
		"call_sid", *resp.Sid,
		"status", *resp.Status,
	)

	return *resp.Sid, nil
}

// GetCallStatus retrieves the current status of a call
func (c *RealTwilioClient) GetCallStatus(ctx context.Context, callSID string) (string, error) {
	c.logger.Debug("Getting call status", "call_sid", callSID)

	params := &twilioApi.FetchCallParams{}
	resp, err := c.client.Api.FetchCall(callSID, params)
	if err != nil {
		if strings.Contains(err.Error(), "Unable to fetch record") {
			return "", ErrCallNotFound
		}
		return "", fmt.Errorf("failed to fetch call: %w", err)
	}

	c.logger.Debug("Retrieved call status",
		"call_sid", callSID,
		"status", *resp.Status,
	)

	return *resp.Status, nil
}

// EndCall terminates an active call
func (c *RealTwilioClient) EndCall(ctx context.Context, callSID string) error {
	c.logger.Debug("Ending call", "call_sid", callSID)

	params := &twilioApi.UpdateCallParams{}
	params.SetStatus("completed")

	_, err := c.client.Api.UpdateCall(callSID, params)
	if err != nil {
		if strings.Contains(err.Error(), "Unable to update record") {
			return ErrCallNotFound
		}
		return fmt.Errorf("failed to end call: %w", err)
	}

	c.logger.Info("Call ended", "call_sid", callSID)
	return nil
}
