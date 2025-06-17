package internal

import (
	"context"
	"fmt"
	"sync"

	"talent-rodeo/internal/inbox"
)

// InboxDispatch implements EmailHandler and dispatches emails to multiple registered handlers
type InboxDispatch struct {
	mu       sync.RWMutex
	handlers map[string]inbox.EmailHandler
}

// NewInboxDispatch creates a new InboxDispatch instance
func NewInboxDispatch() *InboxDispatch {
	return &InboxDispatch{
		handlers: make(map[string]inbox.EmailHandler),
	}
}

// Register adds a new handler with the given ID.
// If a handler with the same ID already exists, it will be replaced.
// Returns an error if the id is empty or the handler is nil.
func (id *InboxDispatch) Register(handlerID string, handler inbox.EmailHandler) error {
	if handlerID == "" {
		return fmt.Errorf("handler ID cannot be empty")
	}
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}
	id.mu.Lock()
	defer id.mu.Unlock()
	id.handlers[handlerID] = handler
	return nil
}

// Deregister removes a handler by ID
// Returns true if the handler was found and removed, false otherwise
func (id *InboxDispatch) Deregister(handlerID string) bool {
	id.mu.Lock()
	defer id.mu.Unlock()

	if _, exists := id.handlers[handlerID]; exists {
		delete(id.handlers, handlerID)
		return true
	}
	return false
}

// GetHandlerCount returns the number of currently registered handlers
func (id *InboxDispatch) GetHandlerCount() int {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return len(id.handlers)
}

// HandleEmail implements inbox.EmailHandler and dispatches the email to all registered handlers
func (id *InboxDispatch) HandleEmail(ctx context.Context, email *inbox.Email) error {
	id.mu.RLock()
	handlers := make([]inbox.EmailHandler, 0, len(id.handlers))
	for _, handler := range id.handlers {
		handlers = append(handlers, handler)
	}
	id.mu.RUnlock()

	// Call handlers sequentially, collecting any errors
	var errors []error
	for _, handler := range handlers {
		func() {
			defer func() {
				// Recover from panics and treat them as errors
				if r := recover(); r != nil {
					errors = append(errors, fmt.Errorf("handler panicked: %v", r))
				}
			}()
			if err := handler.HandleEmail(ctx, email); err != nil {
				errors = append(errors, err)
			}
		}()
	}

	// If any errors occurred, return them as a combined error
	if len(errors) > 0 {
		return fmt.Errorf("errors occurred in %d handlers: %v", len(errors), errors)
	}

	return nil
}
