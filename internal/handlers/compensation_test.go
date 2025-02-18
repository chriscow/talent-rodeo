package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestExtractCompensation(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Test job description
	jobDesc := `We offer a competitive compensation package including:
    - Base salary range: $120,000 - $180,000 USD per year
    - Equity: 0.1% - 0.5% in stock options with 4-year vesting
    - Benefits:
        * Health, dental, and vision insurance
        * 401(k) with 4% match
        * Unlimited PTO
        * Annual learning stipend
    - Additional compensation:
        * Annual performance bonus
        * Home office setup allowance`

	want := Compensation{
		SalaryRange: &SalaryRange{
			Min:      120000,
			Max:      180000,
			Currency: "USD",
			Period:   "yearly",
		},
		Benefits: []string{
			"Health, dental, and vision insurance",
			"401(k) with 4% match",
			"Unlimited PTO",
			"Annual learning stipend",
		},
		AdditionalCompensation: []string{
			"Annual performance bonus",
			"Home office setup allowance",
		},
		Equity: &EquityRange{
			Min:           0.1,
			Max:           0.5,
			Type:          "Options",
			VestingPeriod: "4 years",
		},
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("TestExtractCompensation: Failed to marshal result: %v", err)
	}

	env.llm.responses = map[string]string{
		compensationPrompt: string(wantJSON),
	}

	// Create handler
	handler, err := NewCompensationHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Test ExtractCompensation
	got, err := handler.extract(context.Background(), jobDesc)
	if err != nil {
		t.Fatalf("ExtractCompensation failed: %v", err)
	}

	// Verify salary range
	if got.SalaryRange == nil {
		t.Error("Expected salary range, got nil")
	} else {
		if got.SalaryRange.Min != want.SalaryRange.Min {
			t.Errorf("got min salary %f, want %f", got.SalaryRange.Min, want.SalaryRange.Min)
		}
		if got.SalaryRange.Max != want.SalaryRange.Max {
			t.Errorf("got max salary %f, want %f", got.SalaryRange.Max, want.SalaryRange.Max)
		}
		if got.SalaryRange.Currency != want.SalaryRange.Currency {
			t.Errorf("got currency %s, want %s", got.SalaryRange.Currency, want.SalaryRange.Currency)
		}
		if got.SalaryRange.Period != want.SalaryRange.Period {
			t.Errorf("got period %s, want %s", got.SalaryRange.Period, want.SalaryRange.Period)
		}
	}

	// Verify equity
	if got.Equity == nil {
		t.Error("Expected equity, got nil")
	} else {
		if got.Equity.Min != want.Equity.Min {
			t.Errorf("got min equity %f, want %f", got.Equity.Min, want.Equity.Min)
		}
		if got.Equity.Max != want.Equity.Max {
			t.Errorf("got max equity %f, want %f", got.Equity.Max, want.Equity.Max)
		}
		if got.Equity.Type != want.Equity.Type {
			t.Errorf("got equity type %s, want %s", got.Equity.Type, want.Equity.Type)
		}
		if got.Equity.VestingPeriod != want.Equity.VestingPeriod {
			t.Errorf("got vesting period %s, want %s", got.Equity.VestingPeriod, want.Equity.VestingPeriod)
		}
	}

	// Verify benefits
	expectedBenefits := []string{
		"Health, dental, and vision insurance",
		"401(k) with 4% match",
		"Unlimited PTO",
		"Annual learning stipend",
	}
	if len(got.Benefits) != len(expectedBenefits) {
		t.Errorf("got %d benefits, want %d", len(got.Benefits), len(expectedBenefits))
	}

	// Verify additional compensation
	expectedAdditional := []string{
		"Annual performance bonus",
		"Home office setup allowance",
	}
	if len(got.AdditionalCompensation) != len(expectedAdditional) {
		t.Errorf("got %d additional compensation items, want %d",
			len(got.AdditionalCompensation), len(expectedAdditional))
	}
}

func TestCompensationHandleMessage(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Create test input with a specific job description
	input := JobDescriptionInput{
		Description: "Competitive salary range $100k-$150k with benefits",
	}

	// Create expected response
	want := &Compensation{
		SalaryRange: &SalaryRange{
			Min:      100000,
			Max:      150000,
			Currency: "USD",
			Period:   "yearly",
		},
		Benefits: []string{"benefits"},
	}

	// Set up mock LLM response
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("TestCompensationHandleMessage: Failed to marshal result: %v", err)
	}
	env.llm.responses[compensationPrompt] = string(wantJSON)

	// Create handler with a background context since we don't need to control its lifecycle.
	// The handler is required even though we don't explicitly use it because it registers
	// the message handler with the GenericHandler during creation.
	_, err = NewCompensationHandler(context.Background(), env.js, env.llm, env.logger)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Send test message and verify response
	result, err := SendAndReceiveMessage[JobDescriptionInput, Compensation](
		t,
		context.Background(),
		env.js,
		rodeoStream,
		TopicJobDescriptionInput,
		TopicCompensationExtracted,
		input,
		1*time.Second,
	)
	if err != nil {
		t.Fatalf("Failed to send and receive message: %v", err)
	}

	// Verify basic response structure
	if result.SalaryRange == nil {
		t.Error("Expected salary range in response, got nil")
	} else {
		if result.SalaryRange.Min != want.SalaryRange.Min {
			t.Errorf("got min salary %f, want %f", result.SalaryRange.Min, want.SalaryRange.Min)
		}
		if result.SalaryRange.Max != want.SalaryRange.Max {
			t.Errorf("got max salary %f, want %f", result.SalaryRange.Max, want.SalaryRange.Max)
		}
	}
}
