package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/tasks"
	th "talent-rodeo/internal/tasks/handlers"

	"github.com/alexedwards/scs/v2"
	"github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/pusher/pusher-http-go/v5"
)

func TestMain(m *testing.M) {
	// Start emulator
	if err := internal.StartEmulator(); err != nil {
		log.Fatalf("Failed to start emulator: %v", err)
	}

	// Ensure emulator is stopped after tests
	defer internal.StopEmulator()

	// Run tests
	code := m.Run()

	os.Exit(code)
}

type testServer struct {
	*testing.T
	app        *application
	adminUser1 *internal.User
	user1      *internal.User
	resume1    *internal.Resume
}

// deleteStreams removes all streams from the JetStream server
func deleteStreams(t *testing.T, js jetstream.JetStream) {
	t.Helper()

	ctx := context.Background()
	streams := js.StreamNames(ctx)
	for name := range streams.Name() {
		stream, err := js.Stream(ctx, name)
		if err != nil {
			t.Fatalf("Failed to get stream %s: %v", name, err)
		}
		consumers := stream.ListConsumers(ctx)
		for info := range consumers.Info() {
			if err := stream.DeleteConsumer(ctx, info.Name); err != nil {
				t.Fatalf("Failed to delete consumer %s: %v", info.Name, err)
			}
		}
		if err := js.DeleteStream(ctx, name); err != nil {
			t.Fatalf("Failed to delete stream %s: %v", name, err)
		}
	}
}

func newTestServer(t *testing.T, name string) *testServer {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Initialize Firestore client based on test mode
	var fs *internal.FirestoreClient
	var err error
	if testing.Short() {
		fs, err = internal.NewTestFirestoreClient(context.Background())
		if err != nil {
			t.Fatalf("Failed to create test firestore client: %v", err)
		}
	} else {
		fs, err = internal.NewFirestoreClient(context.Background())
		if err != nil {
			t.Fatalf("Failed to create firestore client: %v", err)
		}
	}

	// Create test session manager
	sessionManager := scs.New()
	sessionManager.Lifetime = 24 * time.Hour

	// Create a mock pusher client
	pusherClient := pusher.Client{
		AppID:   "test-app-id",
		Key:     "test-key",
		Secret:  "test-secret",
		Cluster: "test-cluster",
		Secure:  true,
	}

	// Create a task processor
	taskProcessor := tasks.NewProcessor(tasks.NewFirestoreTaskStorage(fs), logger)
	taskProcessor.RegisterHandler(th.NewWebhookScheduler(logger, fs))

	// Create NATS server with JetStream enabled
	opts := test.DefaultTestOptions
	opts.JetStream = true
	srv := test.RunServer(&opts)
	t.Cleanup(func() {
		logger.Debug("cleaning up test NATS server")
		srv.Shutdown()
	})

	// Wait for server to be ready
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server failed to start")
	}

	// Connect to NATS
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}

	// Create JetStream context
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("Failed to create JetStream context: %v", err)
	}

	// Delete any existing streams
	deleteStreams(t, js)

	slug := funcs.Slugify(name)

	// Create stream
	_, err = js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:      funcs.Slugify("RODEO-TEST-" + strings.ToUpper(name)),
		Subjects:  []string{slug + ".rodeo.>", slug + ".test.>"},
		Retention: jetstream.InterestPolicy,
	})
	if err != nil {
		t.Fatalf("Failed to create stream: %v", err)
	}

	/* KEEP THIS COMMENTED OUT FOR NOW
	// Create a scheduler
	scheduler := th.NewNATSMessageScheduler(js, logger)

	// Register the scheduler with the processor
	taskProcessor.RegisterHandler(scheduler)
	*/

	app := &application{
		config: config{
			baseURL:  "http://localhost:2304",
			httpPort: 2304,
			testing:  true,
		},
		logger:         logger,
		sessionManager: sessionManager,
		// db:             db,
		fs:           fs,
		pusher:       &pusherClient,
		processor:    taskProcessor,
		natsShutdown: make(chan struct{}),
		js:           js,
		// natsServer:     srv,
	}

	// Create default test users
	adminUser1 := internal.CreateTestUser(t, app.fs, slug+"-admin", "admin@example.com", "+14251234567", true)
	user1 := internal.CreateTestUser(t, app.fs, slug+"-candidate", "candidate@example.com", "+14251234567", false)
	resume1 := internal.CreateTestResume(t, app.fs, user1, "test-resume-content", "test-resume-annotated")

	ts := &testServer{
		T:          t,
		app:        app,
		adminUser1: adminUser1,
		user1:      user1,
		resume1:    resume1,
	}

	return ts
}

func (ts *testServer) get(t *testing.T, urlPath string) (int, http.Header, string) {
	t.Helper()

	w := httptest.NewRecorder()
	r, err := http.NewRequest(http.MethodGet, urlPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Load session data into context
	ctx, err := ts.app.sessionManager.Load(r.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	r = r.WithContext(ctx)

	ts.app.routes().ServeHTTP(w, r)
	return w.Code, w.Header(), w.Body.String()
}

func (ts *testServer) authenticatedRequest(t *testing.T, method, urlPath string, user *internal.User, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, urlPath, body)

	// Create a new session and add it to the context
	ctx, err := ts.app.sessionManager.Load(r.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	r = r.WithContext(ctx)

	// Store user ID in session
	if user != nil {
		ts.app.sessionManager.Put(r.Context(), "UserID", user.ID)
	}

	// Add user to request context
	if user != nil {
		r = contextSetAuthenticatedUser(r, user)
	}

	// Add CSRF token for POST requests
	if method == "POST" {
		// Store the CSRF token in the session
		ts.app.sessionManager.Put(r.Context(), "csrf_token", "test-csrf-token")

		// Add CSRF token to request header and cookie
		r.Header.Set("X-CSRF-Token", "test-csrf-token")
		cookie := &http.Cookie{
			Name:     "csrf_token",
			Value:    "test-csrf-token",
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		}
		r.AddCookie(cookie)
	}

	return r
}

func (ts *testServer) postForm(_ *testing.T, urlPath string, form url.Values) (int, http.Header, string) {
	req := httptest.NewRequest("POST", urlPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Add CSRF token for POST requests
	if req.Method == "POST" {
		req.Header.Set("X-CSRF-Token", "test-csrf-token")
		cookie := &http.Cookie{
			Name:     "csrf_token",
			Value:    "test-csrf-token",
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		}
		req.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	ts.app.routes().ServeHTTP(w, req)
	return w.Code, w.Header(), w.Body.String()
}

func (ts *testServer) authenticatedPostForm(t *testing.T, user *internal.User, urlPath string, form url.Values) (int, http.Header, string) {
	r := ts.authenticatedRequest(t, "POST", urlPath, user, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	ts.app.routes().ServeHTTP(w, r)
	return w.Code, w.Header(), w.Body.String()
}

func (ts *testServer) authenticatedGet(t *testing.T, urlPath string) (int, http.Header, string) {
	r := ts.authenticatedRequest(t, "GET", urlPath, ts.user1, nil)
	w := httptest.NewRecorder()
	ts.app.routes().ServeHTTP(w, r)
	return w.Code, w.Header(), w.Body.String()
}

func (ts *testServer) Cleanup() {
	internal.DeleteTestData(ts.T, ts.app.fs, "test-user-")
	internal.DeleteTestData(ts.T, ts.app.fs, "test-resume-")
	ts.app.fs.Close()
}

// CreateTestJob creates a test job for the given user
func (ts *testServer) CreateTestJob(t *testing.T, user *internal.User, status internal.JobStatus) *internal.UserJob {
	t.Helper()

	now := time.Now()
	job := &internal.UserJob{
		ID:        fmt.Sprintf("test-job-%d", time.Now().UnixNano()),
		Status:    status,
		JobRef:    ts.app.fs.Collection("jobs").Doc("test-job"),
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := ts.app.fs.Collection("users").Doc(user.ID).Collection("jobs").Doc(job.ID).Set(context.Background(), job)
	if err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}

	return job
}

// CreateTestAppointment creates a test appointment for the given user
func (ts *testServer) CreateTestAppointment(t *testing.T, user *internal.User, status internal.AppointmentStatus) *internal.Appointment {
	t.Helper()

	appt := &internal.Appointment{
		ID:            fmt.Sprintf("test-appt-%d", time.Now().UnixNano()),
		UserID:        user.ID,
		ScheduledTime: time.Now().Add(24 * time.Hour),
		Status:        status,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	_, err := ts.app.fs.Collection("appointments").Doc(appt.ID).Set(context.Background(), appt)
	if err != nil {
		t.Fatalf("Failed to create test appointment: %v", err)
	}

	return appt
}
