package inbox

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"talent-rodeo/internal/cloudflare"
	"time"
)

const (
	DefaultTickInterval = 5 * time.Minute
	InboxProcessorKey   = "inbox-processor"
)

type ProcessorSettings struct {
	Paused    bool
	Interval  time.Duration
	StartHour int
	EndHour   int
	MaxEmails int
}

var DefaultSettings = ProcessorSettings{
	Paused:    false,
	Interval:  DefaultTickInterval,
	StartHour: 0,
	EndHour:   0,
	MaxEmails: 50,
}

// Processor handles periodic inbox checking and email processing
type Processor struct {
	reader     InboxReader
	handler    EmailHandler
	logger     *slog.Logger
	ticker     *time.Ticker
	done       chan bool
	wg         sync.WaitGroup
	kv         *cloudflare.KV
	settings   ProcessorSettings
	isRunning  bool
	cancelFunc context.CancelFunc
}

// NewProcessor creates a new inbox processor
func NewProcessor(reader InboxReader, handler EmailHandler, logger *slog.Logger) *Processor {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	if reader == nil {
		panic("inbox reader cannot be nil")
	}
	if handler == nil {
		panic("email handler cannot be nil")
	}

	settings := DefaultSettings
	logger = logger.With("pkg", "inbox.Processor")

	return &Processor{
		reader:   reader,
		handler:  handler,
		logger:   logger,
		done:     make(chan bool),
		settings: settings,
	}
}

// Start begins the inbox processor operation
func (p *Processor) Start(ctx context.Context) error {
	if p.isRunning {
		return fmt.Errorf("processor is already running")
	}

	p.logger.Info("starting inbox processor", "interval", p.settings.Interval)

	if err := p.initCloudflareKV(); err != nil {
		p.logger.Error("failed to initialize Cloudflare KV client", "error", err)
	}

	if err := p.RefreshSettings(); err != nil {
		p.logger.Error("failed to refresh inbox processor settings", "error", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	p.cancelFunc = cancel

	p.ticker = time.NewTicker(p.settings.Interval)
	p.isRunning = true

	// Run once at startup
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		if err := p.process(ctx); err != nil {
			p.logger.Error("error during initial inbox processing", "error", err)
		}
	}()

	// Then run on ticker
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-p.ticker.C:
				if err := p.process(ctx); err != nil {
					p.logger.Error("error during inbox processing", "error", err)
				}
			case <-p.done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// Stop halts the inbox processor
func (p *Processor) Stop() {
	if !p.isRunning {
		return
	}

	p.logger.Info("stopping inbox processor")
	p.ticker.Stop()
	p.done <- true

	if p.cancelFunc != nil {
		p.cancelFunc()
	}

	p.wg.Wait()

	if err := p.reader.Close(); err != nil {
		p.logger.Error("error closing inbox reader", "error", err)
	}

	p.isRunning = false
	p.logger.Info("inbox processor stopped")
}

// initCloudflareKV initializes the Cloudflare KV client
func (p *Processor) initCloudflareKV() error {
	kv, err := cloudflare.NewKV()
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare KV client: %w", err)
	}
	p.kv = kv
	return nil
}

// RefreshSettings refreshes processor settings from Cloudflare KV
func (p *Processor) RefreshSettings() error {
	var settings ProcessorSettings
	if p.kv != nil {
		if err := p.kv.Get(InboxProcessorKey, &settings); err != nil {
			settings = DefaultSettings
			p.logger.Info("No inbox processor settings found, using defaults")
			if err := p.kv.Set(InboxProcessorKey, settings, 0); err != nil {
				return fmt.Errorf("failed to set default inbox processor settings: %w", err)
			}
			p.logger.Info("Default inbox processor settings saved to Cloudflare KV", "settings", settings)
		}
	} else {
		p.logger.Info("Cloudflare KV not initialized, using default settings")
	}

	p.settings = settings
	if p.settings.Interval <= 0 {
		p.settings.Interval = DefaultTickInterval
	}
	if p.settings.StartHour == 0 && p.settings.EndHour == 0 {
		p.settings.StartHour = DefaultSettings.StartHour
		p.settings.EndHour = DefaultSettings.EndHour
	}
	if p.settings.MaxEmails <= 0 {
		p.settings.MaxEmails = DefaultSettings.MaxEmails
	}

	if p.ticker != nil {
		p.ticker.Stop()
	}
	p.ticker = time.NewTicker(p.settings.Interval)

	return nil
}

// IsWithinAllowedHours checks if current time is within allowed processing hours
func (p *Processor) IsWithinAllowedHours() bool {
	if p.settings.StartHour == 0 && p.settings.EndHour == 0 {
		return true
	}

	now := time.Now().UTC()
	currentHour := now.Hour()

	if p.settings.StartHour > p.settings.EndHour {
		return currentHour >= p.settings.StartHour || currentHour < p.settings.EndHour
	} else {
		return currentHour >= p.settings.StartHour && currentHour < p.settings.EndHour
	}
}

// process handles the main inbox processing logic
func (p *Processor) process(ctx context.Context) error {
	p.RefreshSettings()

	if p.settings.Paused {
		p.logger.Info("inbox processor is paused, skipping processing")
		return nil
	}

	if !p.IsWithinAllowedHours() {
		p.logger.Debug("inbox processor outside allowed hours",
			"start_hour", p.settings.StartHour,
			"end_hour", p.settings.EndHour,
			"current_time_utc", time.Now().UTC().Format("15:04"))
		return nil
	}

	p.logger.Info("processing inbox...")

	// Get unread emails
	emails, err := p.reader.GetUnreadEmails(ctx, p.settings.MaxEmails)
	if err != nil {
		return fmt.Errorf("failed to get unread emails: %w", err)
	}

	if len(emails) == 0 {
		p.logger.Debug("no unread emails found")
		return nil
	}

	p.logger.Info("found unread emails", "count", len(emails))

	// Process each email
	processedCount := 0
	for _, email := range emails {
		if err := p.handler.HandleEmail(ctx, email); err != nil {
			p.logger.Error("failed to handle email", "email_id", email.ID, "error", err)
			continue
		}

		// Mark email as read after successful processing
		if err := p.reader.MarkAsRead(ctx, email.ID); err != nil {
			p.logger.Error("failed to mark email as read", "email_id", email.ID, "error", err)
			continue
		}

		processedCount++
		p.logger.Debug("processed email", "email_id", email.ID, "subject", email.Subject)
	}

	p.logger.Info("completed inbox processing", "processed", processedCount, "total", len(emails))
	return nil
}