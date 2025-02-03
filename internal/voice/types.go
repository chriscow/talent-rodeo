package voice

import (
	"context"
)

// AudioReader reads audio chunks from a source.
// Implementations must be safe for concurrent use.
type AudioReader interface {
	Read(ctx context.Context) (AudioChunk, error)
	Close() error
}

// AudioWriter writes audio chunks to a destination.
// Implementations must be safe for concurrent use.
type AudioWriter interface {
	Write(ctx context.Context, chunk AudioChunk) error
	Close() error
}

type Converter interface {
	Convert(ctx context.Context, text string) (AudioChunk, error)
	Close() error
}

type TextChunk struct {
	Content string
	EOF     bool
}

const outputFormat = "ulaw_8000"

// AudioChunk represents a piece of audio data with its associated metadata
type AudioChunk struct {
	Data       []byte
	Speaker    string // "human" or "assistant"
	ByteOffset int64  // Offset in the original audio file
	Format     string
	Final      bool
	Error      error
}

// AudioProcessor defines the interface for components that can process audio data
// in either direction. Processors can be chained together to form a processing pipeline.
type AudioProcessor interface {
	// Process receives an audio chunk and returns a modified chunk.
	// It can modify the audio data, collect metrics, or perform side effects like recording.
	// Returning an error will halt the pipeline.
	Process(ctx context.Context, chunk AudioChunk) (AudioChunk, error)
}

// AudioProcessorFunc is an adapter to allow the use of ordinary functions as AudioProcessors.
type AudioProcessorFunc func(ctx context.Context, chunk AudioChunk) (AudioChunk, error)

func (f AudioProcessorFunc) Process(ctx context.Context, chunk AudioChunk) (AudioChunk, error) {
	return f(ctx, chunk)
}
