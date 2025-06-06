package internal

import (
	"context"
	"fmt"
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

	usersCollection = "users"

	UserAccessUpdateThreshold = time.Hour // 1 hour
)

// User represents a user document in Firestore
type User struct {
	ID           string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Email        string
	HashedSecret string
	IsSuperUser  bool

	Provider    string
	Name        string
	FirstName   string
	LastName    string
	NickName    string
	Description string
	UserID      string
	AvatarURL   string
	Location    string

	Phone *string

	State UserState

	SteeringQuery      *SteeringQuery
	LocationPreference *string

	Tier            UserTier
	CompletedAt     *time.Time
	SubscriptionEnd *time.Time

	LastAccessedAt time.Time // Last time the user accessed the system
}

type ErrUserExists struct {
	ID string
}

func (e ErrUserExists) Error() string {
	return fmt.Sprintf("user already exists: %s", e.ID)
}

// CreateUser creates a new user document in Firestore
func CreateUser(ctx context.Context, fs *FirestoreClient, u *User) error {
	if u.Name == "" {
		return fmt.Errorf("user name is required")
	}

	if u.ID == "" {
		u.ID = uuid.New().String()
	}

	if u.State.Status == UserStatusError || u.State.Status == UserStatusUnknown {
		return fmt.Errorf("user state is invalid: %s", u.State.Status.String())
	}

	u.CreatedAt = time.Now()
	u.UpdatedAt = u.CreatedAt

	if u.Tier == UserTierUnknown {
		u.Tier = UserTierFree
	}

	u.State = UserState{
		Status:  UserStatusPendingResumeUpload,
		Started: u.CreatedAt,
	}

	_, err := fs.Collection(usersCollection).Doc(u.ID).Create(ctx, u)
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return ErrUserExists{ID: u.ID}
		}
		return fmt.Errorf("failed to create user: %w", err)
	}
	fs.SetUserCache(u.ID, u)
	return nil
}

// GetUser retrieves a user by ID
func GetUser(ctx context.Context, fs *FirestoreClient, id string) (*User, bool, error) {
	if user, found := fs.GetUserFromCache(id); found {
		return user, true, nil
	}
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

	fs.SetUserCache(id, &user)
	return &user, true, nil
}

// GetUserByEmail retrieves a user by email
func GetUserByEmail(ctx context.Context, fs *FirestoreClient, email string) (*User, bool, error) {
	iter := fs.Collection(usersCollection).Where("Email", "==", email).Limit(1).Documents(ctx)
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
func GetUserByPhone(ctx context.Context, fs *FirestoreClient, phone string) (*User, bool, error) {
	iter := fs.Collection(usersCollection).Where("Phone", "==", phone).Limit(1).Documents(ctx)
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

func GetUsersByIDs(ctx context.Context, fs *FirestoreClient, ids []string) ([]*User, error) {
	iter := fs.Collection(usersCollection).Where("ID", "in", ids).Documents(ctx)
	docs, err := iter.GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to get users by IDs: %w", err)
	}

	users := make([]*User, len(docs))
	for i, doc := range docs {
		var user User
		if err := doc.DataTo(&user); err != nil {
			return nil, fmt.Errorf("failed to unmarshal user data: %w", err)
		}
		users[i] = &user
	}

	return users, nil
}

func GetUserSteeringQuery(ctx context.Context, fs *FirestoreClient, id string) (*SteeringQuery, error) {
	doc, err := fs.Collection(usersCollection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	var user User
	if err := doc.DataTo(&user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user data: %w", err)
	}

	if user.SteeringQuery == nil {
		return nil, fmt.Errorf("steering query not set for user %s", id)
	}

	return user.SteeringQuery, nil
}

func SetUserSteeringQuery(ctx context.Context, fs *FirestoreClient, id string, sq SteeringQuery) error {
	// if the steering query is not valid, return an error
	if err := sq.Validate(); err != nil {
		return fmt.Errorf("invalid steering query: %w", err)
	}

	fsUpdates := []firestore.Update{
		{
			Path:  "SteeringQuery",
			Value: sq,
		},
	}

	_, err := fs.Collection(usersCollection).Doc(id).Update(ctx, fsUpdates)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to update steering query: %w", err)
	}

	return nil
}

func SetUserWorkPreferences(ctx context.Context, fs *FirestoreClient, id string, wp string) error {
	// if the work preferences is not valid, return an error
	if len(wp) > 1000 {
		println("work preferences is too long", len(wp))
		println(wp)
	}

	fsUpdates := []firestore.Update{
		{
			Path:  "WorkPreferences",
			Value: wp,
		},
	}
	_, err := fs.Collection(usersCollection).Doc(id).Update(ctx, fsUpdates)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to update work preferences: %w", err)
	}
	return nil
}

// UpdateUser updates an existing user document
func UpdateUser(ctx context.Context, fs *FirestoreClient, id string, updates map[string]any) error {
	var fsUpdates []firestore.Update

	for field, value := range updates {

		// if any field name is not capitalized, return an error
		if field[0] < 'A' || field[0] > 'Z' {
			return fmt.Errorf("field name %s must be capitalized", field)
		}

		fsUpdates = append(fsUpdates, firestore.Update{
			Path:  field,
			Value: value,
		})
	}

	fsUpdates = append(fsUpdates, firestore.Update{
		Path:  "UpdatedAt",
		Value: time.Now(),
	})

	_, err := fs.Collection(usersCollection).Doc(id).Update(ctx, fsUpdates)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to update user: %w", err)
	}
	// Invalidate cache so next read is fresh
	fs.EvictUserCache(id)
	return nil
}

func UpdateUserTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, id string, updates map[string]any) error {

	var fsUpdates []firestore.Update

	for field, value := range updates {

		fsUpdates = append(fsUpdates, firestore.Update{
			Path:  field,
			Value: value,
		})
	}

	fsUpdates = append(fsUpdates, firestore.Update{
		Path:  "UpdatedAt",
		Value: time.Now(),
	})

	return tx.Update(fs.Collection(usersCollection).Doc(id), fsUpdates)
}

// DeleteUser deletes a user document and all its subcollections
func DeleteUser(ctx context.Context, fs *FirestoreClient, id string) error {

	// Find all subcollections and delete each one
	subcollections := fs.Collection(usersCollection).Doc(id).Collections(ctx)
	for {
		colRef, err := subcollections.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to get subcollection: %w", err)
		}

		// Delete this subcollection using the generic DeleteCollection function
		if err := DeleteCollection(ctx, fs, colRef.Path, 100, true); err != nil {
			return fmt.Errorf("failed to delete subcollection %s: %w", colRef.Path, err)
		}
	}

	// Now delete the main user document
	_, err := fs.Collection(usersCollection).Doc(id).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	fs.EvictUserCache(id)
	return nil
}

// ListUsers retrieves all users with optional limit and offset
func ListUsers(ctx context.Context, fs *FirestoreClient, limit int, offset int) ([]*User, error) {

	query := fs.Collection(usersCollection).OrderBy("CreatedAt", firestore.Asc)
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

// SaveUser saves or updates a user in Firestore
func SaveUser(ctx context.Context, fs *FirestoreClient, user *User) error {
	if user.ID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}

	_, err := fs.Collection("users").Doc(user.ID).Set(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}
	fs.SetUserCache(user.ID, user)
	return nil
}
