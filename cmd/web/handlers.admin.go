package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/cloudflare"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/utils"
	"time"

	"github.com/pinecone-io/go-pinecone/v3/pinecone"
)

func (app *application) adminVoice(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	adminUser := contextGetAuthenticatedUser(r)
	data := app.newTemplateData(r)
	data["VapiAPIKey"] = os.Getenv("VAPI_PUBLIC_API_KEY")
	data["AssistantID"] = os.Getenv("VAPI_ASSISTANT_ID")

	// Fetch all users for the dropdown
	users, err := internal.ListUsers(ctx, app.fs, 100, 0) // Limiting to 100 users for now
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data["Users"] = users

	// Handle POST request for user selection
	if r.Method == http.MethodPost {
		app.logger.Info("handling POST request", "url", r.URL.String())

		if err := r.ParseForm(); err != nil {
			app.logger.Error("failed to parse form", "error", err)
			app.serverError(w, r, fmt.Errorf("failed to parse form: %w", err))
			return
		}

		selectedUserID := r.PostForm.Get("user_id")
		app.logger.Info("received form data", "user_id", selectedUserID)

		if selectedUserID == "" {
			app.logger.Error("no user selected")
			http.Error(w, "No user selected", http.StatusBadRequest)
			return
		}

		data["SelectedUserID"] = selectedUserID

		// Handle the "no user" case
		if selectedUserID == "_none" {
			app.logger.Info("handling _none user case")
			// Build assistant overrides with nil user and nil resume
			overrides, err := internal.BuildAssistantOverrides(ctx, nil, nil, app.fs, app.logger)
			if err != nil {
				app.logger.Error("failed to build assistant overrides", "error", err)
				app.serverError(w, r, fmt.Errorf("failed to build assistant overrides: %w", err))
				return
			}
			data["AssistantOverrides"] = overrides
		} else {
			app.logger.Info("fetching user details", "user_id", selectedUserID)
			// Fetch the selected user's details
			selectedUser, found, err := internal.GetUser(ctx, app.fs, selectedUserID)
			if err != nil {
				app.logger.Error("failed to get user", "error", err, "user_id", selectedUserID)
				app.serverError(w, r, fmt.Errorf("failed to get user: %w", err))
				return
			}
			if !found {
				app.logger.Error("user not found", "user_id", selectedUserID)
				http.Error(w, "User not found", http.StatusNotFound)
				return
			}

			// Fetch the user's latest resume
			resume, _, err := internal.GetLatestResume(ctx, app.fs, selectedUserID)
			if err != nil {
				app.logger.Warn("failed to get resume", "error", err, "user_id", selectedUserID)
				// Continue without resume - it's optional
			}

			// Build assistant overrides for the selected user
			overrides, err := internal.BuildAssistantOverrides(ctx, selectedUser, resume, app.fs, app.logger)
			if err != nil {
				app.logger.Error("failed to build assistant overrides", "error", err, "user_id", selectedUserID)
				app.serverError(w, r, fmt.Errorf("failed to build assistant overrides: %w", err))
				return
			}

			// call.name is the appointment ID
			// call.customer.name is the customer ID
			apptName := funcs.Slugify(fmt.Sprintf("admin-%s", adminUser.Name))
			appt := &internal.Appointment{
				UserID:        selectedUser.ID,
				Name:          apptName,
				ScheduledTime: time.Now(),
				Status:        internal.AppointmentStatusRunning,
			}
			if err := internal.CreateAppointment(ctx, app.fs, selectedUser.ID, appt); err != nil {
				app.logger.Error("failed to create appointment", "error", err, "user_id", selectedUserID)
				app.serverError(w, r, fmt.Errorf("failed to create appointment: %w", err))
				return
			}

			data["CallName"] = appt.ID
			data["CallCustomerName"] = selectedUser.ID
			data["AssistantOverrides"] = overrides
			data["Prompt"] = overrides.Model.Messages[0].Content
		}
	}

	if err := response.Page(w, http.StatusOK, data, "pages/admin/voice.html"); err != nil {
		app.logger.Error("failed to render page", "error", err)
		app.serverError(w, r, err)
	}
}

func (app *application) adminCompareJobs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := app.newTemplateData(r)

	var (
		searchTaskID string
		leftJobID    string
		rightJobID   string
		jobDetails   []*internal.JobDetail
	)

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			app.serverError(w, r, err)
			return
		}
		searchTaskID = r.PostForm.Get("search_task_id")
		leftJobID = r.PostForm.Get("left_job_id")
		rightJobID = r.PostForm.Get("right_job_id")

		if searchTaskID != "" {
			var err error
			jobDetails, err = internal.ListJobDetailsForSearchTask(ctx, app.fs, searchTaskID)
			if err != nil {
				data["Error"] = err.Error()
			} else {
				data["JobDetails"] = jobDetails
				data["SearchTaskID"] = searchTaskID
				data["LeftJobID"] = leftJobID
				data["RightJobID"] = rightJobID

				// Find selected jobs for display
				var leftJob, rightJob *internal.JobDetail
				for _, job := range jobDetails {
					if job.ID == leftJobID {
						leftJob = job
					}
					if job.ID == rightJobID {
						rightJob = job
					}
				}
				data["LeftJob"] = leftJob
				data["RightJob"] = rightJob

				// Calculate similarity if both jobs are selected
				if leftJobID != "" && rightJobID != "" {
					// Get Pinecone client
					pineconeClient, err := internal.NewPineconeClient()
					if err != nil {
						app.logger.Error("failed to create Pinecone client", "error", err)
						data["Error"] = "Failed to initialize Pinecone: " + err.Error()
					} else {
						// Get index host from environment or config
						indexHost := os.Getenv("PINECONE_DENSE_INDEX_HOST")
						if indexHost == "" {
							app.logger.Error("PINECONE_DENSE_INDEX_HOST environment variable not set")
							data["Error"] = "PINECONE_DENSE_INDEX_HOST environment variable not set"
						} else {
							// Fetch vectors for both jobs
							jobIDs := []string{leftJobID, rightJobID}
							app.logger.Info("fetching vectors from Pinecone",
								"index_host", indexHost,
								"job_ids", jobIDs)

							// Try with default namespace first
							fetchResp, err := pineconeClient.FetchVectors(ctx, indexHost, internal.JobDetailNamespace, jobIDs)
							if err != nil {
								app.logger.Error("failed to fetch vectors", "error", err)
								data["Error"] = "Failed to fetch job vectors: " + err.Error()
							} else {
								app.logger.Info("fetched vectors from Pinecone",
									"vector_count", len(fetchResp.Vectors),
									"namespace", fetchResp.Namespace)

								if len(fetchResp.Vectors) == 2 {
									leftVector := fetchResp.Vectors[leftJobID]
									rightVector := fetchResp.Vectors[rightJobID]

									// Calculate cosine similarity
									similarity, err := internal.CosineSimilarity(leftVector, rightVector)
									if err != nil {
										app.logger.Error("failed to calculate similarity", "error", err)
										data["Error"] = "Failed to calculate similarity: " + err.Error()
									} else {
										// Format similarity to 4 decimal places
										data["Similarity"] = fmt.Sprintf("%.4f", similarity)
									}
								} else {
									// Fallback to text similarity if vectors not found
									app.logger.Info("vectors not found in Pinecone, falling back to text similarity")

									// Calculate simple Jaccard similarity as fallback
									if leftJob != nil && rightJob != nil && leftJob.Description != "" && rightJob.Description != "" {
										// Use a simple text-based similarity
										jaccardSim := calculateJaccardSimilarity(leftJob.Description, rightJob.Description)
										data["Similarity"] = fmt.Sprintf("%.4f (text based)", jaccardSim)
										data["SimilarityMethod"] = "text"
									} else {
										app.logger.Error("could not calculate text similarity, missing job descriptions")
										data["Error"] = "Could not calculate similarity: vectors not found and job descriptions missing"
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if err := response.Page(w, http.StatusOK, data, "pages/admin/compare-jobs.html"); err != nil {
		app.serverError(w, r, err)
	}
}

// calculateJaccardSimilarity computes a simple text similarity metric as a fallback
// when vector embeddings are not available. It uses the Jaccard index (intersection over union)
// on the sets of words in each description.
func calculateJaccardSimilarity(text1, text2 string) float64 {
	// Simple tokenization by splitting on whitespace
	words1 := make(map[string]struct{})
	words2 := make(map[string]struct{})

	// Convert to lowercase and split
	for _, word := range strings.Fields(strings.ToLower(text1)) {
		// Skip very short words and basic punctuation
		if len(word) > 2 {
			words1[word] = struct{}{}
		}
	}

	for _, word := range strings.Fields(strings.ToLower(text2)) {
		if len(word) > 2 {
			words2[word] = struct{}{}
		}
	}

	// Count intersection
	intersection := 0
	for word := range words1 {
		if _, exists := words2[word]; exists {
			intersection++
		}
	}

	// Calculate union
	union := len(words1) + len(words2) - intersection

	// Avoid division by zero
	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

func (app *application) adminSearchJobs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := app.newTemplateData(r)

	// Fetch all search criteria for the dropdown
	criteriaList, err := internal.ListSearchCriteria(r.Context(), app.fs)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data["SearchCriteriaList"] = criteriaList

	selectedCriteriaID := ""

	// Default params
	params := internal.DefaultHybridSearchParams()

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			app.serverError(w, r, err)
			return
		}

		searchQuery := r.PostForm.Get("search_query")
		topKStr := r.PostForm.Get("top_k")
		selectedCriteriaID = r.PostForm.Get("criteria_id")
		data["SelectedCriteriaID"] = selectedCriteriaID
		app.logger.Debug("adminSearchJobs", "selectedCriteriaID", selectedCriteriaID)

		// Set default value if not provided
		topK := 10
		if topKStr != "" {
			var err error
			topK, err = strconv.Atoi(topKStr)
			if err != nil {
				data["Error"] = "Invalid top_k value: " + err.Error()
				topK = 10
			}
		}

		// Parse HybridSearchParams fields from form
		if v := r.PostForm.Get("dense_weight"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				params.DenseWeight = f
			}
		}
		if v := r.PostForm.Get("sparse_weight"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				params.SparseWeight = f
			}
		}
		if v := r.PostForm.Get("score_fusion"); v != "" {
			params.ScoreFusion = v
		}
		if v := r.PostForm.Get("recency_weight"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				params.RecencyWeight = f
			}
		}
		if v := r.PostForm.Get("recency_half_life"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				params.RecencyHalfLife = f
			}
		}
		// Add a boolean for enabling reranking
		enableReranking := r.PostForm.Get("enable_reranking") == "on"
		data["EnableReranking"] = enableReranking

		data["SearchQuery"] = searchQuery
		data["TopK"] = topK
		data["HybridParams"] = params
		data["EnableReranking"] = enableReranking // Add to data for template rendering

		if searchQuery != "" {

			results, err := app.semanticSearchJobsWithParams(ctx, searchQuery, topK, selectedCriteriaID, params, enableReranking)
			if err != nil {
				app.logger.Error("failed to search jobs", "error", err)
				data["Error"] = "Failed to \tsearch jobs: " + err.Error()
			} else {
				data["SearchResults"] = results
			}
		}
	}
	// For GET, pre-fill with defaults
	if r.Method != http.MethodPost {
		data["HybridParams"] = params
	}

	if err := response.Page(w, http.StatusOK, data, "pages/admin/search-jobs.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) semanticSearchJobs(ctx context.Context, searchQuery string, topK int, selectedCriteriaID string) (*pinecone.RerankResponse, error) {

	client, err := internal.NewPineconeClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Pinecone client: %w", err)
	}
	// Build filter map
	filter := map[string]any{}
	if selectedCriteriaID != "" {
		filter["criteriaID"] = map[string]any{
			"$eq": selectedCriteriaID,
		}
	}

	denseIndex := os.Getenv("PINECONE_DENSE_INDEX_HOST")
	if denseIndex == "" {
		return nil, fmt.Errorf("PINECONE_DENSE_INDEX_HOST environment variable not set")
	}
	denseResults, err := client.SearchRecords(ctx, denseIndex, internal.JobDetailNamespace, searchQuery,
		internal.WithTopK(topK*2), internal.WithFilter(filter))
	if err != nil {
		return nil, fmt.Errorf("failed to search dense index: %w", err)
	}

	sparseIndex := os.Getenv("PINECONE_SPARSE_INDEX_HOST")
	if sparseIndex == "" {
		return nil, fmt.Errorf("PINECONE_SPARSE_INDEX_HOST environment variable not set")
	}
	sparseResults, err := client.SearchRecords(ctx, sparseIndex, internal.JobDetailNamespace, searchQuery,
		internal.WithTopK(topK*2), internal.WithFilter(filter))
	if err != nil {
		return nil, fmt.Errorf("failed to search sparse index: %w", err)
	}

	merged := make(map[string]pinecone.Document)
	for _, hit := range denseResults.Result.Hits {
		text, ok := hit.Fields["text"].(string)
		if !ok || text == "" {
			continue // skip if not a valid string
		}
		merged[hit.Id] = pinecone.Document{
			"ID":       hit.Id,
			"Score":    hit.Score,
			"Text":     hit.Fields["text"],
			"Title":    hit.Fields["title"],
			"Company":  hit.Fields["company"],
			"Location": hit.Fields["location"],
		}
	}
	for _, hit := range sparseResults.Result.Hits {
		text, ok := hit.Fields["text"].(string)
		if !ok || text == "" {
			continue // skip if not a valid string
		}
		merged[hit.Id] = pinecone.Document{
			"ID":       hit.Id,
			"Score":    hit.Score,
			"Text":     hit.Fields["text"],
			"Title":    hit.Fields["title"],
			"Company":  hit.Fields["company"],
			"Location": hit.Fields["location"],
		}
	}

	// deduplicate documents by hash
	deduplicated := make(map[string]pinecone.Document)
	for _, doc := range merged {
		hash, err := utils.SHA256Hash([]byte(doc["Text"].(string)))
		if err != nil {
			return nil, fmt.Errorf("failed to hash job detail: %w", err)
		}
		deduplicated[hash] = doc
	}

	documents := make([]pinecone.Document, 0, len(deduplicated))
	for _, doc := range deduplicated {
		documents = append(documents, doc)
	}

	params := map[string]any{
		"truncate": "END",
	}

	return client.Inference.Rerank(ctx, &pinecone.RerankRequest{
		Model:      "bge-reranker-v2-m3",
		Query:      searchQuery,
		Documents:  documents,
		RankFields: &[]string{"Text"},
		TopN:       &topK,
		Parameters: &params,
	})
}

func (app *application) semanticSearchJobsWithParams(ctx context.Context, searchQuery string, topK int, selectedCriteriaID string, params *internal.HybridSearchParams, enableReranking bool) ([]internal.ScoredJobDetail, error) {
	// TODO: Wire this to your new hybrid search implementation
	// For now, just call the old function

	// Create a steering query from the search string
	steeringQuery := &internal.SteeringQuery{
		Query:       searchQuery,
		RerankQuery: searchQuery, // Use the same query for reranking
	}

	// Call the improved semantic search function
	scoredJobs, err := internal.SemanticSearchJobs(ctx, steeringQuery, selectedCriteriaID, topK, params)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Apply reranking if enabled
	if enableReranking && len(scoredJobs) > 0 {
		// Need to convert flat list to pointers for reranking
		pointerJobs := make([]*internal.ScoredJobDetail, len(scoredJobs))
		for i := range scoredJobs {
			pointerJobs[i] = &scoredJobs[i]
		}

		client, err := internal.NewPineconeClient()
		if err != nil {
			return scoredJobs, fmt.Errorf("failed to create pinecone client for reranking: %w", err)
		}

		// Attempt reranking, but fall back to original results if it fails
		rerankedJobs, err := internal.RerankResults(ctx, client, pointerJobs, searchQuery, topK, internal.PineconeRerankBGEv2M3)
		if err != nil {
			app.logger.Error("reranking failed, using original results", "error", err)
		} else {
			return rerankedJobs, nil
		}
	}

	return scoredJobs, nil
}

func (app *application) adminFeatureFlags(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)

	// Initialize Cloudflare KV client
	kv, err := cloudflare.NewKV()
	if err != nil {
		app.logger.Error("failed to create Cloudflare KV client", "error", err)
		data["Error"] = "Failed to initialize Cloudflare KV: " + err.Error()
		if err := response.Page(w, http.StatusOK, data, "pages/admin/feature-flags.html"); err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	// Get current flag states
	searchSchedulerEnabled := true       // Default to enabled
	subscriptionProcessorEnabled := true // Default to enabled

	// Read search-scheduler flag
	var searchFlag bool
	if err := kv.Get("search-scheduler", &searchFlag); err != nil {
		if err != cloudflare.ErrNotFound {
			app.logger.Error("failed to get search-scheduler flag", "error", err)
		}
		// Keep default value if error or not found
	} else {
		searchSchedulerEnabled = searchFlag
	}

	// Read subscription-processor flag
	var subscriptionFlag bool
	if err := kv.Get("subscription-processor", &subscriptionFlag); err != nil {
		if err != cloudflare.ErrNotFound {
			app.logger.Error("failed to get subscription-processor flag", "error", err)
		}
		// Keep default value if error or not found
	} else {
		subscriptionProcessorEnabled = subscriptionFlag
	}

	data["SearchSchedulerEnabled"] = searchSchedulerEnabled
	data["SubscriptionProcessorEnabled"] = subscriptionProcessorEnabled

	// Handle POST request for updating flags
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			app.logger.Error("failed to parse form", "error", err)
			data["Error"] = "Failed to parse form: " + err.Error()
		} else {
			// Get checkbox values (unchecked = not present in form)
			newSearchEnabled := r.PostForm.Get("search-scheduler") == "true"
			newSubscriptionEnabled := r.PostForm.Get("subscription-processor") == "true"

			var errors []string

			// Update search-scheduler flag
			if err := kv.Set("search-scheduler", newSearchEnabled, 0); err != nil {
				app.logger.Error("failed to set search-scheduler flag", "error", err)
				errors = append(errors, "Failed to update search-scheduler: "+err.Error())
			} else {
				app.logger.Info("updated search-scheduler flag", "enabled", newSearchEnabled)
			}

			// Update subscription-processor flag
			if err := kv.Set("subscription-processor", newSubscriptionEnabled, 0); err != nil {
				app.logger.Error("failed to set subscription-processor flag", "error", err)
				errors = append(errors, "Failed to update subscription-processor: "+err.Error())
			} else {
				app.logger.Info("updated subscription-processor flag", "enabled", newSubscriptionEnabled)
			}

			if len(errors) > 0 {
				data["Error"] = strings.Join(errors, "; ")
			} else {
				data["Success"] = "Feature flags updated successfully"
				// Update the displayed values
				data["SearchSchedulerEnabled"] = newSearchEnabled
				data["SubscriptionProcessorEnabled"] = newSubscriptionEnabled
			}
		}
	}

	if err := response.Page(w, http.StatusOK, data, "pages/admin/feature-flags.html"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) adminUserDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := app.newTemplateData(r)

	// Fetch all users for the dropdown
	users, err := internal.ListUsers(ctx, app.fs, 100, 0) // Limiting to 100 users
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data["Users"] = users

	// Handle POST request for user selection
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			app.serverError(w, r, fmt.Errorf("failed to parse form: %w", err))
			return
		}

		selectedUserID := r.PostForm.Get("user_id")
		if selectedUserID == "" {
			data["Error"] = "No user selected"
		} else {
			// Fetch the selected user's details
			selectedUser, found, err := internal.GetUser(ctx, app.fs, selectedUserID)
			if err != nil {
				app.serverError(w, r, fmt.Errorf("failed to get user: %w", err))
				return
			}
			if !found {
				data["Error"] = "User not found"
			} else {
				data["SelectedUser"] = selectedUser
				data["SelectedUserID"] = selectedUserID

				// Get user's dashboard data similar to showDashboard
				status := internal.JobStatusNew
				if statusParam := r.PostForm.Get("status"); statusParam != "" {
					if parsedStatus, isValid := internal.ParseJobStatus(statusParam); isValid {
						status = parsedStatus
					}
				}

				showAllRatings := r.PostForm.Get("show_all") == "true"
				pageSize := 10 // Show more jobs in admin view

				// Get the job count for the selected status
				statusJobCount, err := internal.GetJobsCount(ctx, app.fs, selectedUser.ID, status, showAllRatings)
				if err != nil {
					app.logger.Warn("adminUserDashboard: failed to get job count", "error", err, "user_id", selectedUserID)
					statusJobCount = 0
				}

				// Hybrid Approach: Use semantic search for "New" jobs, existing queries for other statuses
				jobs := map[string]JobWithDetails{}
				var semanticJobsSlice []JobWithDetails // Ordered slice for semantic search results
				var nextPageToken string

				if status == internal.JobStatusNew {
					// For "New" status, use semantic search with jobs that have no UserJob record

					// Get user's subscriptions to find criteriaID for semantic search
					subscriptions, err := internal.GetSubscriptionsForUser(ctx, app.fs, selectedUser.ID)
					if err != nil {
						app.logger.Warn("adminUserDashboard: failed to get subscriptions", "error", err, "user_id", selectedUserID)
					} else if len(subscriptions) > 0 {
						// Use the first subscription's criteria for semantic search
						criteriaID := subscriptions[0].CriteriaID

						// Get semantic search results for the user
						semanticJobs, err := internal.GetSemanticJobsForUser(ctx, app.fs, selectedUser, criteriaID, pageSize)
						if err != nil {
							app.logger.Warn("adminUserDashboard: failed to get semantic jobs", "error", err, "user_id", selectedUserID)
						} else {
							// Filter semantic jobs to only include those without existing UserJob records
							existingUserJobs, err := internal.ListUserJobs(ctx, app.fs, selectedUser.ID)
							if err != nil {
								app.logger.Warn("adminUserDashboard: failed to get existing user jobs for filtering", "error", err, "user_id", selectedUserID)
								existingUserJobs = []internal.UserJob{} // Continue with empty list
							}

							// Create a set of existing job IDs for fast lookup
							existingJobIDs := make(map[string]bool)
							for _, userJob := range existingUserJobs {
								existingJobIDs[userJob.JobRef.ID] = true
							}

							// Add semantic jobs that don't have UserJob records, preserving order
							for _, semanticJob := range semanticJobs {
								if !existingJobIDs[semanticJob.JobDetail.ID] {
									jobWithDetails := JobWithDetails{
										ScoredJobDetail: semanticJob,
										IsSemanticOnly:  true,
									}
									jobs[semanticJob.JobDetail.ID] = jobWithDetails
									semanticJobsSlice = append(semanticJobsSlice, jobWithDetails)
								}
							}
						}
					}
				} else {
					// For other statuses, use existing UserJob queries

					// Get paginated jobs by status
					userJobs, pageToken, err := internal.GetUserJobsSorted(ctx, app.fs, selectedUser.ID, status, pageSize, "", showAllRatings)
					if err != nil {
						app.logger.Warn("adminUserDashboard: failed to get user jobs", "error", err, "user_id", selectedUserID)
						userJobs = []*internal.UserJob{}
					}
					nextPageToken = pageToken

					// Get job details for UserJob records
					jobIDs := make([]string, 0, len(userJobs))
					for _, job := range userJobs {
						jobIDs = append(jobIDs, job.JobRef.ID)
						jobs[job.JobRef.ID] = JobWithDetails{
							UserJob: job,
						}
					}

					if len(jobIDs) > 0 {
						jobDetails, err := internal.GetJobDetailsByIDs(ctx, app.fs, jobIDs)
						if err != nil {
							app.logger.Warn("adminUserDashboard: failed to get job details", "error", err, "user_id", selectedUserID)
						} else {
							// Merge job details into the jobs map (matching main dashboard approach)
							for _, detail := range jobDetails {
								if job, exists := jobs[detail.ID]; exists {
									job.JobDetail = *detail
									jobs[detail.ID] = job
								}
							}
						}
					}
				}

				// Add dashboard data to template
				data["HasJobs"] = len(jobs) > 0 || statusJobCount > 0
				data["ActiveStatus"] = string(status)
				data["NextPageToken"] = nextPageToken
				data["HasNextPage"] = nextPageToken != ""
				data["TotalJobsForStatus"] = statusJobCount
				data["JobStatuses"] = internal.GetJobStatuses()
				data["StatusDisplayNames"] = map[string]string{
					"new":      "New Jobs",
					"saved":    "Saved Jobs",
					"applied":  "Applied Jobs",
					"rejected": "Rejected Jobs",
				}
				data["ShowAllRatings"] = showAllRatings
				data["TestMode"] = true // Enable test mode features in dashboard view

				// Set jobs data based on status type (matching main dashboard approach)
				if status == internal.JobStatusNew {
					data["HasJobs"] = len(semanticJobsSlice) > 0 || statusJobCount > 0
					data["Jobs"] = semanticJobsSlice
				} else {
					data["HasJobs"] = len(jobs) > 0 || statusJobCount > 0
					data["Jobs"] = jobs
				}
			}
		}
	}

	if err := response.Page(w, http.StatusOK, data, "pages/admin/user-dashboard.html"); err != nil {
		app.serverError(w, r, err)
	}
}
