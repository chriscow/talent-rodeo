package scrapingdog

/*
// Integration test for FirestoreStore using a real Firestore client
func TestFirestoreStore(t *testing.T) {
	// Get a real Firestore client
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create test prefix for cleanup
	testPrefix := "test-scrapingdog-" + time.Now().Format("20060102-150405")

	fs, cleanup, err := setupFirestoreTest(t, ctx)
	if err != nil {
		t.Fatalf("Failed to setup Firestore test: %v", err)
	}
	defer cleanup()

	// Create a store with the client
	store := NewFirestoreStore(fs,
		WithStoreLogger(slog.Default()),
		WithCacheEnabled(true),
		WithCacheTimeout(1*time.Hour),
	)

	// Create test data
	criteria := search.Criteria{
		Query: "golang",
		GeoID: "remote",
	}

	// Create test jobs
	testJobs := []search.JobListing{
		{
			ID:             testPrefix + "-job-1",
			Provider:       ProviderName,
			ProviderJobID:  "test-provider-job-1",
			Title:          "Senior Go Developer",
			Company:        "Test Company",
			Location:       "Remote",
			PostedDate:     time.Now().Add(-24 * time.Hour),
			URL:            "https://example.com/jobs/1",
			CompanyProfile: "https://example.com/companies/1",
			FirstSeenAt:    time.Now(),
		},
		{
			ID:             testPrefix + "-job-2",
			Provider:       ProviderName,
			ProviderJobID:  "test-provider-job-2",
			Title:          "Go Backend Engineer",
			Company:        "Another Company",
			Location:       "San Francisco",
			PostedDate:     time.Now().Add(-48 * time.Hour),
			URL:            "https://example.com/jobs/2",
			CompanyProfile: "https://example.com/companies/2",
			FirstSeenAt:    time.Now(),
		},
	}

	// Test saving jobs
	t.Run("SaveJobs", func(t *testing.T) {
		err := store.SaveJobs(ctx, criteria, testJobs)
		require.NoError(t, err)
	})

	// Test getting cached jobs
	t.Run("GetCachedJobs", func(t *testing.T) {
		cachedJobs, err := store.GetCachedJobs(ctx, criteria)
		require.NoError(t, err)
		require.NotNil(t, cachedJobs)
		assert.Len(t, cachedJobs, 2)
		assert.Equal(t, testJobs[0].ID, cachedJobs[0].ID)
		assert.Equal(t, testJobs[1].ID, cachedJobs[1].ID)
	})

	// Test job details
	t.Run("JobDetails", func(t *testing.T) {
		// Create test job detail
		jobDetail := search.JobDetail{
			ID:             testPrefix + "-job-detail-1",
			Provider:       ProviderName,
			ProviderJobID:  "test-provider-job-detail-1",
			Title:          "Senior Go Developer",
			Company:        "Test Company",
			Location:       "Remote",
			PostedDate:     time.Now().Add(-24 * time.Hour),
			URL:            "https://example.com/job-detail/1",
			Description:    "This is a test job description for a Senior Go Developer position",
			EmploymentType: "Full-time",
			Status:         "active",
			SeniorityLevel: "Senior",
			JobFunction:    "Engineering",
			Industry:       "Software",
			CreatedAt:      time.Now(),
		}

		// Test saving job details
		err := store.SaveJobDetails(ctx, jobDetail)
		require.NoError(t, err)

		// Test getting job details
		retrievedDetails, err := store.GetJobDetails(ctx, jobDetail.ID)
		require.NoError(t, err)
		require.NotNil(t, retrievedDetails)
		assert.Equal(t, jobDetail.ID, retrievedDetails.ID)
		assert.Equal(t, jobDetail.Title, retrievedDetails.Title)
		assert.Equal(t, jobDetail.Company, retrievedDetails.Company)
		assert.Equal(t, jobDetail.Description, retrievedDetails.Description)
	})

	// Test cache timeout
	t.Run("CacheExpiration", func(t *testing.T) {
		// Create a store with a very short timeout
		shortStore := NewFirestoreStore(fs,
			WithCacheTimeout(1*time.Millisecond),
		)

		// Save test jobs
		shortCriteria := search.Criteria{
			Query: "expired",
			GeoID: "expired location",
		}

		err := shortStore.SaveJobs(ctx, shortCriteria, testJobs)
		require.NoError(t, err)

		// Wait for cache to expire
		time.Sleep(10 * time.Millisecond)

		// Check that cache is expired
		cachedJobs, err := shortStore.GetCachedJobs(ctx, shortCriteria)
		require.NoError(t, err)
		assert.Nil(t, cachedJobs, "Cache should be expired")
	})

	// Test cache disabled
	t.Run("CacheDisabled", func(t *testing.T) {
		// Create a store with cache disabled
		disabledStore := NewFirestoreStore(fs,
			WithCacheEnabled(false),
		)

		// Save test jobs - this should be a no-op
		disabledCriteria := search.Criteria{
			Query: "disabled",
			GeoID: "disabled location",
		}

		err := disabledStore.SaveJobs(ctx, disabledCriteria, testJobs)
		require.NoError(t, err)

		// Verify that cache was not actually saved
		// We need to use a different store with cache enabled to verify
		enabledStore := NewFirestoreStore(fs)
		cachedJobs, err := enabledStore.GetCachedJobs(ctx, disabledCriteria)
		require.NoError(t, err)
		assert.Nil(t, cachedJobs, "No jobs should be in cache when cache disabled")
	})
}

// Helper function to set up a Firestore test
func setupFirestoreTest(t *testing.T, ctx context.Context) (*internal.FirestoreClient, func(), error) {
	t.Helper()

	// Use the emulator client
	fs, err := internal.NewTestFirestoreClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Create a cleanup function to delete test data
	cleanup := func() {
		t.Logf("Cleaning up test data...")
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Delete test collections to clean up
		collections := []string{jobListingsCollection, jobDetailsCollection}
		for _, collection := range collections {
			// Get all documents in the collection that match our test prefix
			iter := fs.Collection(collection).Where("ID", ">=", "test-scrapingdog-").Where("ID", "<=", "test-scrapingdog-~").Documents(cleanupCtx)
			docs, err := iter.GetAll()
			if err != nil {
				t.Logf("Failed to get documents for cleanup: %v", err)
				continue
			}

			// Delete each document
			for _, doc := range docs {
				_, err := doc.Ref.Delete(cleanupCtx)
				if err != nil {
					t.Logf("Failed to delete document %s: %v", doc.Ref.ID, err)
				}
			}
		}

		fs.Close()
	}

	return fs, cleanup, nil
}
*/
