package voice

import (
	"context"
	"fmt"
	"log/slog"
)

// ProcessingPipeline manages a chain of AudioProcessors
type ProcessingPipeline struct {
	processors []AudioProcessor
	logger     *slog.Logger
}

func NewProcessingPipeline(processors ...AudioProcessor) *ProcessingPipeline {
	return &ProcessingPipeline{
		processors: processors,
		logger:     slog.Default(),
	}
}

func (p *ProcessingPipeline) Process(ctx context.Context, chunk AudioChunk) (AudioChunk, error) {
	currentChunk := chunk

	for i, processor := range p.processors {
		select {
		case <-ctx.Done():
			return AudioChunk{}, ctx.Err()
		default:
			var err error
			currentChunk, err = processor.Process(ctx, currentChunk)
			if err != nil {
				return AudioChunk{}, fmt.Errorf("processor %d failed: %w", i, err)
			}
		}
	}

	return currentChunk, nil
}
