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
	}

	if err := CreateUser(context.Background(), client, user); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	return user
}

// CreateTestUserJob creates a test job for a given user.
func CreateTestUserJob(t *testing.T, client *FirestoreClient, userID string, jobID string, status JobStatus, createdAt time.Time) *UserJob {
	t.Helper()

	job, err := CreateUserJob(context.Background(), client, userID, jobID)
	if err != nil {
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

	if err := CreateAppointment(context.Background(), client, user.ID, appointment); err != nil {
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
			}
		}

		// Finally delete the user document
		if _, err := doc.Ref.Delete(ctx); err != nil {
			t.Logf("Failed to delete user document %s: %v", doc.Ref.ID, err)
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

func DeleteTestCriteria(t *testing.T, client *FirestoreClient, userID string) {
	t.Helper()

	ctx := context.Background()
	iter := client.Collection(usersCollection).Doc(userID).Collection(userCriteriaCollection).Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Logf("Failed to iterate over criteria: %v", err)
			continue
		}

		if _, err := doc.Ref.Delete(ctx); err != nil {
			t.Logf("Failed to delete criteria %s: %v", doc.Ref.ID, err)
		} else {
			t.Logf("Deleted test criteria %s", doc.Ref.ID)
		}

	}
}

// deleteTestUserCriteria deletes test user criteria records for cleanup
func DeleteTestUserCriteria(ctx context.Context, t *testing.T, fs *FirestoreClient, userID string) {
	t.Helper()

	coll := fs.Collection(userCriteriaCollection)

	// Query for user criteria with the specified userID
	iter := coll.Where("UserID", "==", userID).Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Logf("Failed to iterate through test user criteria: %v", err)
			continue
		}

		_, err = doc.Ref.Delete(ctx)
		if err != nil {
			t.Logf("Failed to delete test user criteria %s: %v", doc.Ref.ID, err)
		}
	}
}

// deleteTestSearchCriteria deletes test search criteria for cleanup
func DeleteTestSearchCriteria(ctx context.Context, t *testing.T, fs *FirestoreClient, prefix string) {
	t.Helper()

	coll := fs.Collection(searchCriteriaCollection)

	// Query for criteria with ID starting with the prefix
	iter := coll.Where("ID", ">=", prefix).Where("ID", "<=", prefix+"\uf8ff").Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Logf("Failed to iterate through test criteria: %v", err)
			continue
		}

		_, err = doc.Ref.Delete(ctx)
		if err != nil {
			t.Logf("Failed to delete test criteria %s: %v", doc.Ref.ID, err)
		}
	}
}

// DeleteTestUserCriteriaByPrefix deletes test user criteria records that match the given prefix
func DeleteTestUserCriteriaByPrefix(ctx context.Context, t *testing.T, fs *FirestoreClient, prefix string) {
	t.Helper()

	coll := fs.Collection(userCriteriaCollection)

	// Query for user criteria with IDs starting with the prefix
	iter := coll.Where("ID", ">=", prefix).Where("ID", "<=", prefix+"\uf8ff").Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Logf("Failed to iterate through test user criteria: %v", err)
			continue
		}

		_, err = doc.Ref.Delete(ctx)
		if err != nil {
			t.Logf("Failed to delete test user criteria %s: %v", doc.Ref.ID, err)
		}
	}
}

// ResetUserState resets a user's state to UserStatusPendingResumeUpload and cleans up their data.
// This is only meant to be used in testing scenarios.
func ResetUserState(ctx context.Context, fs *FirestoreClient, user *User) error {
	// Reset the user's state using the proper UpdateUserStatus function
	if err := UpdateUserStatus(ctx, fs, user, UserStatusPendingResumeUpload); err != nil {
		return fmt.Errorf("failed to reset user state: %w", err)
	}

	// Delete all appointments for the user
	appointments, err := ListAppointments(ctx, fs, user.ID)
	if err != nil {
		return fmt.Errorf("failed to list appointments: %w", err)
	}

	for _, appointment := range appointments {
		if err := DeleteAppointment(ctx, fs, user.ID, appointment.ID); err != nil {
			return fmt.Errorf("failed to delete appointment: %w", err)
		}
	}

	// Delete all resumes for the user
	resumes, err := ListResumes(ctx, fs, user.ID)
	if err != nil {
		return fmt.Errorf("failed to list resumes: %w", err)
	}

	for _, resume := range resumes {
		if err := DeleteResume(ctx, fs, user.ID, resume.ID); err != nil {
			return fmt.Errorf("failed to delete resume: %w", err)
		}
	}

	return nil
}

func StringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
