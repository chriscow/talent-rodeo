package search

import "fmt"

// PageRange represents the range of pages to search
type PageRange struct {
	StartPage        int  // The page to start searching from
	EndPage          int  // The last page to search
	ShouldResetState bool // Whether the search state should be reset
}

// PageRangeInput contains all the inputs needed to calculate a page range
type PageRangeInput struct {
	CurrentTier   string // The candidate's current tier (free, mid, top)
	LastPage      *int   // The last page that was searched, nil if no previous search
	ResetProgress bool   // Whether to force a progress reset (e.g., due to subscription expiry)
	PreviousTier  string // The candidate's previous tier, if this is a tier change
	IsTierChange  bool   // Whether this is a tier change operation
}

// TierLimits defines the maximum pages allowed for each tier
var TierLimits = map[string]int{
	"free": 3,
	"mid":  50,
	"top":  200,
}

// CalculatePageRange determines the appropriate page range for a search based on
// the candidate's tier, last page searched, and whether this is a tier change.
//
// The function follows these rules:
// 1. For new searches (LastPage is nil): Start from page 1
// 2. For tier upgrades: Continue from LastPage + 1
// 3. For tier downgrades:
//   - If LastPage is within new tier limit: Continue from LastPage + 1
//   - If LastPage exceeds new tier limit: Reset to page 1
//
// 4. For normal searches: Continue from LastPage + 1, wrapping to 1 if at limit
func CalculatePageRange(input PageRangeInput) (*PageRange, error) {
	// Validate tier
	maxPages, ok := TierLimits[input.CurrentTier]
	if !ok {
		return nil, fmt.Errorf("invalid tier: %s", input.CurrentTier)
	}

	// Start with default values
	result := &PageRange{
		StartPage:        1,
		EndPage:          maxPages,
		ShouldResetState: false,
	}

	// If we're forcing a reset, return the default range
	if input.ResetProgress {
		result.ShouldResetState = true
		return result, nil
	}

	// If no previous search, start from beginning
	if input.LastPage == nil {
		return result, nil
	}

	// Handle tier changes
	if input.IsTierChange {
		// For downgrades, check if last_page exceeds new tier limit
		prevMaxPages, ok := TierLimits[input.PreviousTier]
		if !ok {
			return nil, fmt.Errorf("invalid previous tier: %s", input.PreviousTier)
		}
		if prevMaxPages > maxPages && *input.LastPage > maxPages {
			// Downgrade exceeding new tier limit - reset to page 1
			result.ShouldResetState = true
			return result, nil
		}
	}

	// Continue from last page + 1
	result.StartPage = *input.LastPage + 1

	// If we've reached the end, start over from page 1
	if result.StartPage > maxPages {
		result.StartPage = 1
	}

	return result, nil
}
