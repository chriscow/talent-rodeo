package internal

import (
	"context"
	"fmt"
	"talent-rodeo/internal/funcs"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"github.com/patrickmn/go-cache"
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

	Search *UserSearch

	Tier            UserTier
	CompletedAt     *time.Time
	SubscriptionEnd *time.Time
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

	return nil
}

// GetUser retrieves a user by ID
func GetUser(ctx context.Context, fs *FirestoreClient, id string) (*User, bool, error) {
	if fs.UsersCache != nil {
		if obj, found := fs.UsersCache.Get(id); found {
			if found {
				return obj.(*User), true, nil
			}
		}
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

	// Cache the user in memory
	if fs.UsersCache != nil {
		fs.UsersCache.Set(id, &user, cache.NoExpiration)
	}

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

	// Cache the user in memory
	if fs.UsersCache != nil {
		fs.UsersCache.Set(user.ID, &user, cache.NoExpiration)
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

	// Cache the user in memory
	if fs.UsersCache != nil {
		fs.UsersCache.Set(user.ID, &user, cache.NoExpiration)
	}

	return &user, true, nil
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

	// Invalidate the user in memory cache
	if fs.UsersCache != nil {
		if _, found := fs.UsersCache.Get(id); found {
			fs.UsersCache.Delete(id)
		}
	}

	return nil
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
		if err := fs.DeleteCollection(ctx, colRef.Path, 100, true); err != nil {
			return fmt.Errorf("failed to delete subcollection %s: %w", colRef.Path, err)
		}
	}

	// Now delete the main user document
	_, err := fs.Collection(usersCollection).Doc(id).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	// Invalidate the user in memory cache
	if fs.UsersCache != nil {
		if _, found := fs.UsersCache.Get(id); found {
			fs.UsersCache.Delete(id)
		}
	}

	return nil
}

// ListUsers retrieves all users with optional limit and offset
func ListUsers(ctx context.Context, fs *FirestoreClient, limit int, offset int) ([]*User, error) {
	// Get all users from the cache if available
	if fs.UsersCache != nil {
		usersMap := fs.UsersCache.Items()
		if len(usersMap) > 0 {
			users := make([]*User, 0, len(usersMap))
			for _, user := range usersMap {
				if u, ok := user.Object.(*User); ok {
					users = append(users, u)
				}
			}
			return users, nil
		}
	}

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

	// Cache the users in memory
	if fs.UsersCache != nil {
		for _, user := range users {
			fs.UsersCache.Set(user.ID, user, cache.NoExpiration)
		}
	}

	return users, nil
}
