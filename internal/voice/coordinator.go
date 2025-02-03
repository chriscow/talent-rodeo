package voice

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
)

type Coordinator struct {
	reader          AudioReader
	writer          AudioWriter
	inputPipeline   *ProcessingPipeline
	outputPipeline  *ProcessingPipeline
	logger          *slog.Logger
	errChan         chan error
	doneChan        chan struct{}
	shutdownTimeout time.Duration
	cancelFunc      context.CancelFunc
	wg              sync.WaitGroup
}

type CoordinatorOption func(*Coordinator)

func WithShutdownTimeout(timeout time.Duration) CoordinatorOption {
	return func(c *Coordinator) {
		c.shutdownTimeout = timeout
	}
}

func WithLogger(logger *slog.Logger) CoordinatorOption {
	return func(c *Coordinator) {
		c.logger = logger
	}
}

func NewCoordinator(
	logger *slog.Logger,
	reader AudioReader,
	writer AudioWriter,
	inputProcessors []AudioProcessor,
	outputProcessors []AudioProcessor,
	opts ...CoordinatorOption,
) *Coordinator {
	c := &Coordinator{
		logger:          logger,
		reader:          reader,
		writer:          writer,
		inputPipeline:   NewProcessingPipeline(inputProcessors...),
		outputPipeline:  NewProcessingPipeline(outputProcessors...),
		errChan:         make(chan error, 2),
		doneChan:        make(chan struct{}),
		shutdownTimeout: 10 * time.Second,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Coordinator) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	c.cancelFunc = cancel

	// Start the read loop
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := c.readLoop(ctx); err != nil && !errors.Is(err, context.Canceled) {
			select {
			case c.errChan <- fmt.Errorf("read loop error: %w", err):
			default:
			}
		}
	}()

	// Wait for error, done signal, or context cancellation
	select {
	case err := <-c.errChan:
		c.shutdown(fmt.Sprintf("error: %v", err))
		return err
	case <-c.doneChan:
		c.shutdown("done channel closed")
		return nil
	case <-ctx.Done():
		c.shutdown("context cancelled")
		return ctx.Err()
	}
}

func (c *Coordinator) readLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			chunk, err := c.reader.Read(ctx)
			if err != nil {
				if errors.Is(err, io.EOF) {
					continue
				}
				return fmt.Errorf("error reading chunk: %w", err)
			}

			// Process the chunk through the output pipeline
			processedChunk, err := c.outputPipeline.Process(ctx, chunk)
			if err != nil {
				return fmt.Errorf("output pipeline error: %w", err)
			}

			// Write the processed chunk
			if err := c.writer.Write(ctx, processedChunk); err != nil {
				return fmt.Errorf("error writing chunk: %w", err)
			}

			// If this was the final chunk, we're done sending audio
			if processedChunk.Final {
				continue
			}
		}
	}
}

func (c *Coordinator) shutdown(reason string) {
	c.logger.Info("coordinator shutting down", "reason", reason)

	if c.cancelFunc != nil {
		c.cancelFunc()
	}

	// Create a context with timeout for shutdown operations
	ctx, cancel := context.WithTimeout(context.Background(), c.shutdownTimeout)
	defer cancel()

	// Wait for goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		c.logger.Debug("all goroutines completed")
	case <-ctx.Done():
		c.logger.Warn("shutdown timed out waiting for goroutines")
	}

	// Close reader and writer
	if err := c.reader.Close(); err != nil {
		c.logger.Error("error closing reader", "error", err)
	}
	if err := c.writer.Close(); err != nil {
		c.logger.Error("error closing writer", "error", err)
	}
}
