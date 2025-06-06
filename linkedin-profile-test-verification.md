# LinkedIn Profile URL Feature - Test Verification

## Manual Testing Checklist

### Prerequisites
- [x] Code compiles successfully (`go build ./cmd/web`)
- [x] Unit tests pass (`go test -v ./internal/auth`)
- [x] LinkedIn OAuth is configured with valid credentials

### Feature Testing Steps

#### 1. New User Registration
- [ ] Log out of the application (if logged in)
- [ ] Click "Login with LinkedIn"
- [ ] Complete LinkedIn OAuth flow
- [ ] Verify user is created successfully
- [ ] Click on avatar in navigation bar
- [ ] Verify dropdown menu appears
- [ ] Verify "View LinkedIn Profile" link is present (if LinkedIn provides URL)
- [ ] Click the link and verify it opens LinkedIn profile in new tab

#### 2. Existing User Login
- [ ] Log out of the application
- [ ] Log in again with the same LinkedIn account
- [ ] Click on avatar in navigation bar
- [ ] Verify "View LinkedIn Profile" link is still present
- [ ] Verify profile URL was not lost during re-login

#### 3. UI/UX Verification
- [ ] Dropdown menu styling is consistent with app theme
- [ ] Avatar image displays correctly in dropdown trigger
- [ ] Dropdown menu includes:
  - [ ] Profile link
  - [ ] LinkedIn Profile link (conditional)
  - [ ] Logout button
- [ ] All links and buttons function correctly

#### 4. Edge Cases
- [ ] User without LinkedIn profile URL:
  - [ ] Dropdown should not show "View LinkedIn Profile" link
  - [ ] No errors should occur
- [ ] Profile URL opens in new tab with `target="_blank"`
- [ ] Profile URL has `rel="noopener noreferrer"` for security

### Database Verification
- [ ] Check Firestore to confirm LinkedInProfileURL field is populated
- [ ] Verify field persists across sessions

## Test Results

### Unit Tests
✅ `TestUserFromReader` - All test cases pass:
- Response with profile URL in profile field
- Response with profile URL in link field  
- Response without profile URL

### Integration Points
- [x] User struct has LinkedInProfileURL field
- [x] LinkedIn OAuth handler extracts profile URL
- [x] Authentication handler saves URL for new users
- [x] Authentication handler updates URL for existing users
- [x] Navigation template conditionally displays LinkedIn profile link

## Notes

- LinkedIn's userinfo endpoint may not always provide the profile URL
- The implementation checks both `profile` and `link` fields in the response
- Profile URLs are stored in Firestore and persist across sessions