package internal

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const personalSearchClaimsCollection = "personal_search_claims"

type personalSearchClaim struct {
	OwnerID      string
	CriteriaID   string
	Activation   time.Time
	SearchTaskID string
}

// GetPersonalSearchOwner reads the persisted owner state without using the user cache.
func GetPersonalSearchOwner(ctx context.Context, fs *FirestoreClient, ownerID string) (*User, error) {
	if fs == nil {
		return nil, ErrFirestoreClientIsNil
	}
	if ownerID == "" {
		return nil, fmt.Errorf("personal search owner ID is required")
	}
	doc, err := fs.Collection(usersCollection).Doc(ownerID).Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("read personal search owner: %w", err)
	}
	var user User
	if err := doc.DataTo(&user); err != nil {
		return nil, fmt.Errorf("decode personal search owner: %w", err)
	}
	if user.ID != ownerID {
		return nil, fmt.Errorf("personal search owner ID mismatch")
	}
	return &user, nil
}

// PersonalSearchTaskEligible admits only tasks created during the owner's current activation.
func PersonalSearchTaskEligible(task *SearchTask, ownerID string, activation time.Time) bool {
	return task != nil && ownerID != "" && task.UserID != nil && *task.UserID == ownerID &&
		!activation.IsZero() && !task.CreatedAt.Before(activation)
}

// CreatePersonalSearchTask scopes active-task deduplication to one owner and activation.
// Existing shared or older tasks remain untouched.
func CreatePersonalSearchTask(ctx context.Context, fs *FirestoreClient, criteria SearchCriteria, provider, ownerID string, activation time.Time) (*SearchTask, error) {
	if fs == nil {
		return nil, ErrFirestoreClientIsNil
	}
	if ownerID == "" || activation.IsZero() {
		return nil, fmt.Errorf("personal search owner and activation are required")
	}
	iter := fs.Collection(SearchTaskCollection).
		Where("Criteria.ID", "==", criteria.ID).
		Where("Status", "in", []SearchTaskStatus{SearchTaskStatusQueued, SearchTaskStatusProcessing}).Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("check personal search tasks: %w", err)
		}
		var existing SearchTask
		if err := doc.DataTo(&existing); err != nil {
			return nil, fmt.Errorf("decode personal search task: %w", err)
		}
		if PersonalSearchTaskEligible(&existing, ownerID, activation) {
			return &existing, ErrSearchTaskExists
		}
	}

	// The claim serializes concurrent creators for this owner and criteria. It
	// references only personal work and never modifies a legacy task.
	claimID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(ownerID+"\x00"+criteria.ID)).String()
	claimRef := fs.Collection(personalSearchClaimsCollection).Doc(claimID)
	var task *SearchTask
	err := fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		claimDoc, err := tx.Get(claimRef)
		if err != nil && status.Code(err) != codes.NotFound {
			return fmt.Errorf("read personal search claim: %w", err)
		}
		if err == nil {
			var claim personalSearchClaim
			if err := claimDoc.DataTo(&claim); err != nil {
				return fmt.Errorf("decode personal search claim: %w", err)
			}
			if claim.OwnerID != ownerID || claim.CriteriaID != criteria.ID {
				return fmt.Errorf("personal search claim identity mismatch")
			}
			if claim.Activation.Equal(activation) && claim.SearchTaskID != "" {
				currentDoc, err := tx.Get(fs.Collection(SearchTaskCollection).Doc(claim.SearchTaskID))
				if err != nil && status.Code(err) != codes.NotFound {
					return fmt.Errorf("read claimed personal search task: %w", err)
				}
				if err == nil {
					var current SearchTask
					if err := currentDoc.DataTo(&current); err != nil {
						return fmt.Errorf("decode claimed personal search task: %w", err)
					}
					if PersonalSearchTaskEligible(&current, ownerID, activation) &&
						(current.Status == SearchTaskStatusQueued || current.Status == SearchTaskStatusProcessing ||
							(current.Status == SearchTaskStatusCompleted && !current.CreatedAt.Before(time.Now().Add(-24*time.Hour)))) {
						task = &current
						return ErrSearchTaskExists
					}
				}
			}
		}

		now := time.Now().UTC()
		if now.Before(activation) {
			return fmt.Errorf("personal search activation is in the future")
		}
		task = &SearchTask{
			ID:        uuid.New().String(),
			UserID:    &ownerID,
			Criteria:  criteria,
			Provider:  provider,
			Status:    SearchTaskStatusQueued,
			CreatedAt: now,
		}
		if err := tx.Create(fs.Collection(SearchTaskCollection).Doc(task.ID), task); err != nil {
			return fmt.Errorf("create personal search task: %w", err)
		}
		claim := personalSearchClaim{OwnerID: ownerID, CriteriaID: criteria.ID, Activation: activation, SearchTaskID: task.ID}
		if err := tx.Set(claimRef, claim); err != nil {
			return fmt.Errorf("write personal search claim: %w", err)
		}
		return nil
	})
	if err != nil {
		return task, err
	}
	return task, nil
}

// GetPersonalSearchJobIDs returns only jobs linked to the owner's completed tasks.
func GetPersonalSearchJobIDs(ctx context.Context, fs *FirestoreClient, criteriaID, ownerID string, activation time.Time) ([]string, error) {
	tasks, err := GetSearchTasksForCriteria(ctx, fs, criteriaID)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	var ids []string
	for i := range tasks {
		task := &tasks[i]
		if task.Status != SearchTaskStatusCompleted || !PersonalSearchTaskEligible(task, ownerID, activation) {
			continue
		}
		jobIDs, err := GetJobIDsForSearchTask(ctx, fs, task.ID)
		if err != nil {
			return nil, err
		}
		for _, id := range jobIDs {
			if _, found := seen[id]; !found {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
}

// PersonalSearchCompletedSince reports whether this activation has a completed search since cutoff.
func PersonalSearchCompletedSince(ctx context.Context, fs *FirestoreClient, criteriaID, ownerID string, activation, cutoff time.Time) (bool, error) {
	tasks, err := GetSearchTasksForCriteria(ctx, fs, criteriaID)
	if err != nil {
		return false, err
	}
	for i := range tasks {
		task := &tasks[i]
		if task.Status == SearchTaskStatusCompleted && PersonalSearchTaskEligible(task, ownerID, activation) && !task.CreatedAt.Before(cutoff) {
			return true, nil
		}
	}
	return false, nil
}
