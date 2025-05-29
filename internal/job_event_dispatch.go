package internal

import (
	"sync"
)

// JobEventHandler defines the interface for handling job-related events
type JobEventHandler interface {
	OnSearchStarted(task SearchTask)
	OnJobListing(task SearchTask, listing JobListing)
	OnJobDetail(task SearchTask, details JobDetail)
	OnPageComplete(task SearchTask, page int, jobsFound int, errors []error)
	OnSearchCompleted(task SearchTask, totalJobs int, errors []error)
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

// Register adds a new handler with the given ID
// If a handler with the same ID already exists, it will be replaced
func (jed *JobEventDispatch) Register(id string, handler JobEventHandler) {
	jed.mu.Lock()
	defer jed.mu.Unlock()
	jed.handlers[id] = handler
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
func (jed *JobEventDispatch) OnSearchStarted(task SearchTask) {
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
			handler.OnSearchStarted(task)
		}()
	}
}

// OnJobListing implements JobEventHandler
func (jed *JobEventDispatch) OnJobListing(task SearchTask, listing JobListing) {
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
			handler.OnJobListing(task, listing)
		}()
	}
}

// OnJobDetail implements JobEventHandler
func (jed *JobEventDispatch) OnJobDetail(task SearchTask, details JobDetail) {
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
			handler.OnJobDetail(task, details)
		}()
	}
}

// OnPageComplete implements JobEventHandler
func (jed *JobEventDispatch) OnPageComplete(task SearchTask, page int, jobsFound int, errors []error) {
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
			handler.OnPageComplete(task, page, jobsFound, errors)
		}()
	}
}

// OnSearchCompleted implements JobEventHandler
func (jed *JobEventDispatch) OnSearchCompleted(task SearchTask, totalJobs int, errors []error) {
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
			handler.OnSearchCompleted(task, totalJobs, errors)
		}()
	}
}
