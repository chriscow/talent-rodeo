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
		Search: &UserSearch{}, // Initialize empty search
	}

	t.Logf("Creating user with ID: %s, Email: %s", user.ID, user.Email)

	if err := CreateUser(context.Background(), client, user); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	t.Logf("Successfully created user with ID: %s", user.ID)

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

	now := time.Now().UTC()
	appointment := &Appointment{
		ID:            fmt.Sprintf("test-appointment-%d", now.UnixNano()),
		UserID:        user.ID,
		ScheduledTime: now.Add(24 * time.Hour).Round(time.Second),
		Timezone:      "UTC",
		Status:        AppointmentStatusScheduled,
		CreatedAt:     now.Round(time.Second),
		UpdatedAt:     now.Round(time.Second),
	}

	if err := CreateAppointment(context.Background(), client, "test-appointment", *user, appointment); err != nil {
		t.Fatalf("Failed to create test appointment: %v", err)
	}

	return appointment
}

// DeleteTestData deletes all test data from Firestore that matches the given prefix.
func DeleteTestData(t *testing.T, client *firestore.Client, prefix string) {
	t.Helper()

	ctx := context.Background()

	// First get all test users
	iter := client.Collection(usersCollection).Where("id", ">=", prefix).Where("id", "<", prefix+"~").Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("Failed to iterate over documents in users collection: %v", err)
		}

		// Delete all appointments in the user's subcollection
		appointmentsIter := doc.Ref.Collection(appointmentsCollection).Documents(ctx)
		for {
			appointmentDoc, err := appointmentsIter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Logf("Failed to iterate over appointments in user %s: %v", doc.Ref.ID, err)
				continue
			}

			if _, err := appointmentDoc.Ref.Delete(ctx); err != nil {
				t.Logf("Failed to delete appointment %s for user %s: %v", appointmentDoc.Ref.ID, doc.Ref.ID, err)
			} else {
				t.Logf("Deleted test appointment %s for user %s", appointmentDoc.Ref.ID, doc.Ref.ID)
			}
		}

		// Delete all resumes in the user's subcollection
		resumesIter := doc.Ref.Collection(resumeCollection).Documents(ctx)
		for {
			resumeDoc, err := resumesIter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Logf("Failed to iterate over resumes in user %s: %v", doc.Ref.ID, err)
				continue
			}

			if _, err := resumeDoc.Ref.Delete(ctx); err != nil {
				t.Logf("Failed to delete resume %s for user %s: %v", resumeDoc.Ref.ID, doc.Ref.ID, err)
			} else {
				t.Logf("Deleted test resume %s for user %s", resumeDoc.Ref.ID, doc.Ref.ID)
			}
		}

		// Finally delete the user document
		if _, err := doc.Ref.Delete(ctx); err != nil {
			t.Logf("Failed to delete user document %s: %v", doc.Ref.ID, err)
		} else {
			t.Logf("Deleted test user %s", doc.Ref.ID)
		}
	}

	// Clean up any other top-level collections
	otherCollections := []string{"job_searches", "job_listings", "job_details"} // Add other top-level collections as needed
	for _, collection := range otherCollections {
		iter := client.Collection(collection).Where("id", ">=", prefix).Where("id", "<", prefix+"~").Documents(ctx)
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Fatalf("Failed to iterate over documents in collection %s: %v", collection, err)
			}

			if _, err := doc.Ref.Delete(ctx); err != nil {
				t.Logf("Failed to delete document %s from collection %s: %v", doc.Ref.ID, collection, err)
			} else {
				t.Logf("Deleted test document %s from collection %s", doc.Ref.ID, collection)
			}
		}
	}
}

// DeleteTestAppointments deletes all appointments for a given user.
func DeleteTestAppointments(t *testing.T, client *firestore.Client, userID string) {
	t.Helper()

	ctx := context.Background()
	iter := client.Collection(usersCollection).Doc(userID).Collection(appointmentsCollection).Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Logf("Failed to iterate over appointments: %v", err)
			continue
		}

		if _, err := doc.Ref.Delete(ctx); err != nil {
			t.Logf("Failed to delete appointment %s: %v", doc.Ref.ID, err)
		} else {
			t.Logf("Deleted test appointment %s", doc.Ref.ID)
		}
	}
}
