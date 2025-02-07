package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// JobAnalysis represents the complete analysis of a job description
// type JobAnalysis struct {
// 	Requirements *Requirements `json:"requirements" description:"Extracted requirements"`
// 	Compensation *Compensation `json:"compensation" description:"Extracted compensation details"`
// 	Workplace    *Workplace    `json:"workplace" description:"Extracted workplace details"`
// 	Culture      *Culture      `json:"culture" description:"Extracted cultural aspects"`
// }

// BaseHandler provides common functionality for all handlers
// type BaseHandler struct {
// 	ctx      context.Context
// 	llm      minds.ContentGenerator
// 	logger   *slog.Logger
// 	js       jetstream.JetStream
// 	consumer jetstream.Consumer
// }

func getConsumer(ctx context.Context, js jetstream.JetStream, streamName string, consumerName string, filterSubject string) (jetstream.Consumer, error) {
	// Create the consumer
	_, err := js.CreateOrUpdateConsumer(ctx, streamName, jetstream.ConsumerConfig{
		Name:          consumerName,
		FilterSubject: filterSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	// Get a consumer instance
	cons, err := js.Consumer(ctx, streamName, consumerName)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer: %w", err)
	}

	return cons, nil
}
