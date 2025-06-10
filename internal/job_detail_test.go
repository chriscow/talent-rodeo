package internal

import (
	"encoding/json"
	"talent-rodeo/internal/utils"
	"testing"
)

type testHybridSearchParams struct {
	DenseWeight     float64
	SparseWeight    float64
	RecencyWeight   float64
	RecencyHalfLife float64
	ScoreThreshold  float64
	ScoreFusion     string
}

type testSteeringQuery struct {
	Queries         string
	RerankQuery     string
	ForceAway       float64
	MoveToward      string
	MoveAway        string
	WorkType        string
	ExperienceLevel string
}

func TestCacheKeyHashStability(t *testing.T) {
	params1 := testHybridSearchParams{
		DenseWeight:     1.0,
		SparseWeight:    2.0,
		RecencyWeight:   3.0,
		RecencyHalfLife: 4.0,
		ScoreThreshold:  5.0,
		ScoreFusion:     "sum",
	}
	params2 := testHybridSearchParams{
		DenseWeight:     1.0,
		SparseWeight:    2.0,
		RecencyWeight:   3.0,
		RecencyHalfLife: 4.0,
		ScoreThreshold:  5.0,
		ScoreFusion:     "sum",
	}

	b1, err := json.Marshal(params1)
	if err != nil {
		t.Fatalf("failed to marshal params1: %v", err)
	}
	b2, err := json.Marshal(params2)
	if err != nil {
		t.Fatalf("failed to marshal params2: %v", err)
	}
	hash1, err := utils.SHA256Hash(b1)
	if err != nil {
		t.Fatalf("failed to hash params1: %v", err)
	}
	hash2, err := utils.SHA256Hash(b2)
	if err != nil {
		t.Fatalf("failed to hash params2: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("expected identical params to produce same hash, got %s and %s", hash1, hash2)
	}

	steering1 := testSteeringQuery{
		Queries:         "foo",
		RerankQuery:     "bar",
		ForceAway:       1.23,
		MoveToward:      "baz",
		MoveAway:        "qux",
		WorkType:        "remote",
		ExperienceLevel: "senior",
	}
	steering2 := testSteeringQuery{
		Queries:         "foo",
		RerankQuery:     "bar",
		ForceAway:       1.23,
		MoveToward:      "baz",
		MoveAway:        "qux",
		WorkType:        "remote",
		ExperienceLevel: "senior",
	}

	b3, err := json.Marshal(steering1)
	if err != nil {
		t.Fatalf("failed to marshal steering1: %v", err)
	}
	b4, err := json.Marshal(steering2)
	if err != nil {
		t.Fatalf("failed to marshal steering2: %v", err)
	}
	hash3, err := utils.SHA256Hash(b3)
	if err != nil {
		t.Fatalf("failed to hash steering1: %v", err)
	}
	hash4, err := utils.SHA256Hash(b4)
	if err != nil {
		t.Fatalf("failed to hash steering2: %v", err)
	}

	if hash3 != hash4 {
		t.Errorf("expected identical steering queries to produce same hash, got %s and %s", hash3, hash4)
	}
}
