package internal

import (
	"sync"
	"testing"
)

// Mock handler for testing
type mockJobEventHandler struct {
	events []string
	mu     sync.Mutex
}

func (m *mockJobEventHandler) OnSearchStarted(task SearchTask) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, "OnSearchStarted")
}

func (m *mockJobEventHandler) OnJobListing(task SearchTask, listing JobListing) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, "OnJobListing")
}

func (m *mockJobEventHandler) OnJobDetail(task SearchTask, details JobDetail) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, "OnJobDetail")
}

func (m *mockJobEventHandler) OnPageComplete(task SearchTask, page int, jobsFound int, errors []error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, "OnPageComplete")
}

func (m *mockJobEventHandler) OnSearchCompleted(task SearchTask, totalJobs int, errors []error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, "OnSearchCompleted")
}

func (m *mockJobEventHandler) getEvents() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.events))
	copy(result, m.events)
	return result
}

func TestNewJobEventDispatch(t *testing.T) {
	dispatch := NewJobEventDispatch()
	if dispatch == nil {
		t.Fatal("NewJobEventDispatch() returned nil")
	}

	if got := dispatch.GetHandlerCount(); got != 0 {
		t.Errorf("GetHandlerCount() = %d, want 0", got)
	}
}

func TestRegisterAndDeregister(t *testing.T) {
	dispatch := NewJobEventDispatch()
	handler1 := &mockJobEventHandler{}
	handler2 := &mockJobEventHandler{}

	// Test registration
	dispatch.Register("handler1", handler1)
	if got := dispatch.GetHandlerCount(); got != 1 {
		t.Errorf("GetHandlerCount() after Register = %d, want 1", got)
	}

	dispatch.Register("handler2", handler2)
	if got := dispatch.GetHandlerCount(); got != 2 {
		t.Errorf("GetHandlerCount() after second Register = %d, want 2", got)
	}

	// Test replacing existing handler
	newHandler1 := &mockJobEventHandler{}
	dispatch.Register("handler1", newHandler1)
	if got := dispatch.GetHandlerCount(); got != 2 {
		t.Errorf("GetHandlerCount() after replacing handler = %d, want 2", got)
	}

	// Test deregistration
	if !dispatch.Deregister("handler1") {
		t.Error("Deregister() = false, want true for existing handler")
	}
	if got := dispatch.GetHandlerCount(); got != 1 {
		t.Errorf("GetHandlerCount() after Deregister = %d, want 1", got)
	}

	// Test deregistering non-existent handler
	if dispatch.Deregister("nonexistent") {
		t.Error("Deregister() = true, want false for non-existent handler")
	}
	if got := dispatch.GetHandlerCount(); got != 1 {
		t.Errorf("GetHandlerCount() after failed Deregister = %d, want 1", got)
	}
}

func TestEventDispatching(t *testing.T) {
	dispatch := NewJobEventDispatch()
	handler1 := &mockJobEventHandler{}
	handler2 := &mockJobEventHandler{}

	dispatch.Register("handler1", handler1)
	dispatch.Register("handler2", handler2)

	// Create mock data
	task := SearchTask{ID: "test-task"}
	listing := JobListing{ID: "test-job"}
	detail := JobDetail{ID: "test-detail"}

	// Test OnSearchStarted
	dispatch.OnSearchStarted(task)
	events1 := handler1.getEvents()
	events2 := handler2.getEvents()

	if len(events1) != 1 || events1[0] != "OnSearchStarted" {
		t.Errorf("handler1 events = %v, want [OnSearchStarted]", events1)
	}
	if len(events2) != 1 || events2[0] != "OnSearchStarted" {
		t.Errorf("handler2 events = %v, want [OnSearchStarted]", events2)
	}

	// Test OnJobListing
	dispatch.OnJobListing(task, listing)
	events1 = handler1.getEvents()
	events2 = handler2.getEvents()

	if len(events1) != 2 || events1[1] != "OnJobListing" {
		t.Errorf("handler1 events after OnJobListing = %v, want [OnSearchStarted, OnJobListing]", events1)
	}
	if len(events2) != 2 || events2[1] != "OnJobListing" {
		t.Errorf("handler2 events after OnJobListing = %v, want [OnSearchStarted, OnJobListing]", events2)
	}

	// Test OnJobDetail
	dispatch.OnJobDetail(task, detail)
	events1 = handler1.getEvents()

	if len(events1) != 3 || events1[2] != "OnJobDetail" {
		t.Errorf("handler1 events after OnJobDetail = %v, want [..., OnJobDetail]", events1)
	}

	// Test OnSearchCompleted
	dispatch.OnSearchCompleted(task, 10, nil)
	events1 = handler1.getEvents()

	if len(events1) != 4 || events1[3] != "OnSearchCompleted" {
		t.Errorf("handler1 events after OnSearchCompleted = %v, want [..., OnSearchCompleted]", events1)
	}
}

func TestEventDispatchingWithNoHandlers(t *testing.T) {
	dispatch := NewJobEventDispatch()
	task := SearchTask{ID: "test-task"}
	listing := JobListing{ID: "test-job"}
	detail := JobDetail{ID: "test-detail"}

	// These should not panic even with no handlers
	dispatch.OnSearchStarted(task)
	dispatch.OnJobListing(task, listing)
	dispatch.OnJobDetail(task, detail)
	dispatch.OnSearchCompleted(task, 0, nil)
}

// Test handler that panics
type panicHandler struct{}

func (p *panicHandler) OnSearchStarted(task SearchTask) {
	panic("test panic")
}

func (p *panicHandler) OnJobListing(task SearchTask, listing JobListing) {
	panic("test panic")
}

func (p *panicHandler) OnJobDetail(task SearchTask, details JobDetail) {
	panic("test panic")
}

func (p *panicHandler) OnPageComplete(task SearchTask, page int, jobsFound int, errors []error) {
	panic("test panic")
}

func (p *panicHandler) OnSearchCompleted(task SearchTask, totalJobs int, errors []error) {
	panic("test panic")
}

func TestEventDispatchingWithPanickingHandler(t *testing.T) {
	dispatch := NewJobEventDispatch()
	panicHandler := &panicHandler{}
	normalHandler := &mockJobEventHandler{}

	dispatch.Register("panic", panicHandler)
	dispatch.Register("normal", normalHandler)

	task := SearchTask{ID: "test-task"}

	// These should not panic even if one handler panics
	dispatch.OnSearchStarted(task)

	// Verify the normal handler still got called
	events := normalHandler.getEvents()
	if len(events) != 1 || events[0] != "OnSearchStarted" {
		t.Errorf("normal handler events = %v, want [OnSearchStarted]", events)
	}
}
