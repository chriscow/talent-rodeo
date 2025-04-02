package internal

import (
	"context"
	"fmt"
	"talent-rodeo/internal/funcs"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UserTier string

const (
	UserTierUnknown    UserTier = ""
	UserTierFree       UserTier = "free"
	UserTierPro        UserTier = "pro"
	UserTierEnterprise UserTier = "enterprise"
)

type UserStatus int

const (
	UserStatusUnknown UserStatus = iota
	UserStatusError
	UserStatusPendingResumeUpload
	UserStatusProcessingResume
	UserStatusResumeUploaded
	UserStatusIntroCallScheduled
	UserStatusIntroCallStarted
	UserStatusIntroCallCompleted
	UserStatusPreferencesExtracted
	UserStatusJobSearchStarted
	UserStatusJobSearchComplete
)

type UserState struct {
	Status    UserStatus `firestore:"status"`
	Started   time.Time  `firestore:"started"`
	Completed *time.Time `firestore:"completed"`
	Error     *string    `firestore:"error"`
}

// User represents a user document in Firestore
type User struct {
	ID           string    `firestore:"id"`
	CreatedAt    time.Time `firestore:"created_at"`
	UpdatedAt    time.Time `firestore:"updated_at"`
	Email        string    `firestore:"email"`
	HashedSecret string    `firestore:"hashed_secret,omitempty"`
	IsSuperUser  bool      `firestore:"is_superuser"`

	Provider    string `firestore:"provider,omitempty"`
	Name        string `firestore:"name,omitempty"`
	FirstName   string `firestore:"first_name,omitempty"`
	LastName    string `firestore:"last_name,omitempty"`
	NickName    string `firestore:"nick_name,omitempty"`
	Description string `firestore:"description,omitempty"`
	UserID      string `firestore:"user_id,omitempty"`
	AvatarURL   string `firestore:"avatar_url,omitempty"`
	Location    string `firestore:"location,omitempty"`

	Phone *string `firestore:"phone,omitempty"`

	State UserState `firestore:"state"`

	Tier            UserTier   `firestore:"tier"`
	LastSearchAt    *time.Time `firestore:"last_search_at"`
	SubscriptionEnd *time.Time `firestore:"subscription_end_date"`
}

const (
	usersCollection = "users"
)

type ErrUserExists struct {
	ID string
}

func (e ErrUserExists) Error() string {
	return fmt.Sprintf("user already exists: %s", e.ID)
}

// CreateUser creates a new user document in Firestore
func CreateUser(ctx context.Context, fs *firestore.Client, u *User) error {
	if u.Name == "" {
		return fmt.Errorf("user name is required")
	}

	if u.ID == "" {
		u.ID = funcs.Slugify(fmt.Sprintf("%s %s", u.Name, uuid.New().String()))
	}

	if u.State.Status == UserStatusError || u.State.Status == UserStatusUnknown {
		return fmt.Errorf("user state is invalid")
	}

	u.CreatedAt = time.Now()
	u.UpdatedAt = u.CreatedAt

	if u.Tier == UserTierUnknown {
		u.Tier = UserTierFree
	}

	u.State = UserState{
		Status:    UserStatusPendingResumeUpload,
		Started:   u.CreatedAt,
		Completed: nil,
	}

	_, err := fs.Collection(usersCollection).Doc(u.ID).Create(ctx, u)
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return ErrUserExists{ID: u.ID}
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetUser retrieves a user by ID
func GetUser(ctx context.Context, fs *firestore.Client, id string) (*User, bool, error) {
	doc, err := fs.Collection(usersCollection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get user: %w", err)
	}

	var user User
	if err := doc.DataTo(&user); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal user data: %w", err)
	}

	return &user, true, nil
}

// GetUserByEmail retrieves a user by email
func GetUserByEmail(ctx context.Context, fs *firestore.Client, email string) (*User, bool, error) {
	iter := fs.Collection(usersCollection).Where("email", "==", email).Limit(1).Documents(ctx)
	doc, err := iter.Next()
	if err != nil {
		if err == iterator.Done {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get user by email: %w", err)
	}

	var user User
	if err := doc.DataTo(&user); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal user data: %w", err)
	}

	return &user, true, nil
}

// GetUserByPhone retrieves a user by phone number
func GetUserByPhone(ctx context.Context, fs *firestore.Client, phone string) (*User, bool, error) {
	iter := fs.Collection(usersCollection).Where("phone", "==", phone).Limit(1).Documents(ctx)
	doc, err := iter.Next()
	if err != nil {
		if err == iterator.Done {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get user by phone: %w", err)
	}

	var user User
	if err := doc.DataTo(&user); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal user data: %w", err)
	}

	return &user, true, nil
}

// UpdateUser updates an existing user document
func UpdateUser(ctx context.Context, fs *firestore.Client, id string, updates map[string]interface{}) error {
	var fsUpdates []firestore.Update
	for field, value := range updates {
		fsUpdates = append(fsUpdates, firestore.Update{
			Path:  field,
			Value: value,
		})
	}

	fsUpdates = append(fsUpdates, firestore.Update{
		Path:  "updated",
		Value: time.Now(),
	})

	_, err := fs.Collection(usersCollection).Doc(id).Update(ctx, fsUpdates)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// DeleteUser deletes a user document
func DeleteUser(ctx context.Context, fs *firestore.Client, id string) error {
	_, err := fs.Collection(usersCollection).Doc(id).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	return nil
}

// ListUsers retrieves all users with optional limit and offset
func ListUsers(ctx context.Context, fs *firestore.Client, limit int, offset int) ([]*User, error) {
	query := fs.Collection(usersCollection).OrderBy("created_at", firestore.Asc)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	iter := query.Documents(ctx)
	var users []*User

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate users: %w", err)
		}

		var user User
		if err := doc.DataTo(&user); err != nil {
			return nil, fmt.Errorf("failed to unmarshal user data: %w", err)
		}
		users = append(users, &user)
	}

	return users, nil
}

func UpdateUserStatus(ctx context.Context, fs *firestore.Client, user *User, status UserStatus) error {
	user.State.Status = status
	user.State.Started = time.Now()
	user.State.Completed = nil
	user.State.Error = nil // Clear any previous errors
	user.UpdatedAt = time.Now()

	return UpdateUser(ctx, fs, user.ID, map[string]any{
		"state":      user.State,
		"updated_at": user.UpdatedAt,
	})
}

// UpdateUserStateWithError updates the user state to Error and sets the error message
func UpdateUserStateWithError(ctx context.Context, fs *firestore.Client, user *User, errorMsg string) error {
	user.State.Status = UserStatusError
	user.State.Started = time.Now()
	user.State.Completed = nil
	errorCopy := errorMsg // Make a copy to ensure it stays in memory
	user.State.Error = &errorCopy
	user.UpdatedAt = time.Now()

	return UpdateUser(ctx, fs, user.ID, map[string]any{
		"state":      user.State,
		"updated_at": user.UpdatedAt,
	})
}

func (s UserStatus) String() string {
	switch s {
	case UserStatusError:
		return "Error"
	case UserStatusUnknown:
		return "Unknown"
	case UserStatusPendingResumeUpload:
		return "Pending Resume Upload"
	case UserStatusProcessingResume:
		return "Processing Resume"
	case UserStatusResumeUploaded:
		return "Resume Uploaded"
	case UserStatusIntroCallScheduled:
		return "Call Scheduled"
	case UserStatusIntroCallStarted:
		return "Call Started"
	case UserStatusIntroCallCompleted:
		return "Call Completed"
	case UserStatusPreferencesExtracted:
		return "Preferences Extracted"
	case UserStatusJobSearchStarted:
		return "Job Search Started"
	case UserStatusJobSearchComplete:
		return "Job Search Complete"
	default:
		return "Unknown Status"
	}
}

func (s UserStatus) Description() string {
	switch s {
	case UserStatusError:
		return "The user is in an error state. Check the error field for more information."
	case UserStatusUnknown:
		return "The user status is unknown."
	case UserStatusPendingResumeUpload:
		return "We are waiting for your resume to be uploaded."
	case UserStatusProcessingResume:
		return "The resume is being processed."
	case UserStatusResumeUploaded:
		return "The resume has been successfully uploaded but a call has not been scheduled yet."
	case UserStatusIntroCallScheduled:
		return "We have your resume and a call has been scheduled."
	case UserStatusIntroCallStarted:
		return "Your introductory call has started."
	case UserStatusIntroCallCompleted:
		return "Your introductory call has been completed."
	case UserStatusPreferencesExtracted:
		return "The user's preferences have been extracted."
	case UserStatusJobSearchStarted:
		return "The job search has started."
	case UserStatusJobSearchComplete:
		return "The job search has been completed."
	default:
		return "No description available for this status."
	}
}
