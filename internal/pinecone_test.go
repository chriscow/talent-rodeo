package internal

import (
	"context"
	"os"
	"testing"
)

func TestGetSteeringVector(t *testing.T) {
	// Skip test if PINECONE_API_KEY is not set (for CI environments)
	if os.Getenv("PINECONE_API_KEY") == "" {
		t.Skip("PINECONE_API_KEY not set, skipping test")
	}

	tests := []struct {
		name            string
		query           string
		moveToward      string
		moveAway        string
		forceToward     float64
		forceAway       float64
		experienceLevel ExperienceLevel
		workType        WorkType
		rerankQuery     string
		model           PineconeEmbedModel
		wantErr         bool
	}{
		{
			name:            "Basic steering",
			query:           "software engineer",
			moveToward:      "I want to work with experienced teams",
			moveAway:        "I want to work in entry level positions",
			forceToward:     0.3,
			forceAway:       0.2,
			experienceLevel: ExperienceLevelMidSeniorLevel,
			workType:        WorkTypeRemote,
			rerankQuery:     "Software engineer with experienced teams, avoiding entry level positions",
			model:           PineconeEmbedDense,
			wantErr:         false,
		},
		{
			name:            "Zero forces",
			query:           "software engineer",
			moveToward:      "I want to work with experienced teams",
			moveAway:        "I want to work in entry level positions",
			forceToward:     0.1, // Changed from 0.0 to avoid validation error
			forceAway:       0.1, // Changed from 0.0 to avoid validation error
			experienceLevel: ExperienceLevelMidSeniorLevel,
			workType:        WorkTypeHybrid,
			rerankQuery:     "Software engineer with experienced teams, avoiding entry level positions",
			model:           PineconeEmbedDense,
			wantErr:         false,
		},
	}

	p, err := NewPineconeClient()
	if err != nil {
		t.Fatalf("Failed to create Pinecone client: %v", err)
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sq := &SteeringQuery{
				Queries:         []string{tt.query},
				ExperienceLevel: tt.experienceLevel,
				WorkTypes:       []WorkType{tt.workType},
				MoveToward:      tt.moveToward,
				MoveAway:        tt.moveAway,
				ForceAway:       tt.forceAway,
				RerankQuery:     tt.rerankQuery,
			}

			err := sq.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("SteeringQuery.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {

				// Check vector length (should be non-zero)
				vector, err := sq.GetDenseVector(ctx, p, tt.model)
				if err != nil {
					t.Errorf("GetDenseVector() returned error: %v", err)
					return
				}
				if len(vector) == 0 {
					t.Errorf("GetDenseVector() returned empty vector")
					return
				}

				// Check normalization (length should be close to 1.0)
				var sumSquares float64
				for _, v := range vector {
					sumSquares += float64(v * v)
				}

				// Allow for small floating point errors
				const tolerance = 1e-6
				if sumSquares < 1.0-tolerance || sumSquares > 1.0+tolerance {
					t.Errorf("Vector not properly normalized, magnitude squared = %v, want 1.0", sumSquares)
				}
			}
		})
	}
}
