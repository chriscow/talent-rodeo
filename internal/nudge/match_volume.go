package nudge

const (
	HighVolumeMatchThreshold    = 10 // 10+ quality matches = high volume
	TypicalVolumeMatchThreshold = 4  // 4-9 quality matches = typical volume
	// 3 or fewer quality matches = low volume
)

// DetermineMatchVolumeCategory categorizes the number of quality matches
// Based on TRIAL_EMAIL_STRATEGY.md specifications
func DetermineMatchVolumeCategory(qualityMatches int) string {
	if qualityMatches >= HighVolumeMatchThreshold {
		return "high"
	}
	if qualityMatches >= TypicalVolumeMatchThreshold {
		return "typical"
	}
	return "low"
}
