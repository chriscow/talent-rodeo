package main

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"talent-rodeo/internal"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/response"
	"time"
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
			jobDetails, err = internal.ListJobDetailsForSearch(ctx, app.fs, searchTaskID)
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
						indexHost := os.Getenv("PINECONE_INDEX_HOST")
						if indexHost == "" {
							app.logger.Error("PINECONE_INDEX_HOST environment variable not set")
							data["Error"] = "PINECONE_INDEX_HOST environment variable not set"
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

	type SearchResult struct {
		JobDetail *internal.JobDetail
		Score     float32
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			app.serverError(w, r, err)
			return
		}

		searchQuery := r.PostForm.Get("search_query")
		topKStr := r.PostForm.Get("top_k")

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

		data["SearchQuery"] = searchQuery
		data["TopK"] = topK

		if searchQuery != "" {
			// Get Pinecone client
			pineconeClient, err := internal.NewPineconeClient()
			if err != nil {
				app.logger.Error("failed to create Pinecone client", "error", err)
				data["Error"] = "Failed to initialize Pinecone: " + err.Error()
			} else {
				// Get index host from environment
				indexHost := os.Getenv("PINECONE_INDEX_HOST")
				if indexHost == "" {
					app.logger.Error("PINECONE_INDEX_HOST environment variable not set")
					data["Error"] = "PINECONE_INDEX_HOST environment variable not set"
				} else {
					app.logger.Info("searching records in Pinecone",
						"index_host", indexHost,
						"namespace", internal.JobDetailNamespace,
						"query", searchQuery)

					// Call SearchRecords with the text query
					searchResp, err := pineconeClient.SearchRecords(ctx, indexHost, internal.JobDetailNamespace, searchQuery,
						internal.WithTopK(topK))

					if err != nil {
						app.logger.Error("failed to search records", "error", err)
						data["Error"] = "Failed to search job records: " + err.Error()
					} else {
						// Get hit IDs and scores
						var jobIDs []string
						scoreMap := make(map[string]float32)

						for _, hit := range searchResp.Result.Hits {
							// Extract the job ID from hit fields
							if id, ok := hit.Fields["ID"].(string); ok && id != "" {
								jobIDs = append(jobIDs, id)
								scoreMap[id] = hit.Score
							} else if id, ok := hit.Id, true; ok && id != "" {
								// Use the hit ID directly if Fields["ID"] is not available
								jobIDs = append(jobIDs, id)
								scoreMap[id] = hit.Score
							}
						}

						app.logger.Info("found matches in Pinecone",
							"hits_count", len(searchResp.Result.Hits),
							"job_ids", jobIDs)

						if len(jobIDs) > 0 {
							// Fetch the job details for the matching IDs
							jobDetails, err := internal.GetJobDetailsByIDs(ctx, app.fs, jobIDs)
							if err != nil {
								app.logger.Error("failed to get job details", "error", err)
								data["Error"] = "Failed to get job details: " + err.Error()
							} else {
								// Build search results with scores
								var searchResults []SearchResult
								for _, job := range jobDetails {
									score, ok := scoreMap[job.ID]
									if !ok {
										score = 0
									}

									searchResults = append(searchResults, SearchResult{
										JobDetail: job,
										Score:     score,
									})
								}

								// Sort results by score in descending order
								sort.Slice(searchResults, func(i, j int) bool {
									return searchResults[i].Score > searchResults[j].Score
								})

								data["SearchResults"] = searchResults
							}
						}
					}
				}
			}
		}
	}

	if err := response.Page(w, http.StatusOK, data, "pages/admin/search-jobs.html"); err != nil {
		app.serverError(w, r, err)
	}
}
