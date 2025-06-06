package auth

import (
	"strings"
	"testing"

	"github.com/markbates/goth"
)

func TestUserFromReader(t *testing.T) {
	tests := []struct {
		name               string
		jsonResponse       string
		expectedProfileURL string
		expectedEmail      string
		expectedName       string
	}{
		{
			name: "Response with profile URL in profile field",
			jsonResponse: `{
				"sub": "12345",
				"name": "John Doe",
				"given_name": "John",
				"family_name": "Doe",
				"email": "john@example.com",
				"email_verified": true,
				"picture": "https://example.com/picture.jpg",
				"profile": "https://www.linkedin.com/in/johndoe"
			}`,
			expectedProfileURL: "https://www.linkedin.com/in/johndoe",
			expectedEmail:      "john@example.com",
			expectedName:       "John Doe",
		},
		{
			name: "Response with profile URL in link field",
			jsonResponse: `{
				"sub": "12345",
				"name": "Jane Smith",
				"given_name": "Jane",
				"family_name": "Smith",
				"email": "jane@example.com",
				"email_verified": true,
				"picture": "https://example.com/picture.jpg",
				"link": "https://www.linkedin.com/in/janesmith"
			}`,
			expectedProfileURL: "https://www.linkedin.com/in/janesmith",
			expectedEmail:      "jane@example.com",
			expectedName:       "Jane Smith",
		},
		{
			name: "Response without profile URL",
			jsonResponse: `{
				"sub": "12345",
				"name": "Bob Johnson",
				"given_name": "Bob",
				"family_name": "Johnson",
				"email": "bob@example.com",
				"email_verified": true,
				"picture": "https://example.com/picture.jpg"
			}`,
			expectedProfileURL: "",
			expectedEmail:      "bob@example.com",
			expectedName:       "Bob Johnson",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.jsonResponse)
			user := &goth.User{}

			err := userFromReader(reader, user)
			if err != nil {
				t.Fatalf("userFromReader() error = %v", err)
			}

			// Check basic fields
			if user.Email != tt.expectedEmail {
				t.Errorf("Email = %v, want %v", user.Email, tt.expectedEmail)
			}
			if user.Name != tt.expectedName {
				t.Errorf("Name = %v, want %v", user.Name, tt.expectedName)
			}

			// Check profile URL in RawData
			profileURL := ""
			if user.RawData != nil {
				if url, ok := user.RawData["linkedin_profile_url"].(string); ok {
					profileURL = url
				}
			}
			if profileURL != tt.expectedProfileURL {
				t.Errorf("linkedin_profile_url = %v, want %v", profileURL, tt.expectedProfileURL)
			}
		})
	}
}
