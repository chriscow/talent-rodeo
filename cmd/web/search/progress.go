package search

import (
	"time"

	"github.com/dgraph-io/ristretto/v2"
)

const (
	// Default TTL for progress entries in the cache (15 minutes)
	defaultProgressTTL = 15 * time.Minute
)

// SearchProgressCache manages the cache for search progress data
type SearchProgressCache struct {
	cache *ristretto.Cache[string, *SearchProgress]
}

// NewSearchProgressCache creates a new cache for search progress with the given configuration
func NewSearchProgressCache() (*SearchProgressCache, error) {
	cache, err := ristretto.NewCache(&ristretto.Config[string, *SearchProgress]{
		NumCounters: 1e6,     // number of keys to track frequency of
		MaxCost:     1 << 30, // maximum cost of cache (1GB)
		BufferItems: 64,      // number of keys per Get buffer
		Metrics:     false,   // no metrics collection
	})
	if err != nil {
		return nil, err
	}

	return &SearchProgressCache{
		cache: cache,
	}, nil
}

// Get retrieves a search progress from the cache
func (c *SearchProgressCache) Get(userID string) (*SearchProgress, bool) {
	return c.cache.Get(userID)
}

// Set stores a search progress in the cache with the default TTL
func (c *SearchProgressCache) Set(userID string, progress *SearchProgress) {
	c.SetWithTTL(userID, progress, defaultProgressTTL)
}

// SetWithTTL stores a search progress in the cache with a custom TTL
func (c *SearchProgressCache) SetWithTTL(userID string, progress *SearchProgress, ttl time.Duration) {
	c.cache.SetWithTTL(userID, progress, 1, ttl)
	// Wait for the write to complete
	c.cache.Wait()
}

// Delete removes a search progress from the cache
func (c *SearchProgressCache) Delete(userID string) {
	c.cache.Del(userID)
	// Wait for the delete to complete
	c.cache.Wait()
}

// GetOrCreate retrieves a search progress from the cache or creates a new one if not found
func (c *SearchProgressCache) GetOrCreate(userID string) *SearchProgress {
	// Try to get from cache first
	progress, found := c.Get(userID)
	if found {
		return progress
	}

	// Create new progress if not found
	progress = &SearchProgress{
		UserID:             userID,
		TotalJobsFound:     0,
		JobsAnalyzed:       0,
		JobsMatched:        0,
		JobsNotMatched:     0,
		SearchCompleted:    false,
		AnalysisInProgress: false,
	}

	// Store in cache with default TTL
	c.Set(userID, progress)
	return progress
}

// UpdateProgress safely updates a search progress using the provided function
func (c *SearchProgressCache) UpdateProgress(userID string, updateFn func(*SearchProgress)) *SearchProgress {
	progress := c.GetOrCreate(userID)

	// Lock the progress to avoid concurrent modifications to the same progress object
	progress.mu.Lock()
	defer progress.mu.Unlock()

	// Apply the update function
	updateFn(progress)

	// Update the cache with the modified progress
	c.Set(userID, progress)

	return progress
}
