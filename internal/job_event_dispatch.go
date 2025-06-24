package internal

import (
	"fmt"
	"sync"
)

// JobEventHandler defines the interface for handling job-related events
type JobEventHandler interface {
	OnSearchStarted(task SearchTask, userID *string)
	// OnJobListing returns true if the listing is new (should fetch details), false if duplicate
	OnJobListing(task *SearchTask, listing JobListing, userID *string) bool
	OnJobDetail(task SearchTask, listing JobListing, details JobDetail, userID *string)
	OnPageComplete(task SearchTask, page int, jobsFound int, errors []error, userID *string)
	OnSearchCompleted(task SearchTask, totalJobs int, errors []error, userID *string)
	OnJobDetailExtractionComplete(task SearchTask, jobDetail JobDetail, userID *string)
	OnAllExtractionsComplete(task SearchTask, totalJobs int, userID *string)
}

// JobEventDispatch implements JobEventHandler and dispatches events to multiple registered handlers
type JobEventDispatch struct {
	mu       sync.RWMutex
	handlers map[string]JobEventHandler
}

// NewJobEventDispatch creates a new JobEventDispatch instance
func NewJobEventDispatch() *JobEventDispatch {
	return &JobEventDispatch{
		handlers: make(map[string]JobEventHandler),
	}
}

// Register adds a new handler with the given ID.
// If a handler with the same ID already exists, it will be replaced.
// Returns an error if the id is empty or the handler is nil.
func (jed *JobEventDispatch) Register(id string, handler JobEventHandler) error {
	if id == "" {
		return fmt.Errorf("handler ID cannot be empty")
	}
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}
	jed.mu.Lock()
	defer jed.mu.Unlock()
	jed.handlers[id] = handler
	return nil
}

// Deregister removes a handler by ID
// Returns true if the handler was found and removed, false otherwise
func (jed *JobEventDispatch) Deregister(id string) bool {
	jed.mu.Lock()
	defer jed.mu.Unlock()

	if _, exists := jed.handlers[id]; exists {
		delete(jed.handlers, id)
		return true
	}
	return false
}

// GetHandlerCount returns the number of currently registered handlers
func (jed *JobEventDispatch) GetHandlerCount() int {
	jed.mu.RLock()
	defer jed.mu.RUnlock()
	return len(jed.handlers)
}

// OnSearchStarted implements JobEventHandler
func (jed *JobEventDispatch) OnSearchStarted(task SearchTask, userID *string) {
	jed.mu.RLock()
	handlers := make([]JobEventHandler, 0, len(jed.handlers))
	for _, handler := range jed.handlers {
		handlers = append(handlers, handler)
	}
	jed.mu.RUnlock()

	// Call handlers sequentially, ignoring any errors
	for _, handler := range handlers {
		func() {
			defer func() {
				// Ignore panics from handlers
				recover()
			}()
			handler.OnSearchStarted(task, userID)
		}()
	}
}

// OnJobListing implements JobEventHandler
func (jed *JobEventDispatch) OnJobListing(task *SearchTask, listing JobListing, userID *string) bool {
	jed.mu.RLock()
	handlers := make([]JobEventHandler, 0, len(jed.handlers))
	for _, handler := range jed.handlers {
		handlers = append(handlers, handler)
	}
	jed.mu.RUnlock()

	// Call handlers sequentially, OR-ing their results
	var result bool
	for _, handler := range handlers {
		func() {
			defer func() {
				// Ignore panics from handlers
				recover()
			}()
			if handler.OnJobListing(task, listing, userID) {
				result = true // returning true indicates a new listing and it should be fetched
			}
		}()
	}
	return result
}

// OnJobDetail implements JobEventHandler
func (jed *JobEventDispatch) OnJobDetail(task SearchTask, listing JobListing, details JobDetail, userID *string) {
	jed.mu.RLock()
	handlers := make([]JobEventHandler, 0, len(jed.handlers))
	for _, handler := range jed.handlers {
		handlers = append(handlers, handler)
	}
	jed.mu.RUnlock()

	// Call handlers sequentially, ignoring any errors
	for _, handler := range handlers {
		func() {
			defer func() {
				// Ignore panics from handlers
				recover()
			}()
			handler.OnJobDetail(task, listing, details, userID)
		}()
	}
}

// OnPageComplete implements JobEventHandler
func (jed *JobEventDispatch) OnPageComplete(task SearchTask, page int, jobsFound int, errors []error, userID *string) {
	jed.mu.RLock()
	handlers := make([]JobEventHandler, 0, len(jed.handlers))
	for _, handler := range jed.handlers {
		handlers = append(handlers, handler)
	}
	jed.mu.RUnlock()

	// Call handlers sequentially, ignoring any errors
	for _, handler := range handlers {
		func() {
			defer func() {
				// Ignore panics from handlers
				recover()
			}()
			handler.OnPageComplete(task, page, jobsFound, errors, userID)
		}()
	}
}

// OnSearchCompleted implements JobEventHandler
func (jed *JobEventDispatch) OnSearchCompleted(task SearchTask, totalJobs int, errors []error, userID *string) {
	jed.mu.RLock()
	handlers := make([]JobEventHandler, 0, len(jed.handlers))
	for _, handler := range jed.handlers {
		handlers = append(handlers, handler)
	}
	jed.mu.RUnlock()

	// Call handlers sequentially, ignoring any errors
	for _, handler := range handlers {
		func() {
			defer func() {
				// Ignore panics from handlers
				recover()
			}()
			handler.OnSearchCompleted(task, totalJobs, errors, userID)
		}()
	}
}

// OnJobDetailExtractionComplete implements JobEventHandler
func (jed *JobEventDispatch) OnJobDetailExtractionComplete(task SearchTask, jobDetail JobDetail, userID *string) {
	jed.mu.RLock()
	handlers := make([]JobEventHandler, 0, len(jed.handlers))
	for _, handler := range jed.handlers {
		handlers = append(handlers, handler)
	}
	jed.mu.RUnlock()

	// Call handlers sequentially, ignoring any errors
	for _, handler := range handlers {
		func() {
			defer func() {
				// Ignore panics from handlers
				recover()
			}()
			handler.OnJobDetailExtractionComplete(task, jobDetail, userID)
		}()
	}
}

// OnAllExtractionsComplete implements JobEventHandler
func (jed *JobEventDispatch) OnAllExtractionsComplete(task SearchTask, totalJobs int, userID *string) {
	jed.mu.RLock()
	handlers := make([]JobEventHandler, 0, len(jed.handlers))
	for _, handler := range jed.handlers {
		handlers = append(handlers, handler)
	}
	jed.mu.RUnlock()

	// Call handlers sequentially, ignoring any errors
	for _, handler := range handlers {
		func() {
			defer func() {
				// Ignore panics from handlers
				recover()
			}()
			handler.OnAllExtractionsComplete(task, totalJobs, userID)
		}()
	}
}

// Waiter is an interface for waiting on a specific event.
type Waiter interface {
	Wait() <-chan struct{}
}
