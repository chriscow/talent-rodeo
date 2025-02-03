package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
)

/*
{
  "conversation_id": "12345",
  "timestamp": "2025-01-29T06:28:31Z",
  "duration": "583.4",
  "format": "ulaw",
  "sample_rate": 8000,
  "segments": [
    {
      "speaker": "human",
      "start": 0.0,
      "end": 12.3,
      "transcript": "Hello, this is Chris...",
      "silence_before": 0.0,
      "silence_after": 0.8,
      "volume_avg": 0.75
    },
    {
      "speaker": "assistant",
      "start": 13.1,
      "end": 25.6,
      "transcript": "Hi Chris, this is Shirley...",
      "silence_before": 0.8,
      "silence_after": 0.4,
      "volume_avg": 0.82,
      "interruption": false
    }
  ],
  "analysis": {
    "total_silence": 45.2,
    "interruptions": 2,
    "speaker_time": {
      "human": 234.5,
      "assistant": 303.7
    },
    "average_turn_length": {
      "human": 8.2,
      "assistant": 12.4
    }
  }
}
*/

type Segment struct {
	Speaker    string    `json:"speaker"`
	StartTime  time.Time `json:"start_time"`
	ByteOffset int64     `json:"byte_offset"`
	Length     int       `json:"length"`
}

type ConversationMetadata struct {
	StartTime time.Time `json:"start_time"`
	Segments  []Segment `json:"segments"`
}

type ConversationRecorder struct {
	file     *os.File
	metadata *ConversationMetadata
	mu       sync.Mutex
	logger   *slog.Logger
}

func NewConversationRecorder(path string, logger *slog.Logger) (*ConversationRecorder, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create recording file: %w", err)
	}

	return &ConversationRecorder{
		file: f,
		metadata: &ConversationMetadata{
			StartTime: time.Now(),
			Segments:  make([]Segment, 0),
		},
		logger: logger,
	}, nil
}

// Close implements io.Closer
func (r *ConversationRecorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Write final metadata
	if err := r.writeMetadata(); err != nil {
		r.logger.Error("failed to write metadata", "error", err)
	}

	return r.file.Close()
}

func (r *ConversationRecorder) writeMetadata() error {
	// Write metadata to the end of the file
	if _, err := r.file.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("seek to end: %w", err)
	}

	if _, err := r.file.WriteString("metadata: "); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}

	if err := json.NewEncoder(r.file).Encode(r.metadata); err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}

	return nil
}

func (r *ConversationRecorder) readMetadata(f *os.File) (*ConversationMetadata, error) {
	// Seek to the beginning of the file
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek to start: %w", err)
	}

	// Read metadata
	var metadata ConversationMetadata
	_, err := fmt.Fscanf(f, "metadata: ")
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}

	if err := json.NewDecoder(f).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("decode metadata: %w", err)
	}

	return &metadata, nil
}

// Process implements AudioProcessor
func (r *ConversationRecorder) Process(ctx context.Context, chunk AudioChunk) (AudioChunk, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Seek to end to determine current offset
	currentOffset, err := r.file.Seek(0, io.SeekEnd)
	if err != nil {
		return chunk, fmt.Errorf("get current offset: %w", err)
	}

	// Update the chunk offset
	chunk.ByteOffset = currentOffset

	// Write the audio data
	if _, err := r.file.Write(chunk.Data); err != nil {
		return chunk, fmt.Errorf("write audio: %w", err)
	}

	// Append a new segment with updated offset info
	r.metadata.Segments = append(r.metadata.Segments, Segment{
		Speaker:    chunk.Speaker,
		StartTime:  time.Now(),
		ByteOffset: chunk.ByteOffset,
		Length:     len(chunk.Data),
	})

	return chunk, nil
}
