package search

import (
	"testing"
)

func ptr(i int) *int {
	return &i
}

func TestCalculatePageRange(t *testing.T) {
	tests := []struct {
		name          string
		input         PageRangeInput
		wantRange     *PageRange
		wantErr       bool
		wantErrString string
	}{
		{
			name: "new search - free tier",
			input: PageRangeInput{
				CurrentTier: "free",
				LastPage:    nil,
			},
			wantRange: &PageRange{
				StartPage:        1,
				EndPage:          3,
				ShouldResetState: false,
			},
		},
		{
			name: "continue search - free tier",
			input: PageRangeInput{
				CurrentTier: "free",
				LastPage:    ptr(2),
			},
			wantRange: &PageRange{
				StartPage:        3,
				EndPage:          3,
				ShouldResetState: false,
			},
		},
		{
			name: "wrap around - free tier",
			input: PageRangeInput{
				CurrentTier: "free",
				LastPage:    ptr(3),
			},
			wantRange: &PageRange{
				StartPage:        1,
				EndPage:          3,
				ShouldResetState: false,
			},
		},
		{
			name: "upgrade from free to mid",
			input: PageRangeInput{
				CurrentTier:  "mid",
				PreviousTier: "free",
				LastPage:     ptr(3),
				IsTierChange: true,
			},
			wantRange: &PageRange{
				StartPage:        4,
				EndPage:          50,
				ShouldResetState: false,
			},
		},
		{
			name: "downgrade mid to free within limit",
			input: PageRangeInput{
				CurrentTier:  "free",
				PreviousTier: "mid",
				LastPage:     ptr(2),
				IsTierChange: true,
			},
			wantRange: &PageRange{
				StartPage:        3,
				EndPage:          3,
				ShouldResetState: false,
			},
		},
		{
			name: "downgrade mid to free exceeds limit",
			input: PageRangeInput{
				CurrentTier:  "free",
				PreviousTier: "mid",
				LastPage:     ptr(45),
				IsTierChange: true,
			},
			wantRange: &PageRange{
				StartPage:        1,
				EndPage:          3,
				ShouldResetState: true,
			},
		},
		{
			name: "downgrade top to mid within limit",
			input: PageRangeInput{
				CurrentTier:  "mid",
				PreviousTier: "top",
				LastPage:     ptr(30),
				IsTierChange: true,
			},
			wantRange: &PageRange{
				StartPage:        31,
				EndPage:          50,
				ShouldResetState: false,
			},
		},
		{
			name: "downgrade top to mid exceeds limit",
			input: PageRangeInput{
				CurrentTier:  "mid",
				PreviousTier: "top",
				LastPage:     ptr(150),
				IsTierChange: true,
			},
			wantRange: &PageRange{
				StartPage:        1,
				EndPage:          50,
				ShouldResetState: true,
			},
		},
		{
			name: "force reset progress",
			input: PageRangeInput{
				CurrentTier:   "mid",
				LastPage:      ptr(25),
				ResetProgress: true,
			},
			wantRange: &PageRange{
				StartPage:        1,
				EndPage:          50,
				ShouldResetState: true,
			},
		},
		{
			name: "invalid tier",
			input: PageRangeInput{
				CurrentTier: "invalid",
				LastPage:    ptr(1),
			},
			wantErr:       true,
			wantErrString: "invalid tier: invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculatePageRange(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if err.Error() != tt.wantErrString {
					t.Errorf("expected error %q, got %q", tt.wantErrString, err.Error())
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if got.StartPage != tt.wantRange.StartPage {
				t.Errorf("StartPage = %v, want %v", got.StartPage, tt.wantRange.StartPage)
			}
			if got.EndPage != tt.wantRange.EndPage {
				t.Errorf("EndPage = %v, want %v", got.EndPage, tt.wantRange.EndPage)
			}
			if got.ShouldResetState != tt.wantRange.ShouldResetState {
				t.Errorf("ShouldResetState = %v, want %v", got.ShouldResetState, tt.wantRange.ShouldResetState)
			}
		})
	}
}
