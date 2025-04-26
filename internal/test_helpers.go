package internal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/api/iterator"
)

// CreateTestUser creates a test user with default values.
func CreateTestUser(t *testing.T, client *FirestoreClient, name, email, phone string, isSuperUser bool) *User {
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

	if err := CreateUser(context.Background(), client, user); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	return user
}

// CreateTestJob creates a test job for a given user.
func CreateTestJob(t *testing.T, client *FirestoreClient, userID string, jobID string, status JobStatus, createdAt time.Time) *UserJob {
	t.Helper()

	job := &UserJob{
		ID:         jobID,
		SearchHash: fmt.Sprintf("test-search-hash-%s", jobID),
		JobRef:     client.Collection("job_details").Doc(jobID),
		Status:     status,
		CreatedAt:  createdAt,
		UpdatedAt:  createdAt,
	}

	if err := CreateUserJob(context.Background(), client, userID, job); err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}

	return job
}

// CreateTestResume creates a test resume for a given user.
func CreateTestResume(t *testing.T, client *FirestoreClient, user *User, resumeText, annotatedText string) *Resume {
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

func CreateTestAppointment(t *testing.T, client *FirestoreClient, user *User) *Appointment {
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
func DeleteTestData(t *testing.T, client *FirestoreClient, prefix string) {
	t.Helper()

	ctx := context.Background()

	// First get all test users
	iter := client.Collection(usersCollection).Where("ID", ">=", prefix).Where("ID", "<", prefix+"~").Documents(ctx)
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

		// Delete all jobs in the user's subcollection
		jobsIter := doc.Ref.Collection(userJobsCollection).Documents(ctx)
		for {
			jobDoc, err := jobsIter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Logf("Failed to iterate over jobs in user %s: %v", doc.Ref.ID, err)
				continue
			}

			if _, err := jobDoc.Ref.Delete(ctx); err != nil {
				t.Logf("Failed to delete job %s for user %s: %v", jobDoc.Ref.ID, doc.Ref.ID, err)
			} else {
				t.Logf("Deleted test job %s for user %s", jobDoc.Ref.ID, doc.Ref.ID)
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
		iter := client.Collection(collection).Where("ID", ">=", prefix).Where("ID", "<", prefix+"~").Documents(ctx)
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
func DeleteTestAppointments(t *testing.T, client *FirestoreClient, userID string) {
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
