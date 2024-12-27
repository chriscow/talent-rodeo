// Package linkedin implements the OAuth2 protocol for authenticating users through Linkedin.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/markbates/goth"
	"golang.org/x/oauth2"
)

// more details about linkedin fields:
// User Profile and Email Address - https://docs.microsoft.com/en-gb/linkedin/consumer/integrations/self-serve/sign-in-with-linkedin
// User Avatar - https://docs.microsoft.com/en-gb/linkedin/shared/references/v2/digital-media-asset

const (
	authURL  string = "https://www.linkedin.com/oauth/v2/authorization"
	tokenURL string = "https://www.linkedin.com/oauth/v2/accessToken"

	// userEndpoint requires scope "r_liteprofile"
	userEndpoint string = "//api.linkedin.com/v2/userinfo"
)

// New creates a new linkedin provider, and sets up important connection details.
// You should always call `linkedin.New` to get a new Provider. Never try to create
// one manually.
func New(clientKey, secret, callbackURL string, scopes ...string) *Provider {
	p := &Provider{
		ClientKey:    clientKey,
		Secret:       secret,
		CallbackURL:  callbackURL,
		providerName: "linkedin",
	}
	p.config = newConfig(p, scopes)
	return p
}

// Provider is the implementation of `goth.Provider` for accessing Linkedin.
type Provider struct {
	ClientKey    string
	Secret       string
	CallbackURL  string
	HTTPClient   *http.Client
	config       *oauth2.Config
	providerName string
}

// Name is the name used to retrieve this provider later.
func (p *Provider) Name() string {
	return p.providerName
}

// SetName is to update the name of the provider (needed in case of multiple providers of 1 type)
func (p *Provider) SetName(name string) {
	p.providerName = name
}

// Client returns an HTTPClientWithFallback
func (p *Provider) Client() *http.Client {
	return goth.HTTPClientWithFallBack(p.HTTPClient)
}

// Debug is a no-op for the linkedin package.
func (p *Provider) Debug(debug bool) {}

// BeginAuth asks Linkedin for an authentication end-point.
func (p *Provider) BeginAuth(state string) (goth.Session, error) {
	url := p.config.AuthCodeURL(state)
	session := &Session{
		AuthURL: url,
	}
	return session, nil
}

// FetchUser will go to Linkedin and access basic information about the user.
func (p *Provider) FetchUser(session goth.Session) (goth.User, error) {
	s := session.(*Session)
	user := goth.User{
		AccessToken: s.AccessToken,
		Provider:    p.Name(),
		ExpiresAt:   s.ExpiresAt,
	}

	if user.AccessToken == "" {
		// data is not yet retrieved since accessToken is still empty
		return user, fmt.Errorf("%s cannot get user information without accessToken", p.providerName)
	}

	// create request for user r_liteprofile
	req, err := http.NewRequest("GET", "", nil)
	if err != nil {
		return user, err
	}

	// add url as opaque to avoid escaping of "("
	req.URL = &url.URL{
		Scheme: "https",
		Host:   "api.linkedin.com",
		Opaque: userEndpoint,
	}

	req.Header.Set("Authorization", "Bearer "+s.AccessToken)
	resp, err := p.Client().Do(req)
	if err != nil {
		return user, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return user, fmt.Errorf("%s responded with a %d trying to fetch user profile", p.providerName, resp.StatusCode)
	}

	// read r_liteprofile information
	err = userFromReader(resp.Body, &user)
	if err != nil {
		return user, err
	}

	return user, err
}

func userFromReader(reader io.Reader, user *goth.User) error {
	u := struct {
		Sub        string `json:"sub"`
		Name       string `json:"name"`
		GivenName  string `json:"given_name"`
		FamilyName string `json:"family_name"`
		Picture    string `json:"picture"`
		Locale     struct {
			Language string `json:"language"`
			Country  string `json:"country"`
		} `json:"locale"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}{}

	err := json.NewDecoder(reader).Decode(&u)
	if err != nil {
		return err
	}

	// Populate the goth.User fields
	user.UserID = u.Sub
	user.FirstName = u.GivenName
	user.LastName = u.FamilyName
	user.Name = u.Name
	user.NickName = u.GivenName
	user.AvatarURL = u.Picture
	user.Email = u.Email

	// Optionally, use locale data if needed
	// user.Locale = fmt.Sprintf("%s-%s", u.Locale.Language, u.Locale.Country)

	return nil
}

func newConfig(provider *Provider, scopes []string) *oauth2.Config {
	c := &oauth2.Config{
		ClientID:     provider.ClientKey,
		ClientSecret: provider.Secret,
		RedirectURL:  provider.CallbackURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:  authURL,
			TokenURL: tokenURL,
		},
		Scopes: []string{},
	}

	if len(scopes) == 0 {
		// add helper as new API requires the scope to be specified and these are the minimum to retrieve profile information and user's email address
		scopes = append(scopes, "openid", "profile", "email")
	}

	for _, scope := range scopes {
		c.Scopes = append(c.Scopes, scope)
	}

	return c
}

// RefreshToken refresh token is not provided by linkedin
func (p *Provider) RefreshToken(refreshToken string) (*oauth2.Token, error) {
	return nil, errors.New("Refresh token is not provided by linkedin")
}

// RefreshTokenAvailable refresh token is not provided by linkedin
func (p *Provider) RefreshTokenAvailable() bool {
	return false
}
