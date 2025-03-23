package internal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// CreateTestUser creates a test user with default values.
func CreateTestUser(t *testing.T, client *firestore.Client, name, email, phone string, isSuperUser bool) *User {
	t.Helper()

	now := time.Now()
	user := &User{
		ID:          fmt.Sprintf("test-user-%d", now.UnixNano()),
		CreatedAt:   now,
		Email:       email,
		Name:        name,
		IsSuperUser: isSuperUser,
		Phone:       &phone,
		State: UserState{
			Status:  UserStatusPendingResumeUpload,
			Started: now,
		},
	}

	if err := CreateUser(context.Background(), client, user); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	return user
}

// CreateTestResume creates a test resume for a given user.
func CreateTestResume(t *testing.T, client *firestore.Client, user *User, resumeText, annotatedText string) *Resume {
	t.Helper()

	now := time.Now()
	resume := &Resume{
		ID:        fmt.Sprintf("test-resume-%d", now.UnixNano()),
		RawText:   resumeText,
		Annotated: &annotatedText,
	}

	if err := CreateResume(context.Background(), client, *user, resume); err != nil {
		t.Fatalf("Failed to create test resume: %v", err)
	}

	return resume
}

func CreateTestAppointment(t *testing.T, client *firestore.Client, user *User) *Appointment {
	t.Helper()

	now := time.Now()
	appointment := &Appointment{
		ID: fmt.Sprintf("test-appointment-%d", now.UnixNano()),
	}

	if err := CreateAppointment(context.Background(), client, *user, appointment); err != nil {
		t.Fatalf("Failed to create test appointment: %v", err)
	}

	return appointment
}

// DeleteTestData deletes all test data from Firestore that matches the given prefix.
func DeleteTestData(t *testing.T, client *firestore.Client, prefix string) {
	t.Helper()

	ctx := context.Background()

	// Define the collections to clean up
	collections := []string{"job_searches", "job_listings", "users", "resumes", "appointments"} // Add other collections as needed

	for _, collection := range collections {
		iter := client.Collection(collection).Where("ID", ">=", prefix).Where("ID", "<", prefix+"~").Documents(ctx)
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Fatalf("Failed to iterate over documents in collection %s: %v", collection, err)
			}

			// Delete the document
			if _, err := doc.Ref.Delete(ctx); err != nil {
				t.Logf("Failed to delete document %s from collection %s: %v", doc.Ref.ID, collection, err)
			} else {
				t.Logf("Deleted test document %s from collection %s", doc.Ref.ID, collection)
			}
		}
	}
}
