package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"talent-rodeo/internal"

	"github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// schedulerTestEnv provides a test environment with NATS
type schedulerTestEnv struct {
	js     jetstream.JetStream
	nc     *nats.Conn
	logger *slog.Logger
	ctx    context.Context
}

// testTaskProcessor implements TaskProcessor for testing
type testTaskProcessor struct {
	enqueuedTasks []*internal.Task
}

func (p *testTaskProcessor) EnqueueTask(ctx context.Context, task *internal.Task) error {
	p.enqueuedTasks = append(p.enqueuedTasks, task)
	return nil
}

func deleteConsumers(t *testing.T, stream jetstream.Stream) {
	t.Helper()

	ctx := context.Background()
	consumers := stream.ListConsumers(ctx)
	for info := range consumers.Info() {
		if err := stream.DeleteConsumer(ctx, info.Name); err != nil {
			t.Fatalf("Failed to delete consumer %s: %v", info.Name, err)
		}
	}
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
		deleteConsumers(t, stream)
		if err := js.DeleteStream(ctx, name); err != nil {
			t.Fatalf("Failed to delete stream %s: %v", name, err)
		}
	}
}

func setupschedulerTestEnv(t *testing.T) *schedulerTestEnv {
	t.Helper()

	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create NATS server with JetStream enabled
	opts := test.DefaultTestOptions
	opts.JetStream = true
	srv := test.RunServer(&opts)
	t.Cleanup(srv.Shutdown)

	// Wait for server to be ready
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server failed to start")
	}

	// Connect to NATS
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}
	t.Cleanup(func() { nc.Close() })

	// Create JetStream context
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("Failed to create JetStream context: %v", err)
	}

	deleteStreams(t, js)

	// Create stream
	ctx := context.Background()
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "TEST_STREAM",
		Subjects:  []string{"test.>"},
		Retention: jetstream.InterestPolicy,
	})
	if err != nil {
		t.Fatalf("Failed to create stream: %v", err)
	}

	return &schedulerTestEnv{
		js:     js,
		nc:     nc,
		logger: logger,
		ctx:    ctx,
	}
}

func TestNATSMessageScheduler_TaskType(t *testing.T) {
	env := setupschedulerTestEnv(t)
	handler := NewNATSMessageScheduler(env.js, env.logger)

	got := handler.TaskType()
	want := "nats_message_scheduler"
	if got != want {
		t.Errorf("TaskType() = %q; want %q", got, want)
	}
}

func TestNATSMessageScheduler_ProcessTask(t *testing.T) {
	tests := []struct {
		name    string
		payload NATSMessagePayload
		wantErr bool
	}{
		{
			name: "valid task",
			payload: NATSMessagePayload{
				Subject:  "test.topic",
				Data:     json.RawMessage(`{"key":"value"}`),
				Metadata: map[string]string{"meta": "data"},
			},
			wantErr: false,
		},
		{
			name: "missing subject",
			payload: NATSMessagePayload{
				Data:     json.RawMessage(`{"key":"value"}`),
				Metadata: map[string]string{"meta": "data"},
			},
			wantErr: true,
		},
		{
			name: "missing data",
			payload: NATSMessagePayload{
				Subject:  "test.topic",
				Metadata: map[string]string{"meta": "data"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupschedulerTestEnv(t)
			handler := NewNATSMessageScheduler(env.js, env.logger)

			// Create consumer for verification if not expecting error
			var consumer jetstream.Consumer
			var err error
			if !tt.wantErr {
				_, err = env.js.CreateOrUpdateConsumer(env.ctx, "TEST_STREAM", jetstream.ConsumerConfig{
					Name:          "TEST_CONSUMER",
					FilterSubject: tt.payload.Subject,
					DeliverPolicy: jetstream.DeliverAllPolicy,
				})
				if err != nil {
					t.Fatalf("Failed to create consumer: %v", err)
				}
			}

			payloadBytes, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("Failed to marshal payload: %v", err)
			}

			taskID := time.Now().UnixNano()

			// Create a task
			task := &internal.Task{
				ID:          taskID,
				Type:        handler.TaskType(),
				RunAt:       time.Now(),
				MaxAttempts: 3,
				Payload:     payloadBytes,
			}

			// Process the task
			err = handler.ProcessTask(env.ctx, task)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProcessTask() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			// Wait for message to be available
			ctx, cancel := context.WithTimeout(env.ctx, 5*time.Second)
			defer cancel()

			// Verify message was published by consuming it
			consumer, err = env.js.Consumer(ctx, "TEST_STREAM", "TEST_CONSUMER")
			if err != nil {
				t.Fatalf("Failed to get consumer: %v", err)
			}

			msgs, err := consumer.Fetch(1)
			if err != nil {
				t.Fatalf("Failed to fetch message: %v", err)
			}

			msg := <-msgs.Messages()
			if msg == nil {
				t.Fatal("No message received")
			}

			if msg.Subject() != tt.payload.Subject {
				t.Errorf("message subject = %s, want %s", msg.Subject(), tt.payload.Subject)
			}

			// Verify message content
			var msgData MessageData
			if err := json.Unmarshal(msg.Data(), &msgData); err != nil {
				t.Fatalf("Failed to unmarshal message data: %v", err)
			}

			// Verify the original data was preserved
			if string(msgData.Data) != string(tt.payload.Data) {
				t.Errorf("message data = %s, want %s", string(msgData.Data), string(tt.payload.Data))
			}

			// Verify metadata was preserved
			for k, v := range tt.payload.Metadata {
				if got := msgData.Metadata[k]; got != v {
					t.Errorf("metadata[%q] = %q, want %q", k, got, v)
				}
			}

			// Verify task ID was added to metadata
			if got := msgData.Metadata["task_id"]; got != fmt.Sprintf("%d", taskID) {
				t.Errorf("metadata[task_id] = %q, want %q", got, fmt.Sprintf("%d", taskID))
			}

			// Ack the message
			if err := msg.Ack(); err != nil {
				t.Fatalf("Failed to ack message: %v", err)
			}
		})
	}
}

func TestNATSMessageScheduler_ScheduleMessage(t *testing.T) {
	tests := []struct {
		name      string
		subject   string
		data      interface{}
		metadata  map[string]string
		runAt     time.Time
		wantErr   bool
		taskProc  *testTaskProcessor
		checkTask func(*testing.T, *internal.Task)
	}{
		{
			name:    "valid message",
			subject: "test.topic",
			data:    map[string]string{"key": "value"},
			metadata: map[string]string{
				"meta": "data",
			},
			runAt:   time.Now().Add(time.Hour),
			wantErr: false,
			taskProc: &testTaskProcessor{
				enqueuedTasks: make([]*internal.Task, 0),
			},
			checkTask: func(t *testing.T, task *internal.Task) {
				var payload NATSMessagePayload
				if err := json.Unmarshal(task.Payload, &payload); err != nil {
					t.Errorf("Failed to unmarshal payload: %v", err)
					return
				}
				if payload.Subject != "test.topic" {
					t.Errorf("payload subject = %q, want test.topic", payload.Subject)
				}
				var data map[string]string
				if err := json.Unmarshal(payload.Data, &data); err != nil {
					t.Errorf("Failed to unmarshal data: %v", err)
					return
				}
				if data["key"] != "value" {
					t.Errorf("data[key] = %q, want value", data["key"])
				}
				if payload.Metadata["meta"] != "data" {
					t.Errorf("metadata[meta] = %q, want data", payload.Metadata["meta"])
				}
			},
		},
		{
			name:     "missing subject",
			subject:  "",
			data:     map[string]string{"key": "value"},
			metadata: map[string]string{"meta": "data"},
			runAt:    time.Now().Add(time.Hour),
			wantErr:  true,
			taskProc: &testTaskProcessor{
				enqueuedTasks: make([]*internal.Task, 0),
			},
		},
		{
			name:     "nil data",
			subject:  "test.topic",
			data:     nil,
			metadata: map[string]string{"meta": "data"},
			runAt:    time.Now().Add(time.Hour),
			wantErr:  true,
			taskProc: &testTaskProcessor{
				enqueuedTasks: make([]*internal.Task, 0),
			},
		},
		{
			name:     "past time",
			subject:  "test.topic",
			data:     map[string]string{"key": "value"},
			metadata: map[string]string{"meta": "data"},
			runAt:    time.Now().Add(-time.Hour),
			wantErr:  true,
			taskProc: &testTaskProcessor{
				enqueuedTasks: make([]*internal.Task, 0),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupschedulerTestEnv(t)
			handler := NewNATSMessageScheduler(env.js, env.logger)

			err := handler.ScheduleMessage(env.ctx, tt.subject, tt.data, tt.metadata, tt.runAt, tt.taskProc)
			if (err != nil) != tt.wantErr {
				t.Errorf("ScheduleMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(tt.taskProc.enqueuedTasks) != 1 {
					t.Errorf("Expected 1 enqueued task, got %d", len(tt.taskProc.enqueuedTasks))
					return
				}

				task := tt.taskProc.enqueuedTasks[0]
				if task.Type != handler.TaskType() {
					t.Errorf("Task type = %q, want %q", task.Type, handler.TaskType())
				}
				if task.Status != internal.TaskStatusPending {
					t.Errorf("Task status = %q, want pending", task.Status)
				}
				if task.MaxAttempts != 3 {
					t.Errorf("Task max attempts = %d, want 3", task.MaxAttempts)
				}
				if !task.RunAt.Equal(tt.runAt.UTC()) {
					t.Errorf("Task run_at = %v, want %v", task.RunAt, tt.runAt.UTC())
				}

				if tt.checkTask != nil {
					tt.checkTask(t, task)
				}
			}
		})
	}
}
