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
		name        string
		query       string
		moveToward  string
		moveAway    string
		forceToward float64
		forceAway   float64
		model       PineconeEmbedModel
		wantErr     bool
	}{
		{
			name:        "Basic steering",
			query:       "software engineer",
			moveToward:  "experienced",
			moveAway:    "entry level",
			forceToward: 0.3,
			forceAway:   0.2,
			model:       PineconeEmbedDense,
			wantErr:     false,
		},
		{
			name:        "Zero forces",
			query:       "software engineer",
			moveToward:  "experienced",
			moveAway:    "entry level",
			forceToward: 0.0,
			forceAway:   0.0,
			model:       PineconeEmbedDense,
			wantErr:     false,
		},
	}

	p, err := NewPineconeClient()
	if err != nil {
		t.Fatalf("Failed to create Pinecone client: %v", err)
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			got, err := NewSteeringQuery(tt.query, tt.model,
				WithMoveToward(tt.moveToward, tt.forceToward),
				WithMoveAway(tt.moveAway, tt.forceAway))

			if (err != nil) != tt.wantErr {
				t.Errorf("NewSteeringQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify the vector has values and is properly normalized
				if got == nil {
					t.Errorf("GetSteeringVector() returned nil vector")
					return
				}

				// Check vector length (should be non-zero)
				vector, err := got.GetDenseVector(ctx, p, tt.model)
				if err != nil {
					t.Errorf("GetSteeringVector() returned error: %v", err)
					return
				}
				if len(vector) == 0 {
					t.Errorf("GetSteeringVector() returned empty vector")
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
