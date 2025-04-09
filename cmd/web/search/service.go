package search

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"talent-rodeo/internal"
	"talent-rodeo/internal/search"
	"time"

	"cloud.google.com/go/firestore"
)

const (
	// defaultTickInterval = 15 * time.Minute
	defaultTickInterval = 5 * time.Second

	FreeTierFrequency       = 72 * time.Hour // 3 days
	ProTierFrequency        = 24 * time.Hour // 1 day
	EnterpriseTierFrequency = 4 * time.Hour  // 4 hours

	TestingModeNumPages = 1
	FreeTierNumPages    = 3
	ProTierNumPages     = 10
)

// Mailer interface for sending notifications
type Mailer interface {
	// func (m *Mailer) Send(recipient string, data any, patterns ...string) error {
	Send(recipient string, data any, patterns ...string) error
}

// JobEventHandler defines the interface for handling job-related events
type JobEventHandler interface {
	OnNewJob(userID string, count int) error
	OnSearchCompleted(userID string, totalJobs int, errors []error) error
}

// Service manages background job searches for users
type Service struct {
	fs                *firestore.Client
	logger            *slog.Logger
	ticker            *time.Ticker
	queue             search.JobQueue
	mailer            Mailer
	done              chan struct{}
	processingWG      sync.WaitGroup
	recipient         string
	pusher            PusherInterface
	jobQueue          chan QueuedJob // Queue for job processing
	jobSemaphore      chan struct{}  // Semaphore for job processing
	maxConcurrentJobs int
}

// NewService creates a new search service
func NewService(fs *firestore.Client, logger *slog.Logger, queue search.JobQueue, mailer Mailer, recipient string, pusher PusherInterface) *Service {
	maxConcurrentJobs := 5 // Can be made configurable
	return &Service{
		fs:                fs,
		logger:            logger,
		queue:             queue,
		mailer:            mailer,
		done:              make(chan struct{}),
		recipient:         recipient,
		pusher:            pusher,
		jobQueue:          make(chan QueuedJob, 100), // Buffer size can be adjusted
		jobSemaphore:      make(chan struct{}, maxConcurrentJobs),
		maxConcurrentJobs: maxConcurrentJobs,
	}
}

// Start begins the background search processing
func (s *Service) Start() error {
	s.logger.Info("search: starting search service")
	if s.ticker == nil {
		s.ticker = time.NewTicker(defaultTickInterval)
	}

	s.StartJobWorkers(context.Background(), 5)

	// First requeue any users that were previously queued but not completed
	if err := s.requeueIncompleteSearches(); err != nil {
		return fmt.Errorf("failed to requeue incomplete searches: %w", err)
	}

	// Start the background processing
	go func() {
		s.logger.Info("search: Starting initial search processing")
		// Process searches immediately on start
		s.processingWG.Add(1)
		s.processSearches()
		s.processingWG.Done()
		s.logger.Info("search: completed startup search processing")

		// Then continue with regular interval processing
		s.run()
	}()

	return nil
}

// Stop gracefully shuts down the search service
func (s *Service) Stop() {
	s.logger.Info("search: Stopping search service")
	if s.ticker != nil {
		s.ticker.Stop()
	}

	// Signal processing to stop
	close(s.done)

	// Wait for any in-progress processing to complete
	s.processingWG.Wait()
	s.logger.Info("search: Search service stopped")
}

// run handles the background search processing
func (s *Service) run() {
	for {
		select {
		case <-s.ticker.C:
			s.processingWG.Add(1)
			s.processSearches()
			s.processingWG.Done()
		case <-s.done:
			s.logger.Info("search: Background processing stopped")
			return
		}
	}
}

// processSearches finds users who need searches performed
func (s *Service) processSearches() {
	ctx := context.Background()

	// Test call to ListUsers to verify user retrieval
	allUsers, err := internal.ListUsers(ctx, s.fs, 10, 0)
	if err != nil {
		s.logger.Error("error listing users", "error", err)
		return
	}

	// see if any users have a search error
	eligibleUsers := make([]*internal.User, 0)
	for _, user := range allUsers {
		if s.shouldSearch(user) {
			eligibleUsers = append(eligibleUsers, user)
		}
	}

	if len(eligibleUsers) > 0 {
		s.logger.Debug("checkUsersForSearch: eligible users", "count", len(eligibleUsers))
	}

	processedCount := 0
	for _, user := range eligibleUsers {
		if err := s.enqueueSearch(ctx, user); err != nil {
			user.Search.Error = err.Error()
			user.Search.QueuedAt = nil
			if err := internal.UpdateUserSearch(ctx, s.fs, user.ID, user.Search); err != nil {
				s.logger.Error("error updating user search error", "error", err, "user_id", user.ID)
				s.notifyAdmin(ctx, user, err)
				continue
			}
		}
		processedCount++
	}

	if processedCount > 0 {
		depth, err := s.queue.GetQueueDepth(ctx)
		if err != nil {
			s.logger.Error("error getting queue depth", "error", err)
		}
		s.logger.Info("search: newly enqueued users", "users", processedCount, "total-enqueued", depth)
	}
}

// shouldSearch determines if a user is due for a search based on their tier
func (s *Service) shouldSearch(user *internal.User) bool {
	shouldLog := os.Getenv("TESTING_MODE") == "true"
	if strings.HasPrefix(user.ID, "test-") {
		if shouldLog {
			s.logger.Debug("search: shouldSearch: user is a test user", "user_id", user.ID)
		}
		return false
	}

	if user.Search == nil {
		if shouldLog {
			s.logger.Debug("search: shouldSearch: user has no search criteria", "user_id", user.ID)
		}
		return false
	}

	// if the user has never been searched, and never queued, then we should search them
	if user.Search.CompletedAt == nil && user.Search.QueuedAt == nil {
		if shouldLog {
			s.logger.Debug("search: shouldSearch: user has never been searched or queued", "user_id", user.ID)
		}
		return true
	}

	// if the user has been queued, then we should not search them
	if user.Search.QueuedAt != nil {
		if shouldLog {
			s.logger.Debug("search: shouldSearch: user has been queued", "user_id", user.ID)
		}
		return false
	}

	// if they've never completed a search, we should search them
	if user.Search.CompletedAt == nil {
		if shouldLog {
			s.logger.Debug("search: shouldSearch: user has never completed a search", "user_id", user.ID)
		}
		return true
	}

	// Determine frequency based on tier
	frequency := FreeTierFrequency
	switch user.Tier {
	case internal.UserTierPro:
		frequency = ProTierFrequency
	case internal.UserTierEnterprise:
		frequency = EnterpriseTierFrequency
	}

	// Check if enough time has passed since last search
	since := time.Since(*user.Search.CompletedAt)
	// if shouldLog {
	// 	s.logger.Debug("search: shouldSearch: time since last search", "user_id", user.ID, "since", since, "frequency", frequency)
	// }
	return since >= frequency
}

// enqueueSearch queues a job search for a user
func (s *Service) enqueueSearch(ctx context.Context, user *internal.User) error {
	if user.Search == nil {
		return fmt.Errorf("nil search")
	}

	s.logger.Info("search: enqueueSearch", "user_id", user.ID, "criteria", user.Search.Criteria)

	// Determine number of pages based on tier and testing mode
	numPages := FreeTierNumPages // Default for free tier
	if user.Tier == internal.UserTierPro {
		numPages = ProTierNumPages
	}

	// Override for testing mode
	if os.Getenv("TESTING_MODE") == "true" {
		numPages = TestingModeNumPages
	}

	// Queue a search for each page
	for page := 1; page <= numPages; page++ {
		// Create a copy of the criteria for this page
		criteria := user.Search.Criteria
		criteria.PageNumber = page

		// ScrapingDog requires a single keyword, so we need to queue a search for each keyword
		for _, keyword := range criteria.Keywords {
			criteria.Keywords = []string{keyword}
			// Queue the search with the provider
			if err := s.queue.QueueSearch(ctx, user.ID, criteria); err != nil {
				// Notify admin of the error before returning
				s.notifyAdmin(ctx, user, err)
				return fmt.Errorf("failed to queue search: %w", err)
			}
		}
	}

	return nil
}

// notifyAdmin sends an error notification to the admin
func (s *Service) notifyAdmin(_ context.Context, user *internal.User, err error) {
	data := map[string]interface{}{
		"UserID":    user.ID,
		"UserEmail": user.Email,
		"Error":     err.Error(),
		"Criteria":  user.Search.Criteria,
		"Timestamp": time.Now().Format(time.RFC3339),
	}

	if err := s.mailer.Send(s.recipient, data, "search-error.html"); err != nil {
		s.logger.Error("failed to send admin notification",
			"error", err,
			"user_id", user.ID)
	}
}

// requeueIncompleteSearches finds and requeues any users that were previously queued but not completed
func (s *Service) requeueIncompleteSearches() error {
	ctx := context.Background()

	// Find all users with QueuedAt != nil
	allUsers, err := internal.ListUsers(ctx, s.fs, 0, 0) // Adjust limit as needed
	if err != nil {
		return fmt.Errorf("failed to list users: %w", err)
	}

	requeueCount := 0
	for _, user := range allUsers {
		// Check if user was queued but not completed
		if user.Search != nil && user.Search.QueuedAt != nil {
			s.logger.Info("search: requeueing incomplete search",
				"user_id", user.ID,
				"queued_at", user.Search.QueuedAt,
				"criteria", user.Search.Criteria)

			if err := s.queue.QueueSearch(ctx, user.ID, user.Search.Criteria); err != nil {
				s.logger.Error("failed to requeue search",
					"error", err,
					"user_id", user.ID)
				continue
			}
			requeueCount++
		}
	}

	if requeueCount > 0 {
		s.logger.Info("search: requeued incomplete searches", "count", requeueCount)
	}

	return nil
}
