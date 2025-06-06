package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const linkedinProfilesCollection = "linkedin_profiles"

// LinkedInProfile defines the structure for storing a user's LinkedIn profile data.
type LinkedInProfile struct {
	UserID    string
	Profile   any
	FetchedAt time.Time
	Available bool
	// ProfileID is the LinkedIn profile ID (e.g., "williamhgates")
	ProfileID string
}

// SetLinkedInProfile creates or updates a LinkedIn profile document in Firestore.
// It uses the user's ID as the document ID.
func SetLinkedInProfile(ctx context.Context, fs *FirestoreClient, userID string, profile *LinkedInProfile) error {
	_, err := fs.Collection(linkedinProfilesCollection).Doc(userID).Set(ctx, profile)
	return err
}

// GetLinkedInProfile retrieves a LinkedIn profile by user ID.
func GetLinkedInProfile(ctx context.Context, fs *FirestoreClient, userID string) (*LinkedInProfile, error) {
	doc, err := fs.Collection(linkedinProfilesCollection).Doc(userID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil // Return nil if profile doesn't exist
		}
		return nil, err
	}

	var profile LinkedInProfile
	if err := doc.DataTo(&profile); err != nil {
		return nil, err
	}
	return &profile, nil
}

// fetchLinkedInProfile fetches a LinkedIn profile using the Proxycurl API given a profile URL and API key.
// Note: Most parameters with '=include' incur extra cost per the Proxycurl pricing table.
func fetchLinkedInProfile(ctx context.Context, linkedinProfileURL, apiKey string) (map[string]any, error) {
	endpoint := "https://nubela.co/proxycurl/api/v2/linkedin"

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to parse endpoint: %w", err)
	}

	q := u.Query()
	q.Set("url", linkedinProfileURL) // required

	// The following parameters incur extra cost:
	// q.Set("extra", "include") // extra cost
	// q.Set("github_profile_id", "include")       // extra cost
	// q.Set("facebook_profile_id", "include")     // extra cost
	// q.Set("twitter_profile_id", "include")      // extra cost
	// q.Set("personal_contact_number", "include") // extra cost
	// q.Set("personal_email", "include")          // extra cost
	q.Set("inferred_salary", "include") // extra cost
	q.Set("skills", "include")          // extra cost

	// The following do NOT incur extra cost:
	q.Set("use_cache", "if-present")
	q.Set("fallback_to_cache", "on-error")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 10
	standardClient := retryClient.StandardClient() // *http.Client

	resp, err := standardClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("proxycurl API error: status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}
