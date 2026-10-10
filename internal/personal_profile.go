package internal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"talent-rodeo/internal/funcs"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// PersonalPreferences contains the search choices the user explicitly confirmed in writing.
type PersonalPreferences struct {
	TargetRoles   []string
	Locations     string
	WorkTypes     []WorkType
	Seniority     ExperienceLevel
	DesiredTraits string
	AvoidedTraits string
	Timezone      string
}

// ProfileStatementGenerator regenerates a statement from confirmed profile source data.
type ProfileStatementGenerator struct{}

// GenerateStatement implements StatementGenerator without calling an LLM or changing stored versions.
func (ProfileStatementGenerator) GenerateStatement(_ context.Context, version *UserStatementVersion) (string, error) {
	if version == nil {
		return "", fmt.Errorf("profile statement version is nil")
	}
	return RegenerateProfileStatement(*version)
}

// Validate checks the fields needed to create search criteria without guessing intent.
func (p PersonalPreferences) Validate() error {
	if len(p.TargetRoles) == 0 || len(p.TargetRoles) > 5 {
		return fmt.Errorf("choose one to five target roles")
	}
	seenRoles := make(map[string]bool, len(p.TargetRoles))
	for _, role := range p.TargetRoles {
		if strings.TrimSpace(role) == "" || len(role) > 100 {
			return fmt.Errorf("target roles must be between 1 and 100 characters")
		}
		key := strings.ToLower(strings.TrimSpace(role))
		if seenRoles[key] {
			return fmt.Errorf("target roles must be distinct")
		}
		seenRoles[key] = true
	}
	if strings.TrimSpace(p.Locations) == "" || len(p.Locations) > 300 {
		return fmt.Errorf("enter a location preference of at most 300 characters")
	}
	if len(p.WorkTypes) == 0 || len(p.WorkTypes) > 3 {
		return fmt.Errorf("choose one to three work arrangements")
	}
	seenWorkTypes := make(map[WorkType]bool, len(p.WorkTypes))
	for _, workType := range p.WorkTypes {
		switch workType {
		case WorkTypeRemote, WorkTypeHybrid, WorkTypeAtWork:
		default:
			return fmt.Errorf("invalid work arrangement")
		}
		if seenWorkTypes[workType] {
			return fmt.Errorf("work arrangements must be distinct")
		}
		seenWorkTypes[workType] = true
	}
	switch p.Seniority {
	case ExperienceLevelInternship, ExperienceLevelEntryLevel, ExperienceLevelAssociate, ExperienceLevelMidSeniorLevel, ExperienceLevelDirector:
	default:
		return fmt.Errorf("choose a seniority level")
	}
	if len(p.DesiredTraits) > 2000 || len(p.AvoidedTraits) > 2000 {
		return fmt.Errorf("desired and avoided traits must each be at most 2000 characters")
	}
	if p.Timezone == "" {
		return fmt.Errorf("choose a timezone")
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return fmt.Errorf("invalid timezone: %w", err)
	}
	return nil
}

// BuildPersonalSteeringQuery uses only explicit choices and the uploaded resume.
func BuildPersonalSteeringQuery(resume Resume, p PersonalPreferences) (SteeringQuery, error) {
	if err := p.Validate(); err != nil {
		return SteeringQuery{}, err
	}
	if strings.TrimSpace(resume.RawText) == "" {
		return SteeringQuery{}, fmt.Errorf("resume text is required")
	}
	toward := "I am looking for " + strings.Join(p.TargetRoles, ", ") + " roles"
	if p.DesiredTraits != "" {
		toward += ". I value " + p.DesiredTraits
	}
	toward += ". My resume describes: " + personalResumeContext(resume)
	away := ""
	forceAway := float64(0)
	if p.AvoidedTraits != "" {
		away = "I want to avoid " + p.AvoidedTraits
		forceAway = DefaultForceAway
	}
	rerank := toward + ". I prefer " + p.Locations + " and " + strings.Join(personalWorkTypeNames(p.WorkTypes), ", ") + " work."
	if away != "" {
		rerank += " " + away + "."
	}
	query := SteeringQuery{
		Queries:         append([]string(nil), p.TargetRoles...),
		ExperienceLevel: p.Seniority,
		WorkTypes:       append([]WorkType(nil), p.WorkTypes...),
		MoveToward:      toward,
		MoveAway:        away,
		ForceAway:       forceAway,
		RerankQuery:     rerank,
	}
	return query, query.Validate()
}

func personalWorkTypeNames(workTypes []WorkType) []string {
	names := make([]string, 0, len(workTypes))
	for _, workType := range workTypes {
		switch workType {
		case WorkTypeAtWork:
			names = append(names, "on-site")
		case WorkTypeHybrid:
			names = append(names, "hybrid")
		case WorkTypeRemote:
			names = append(names, "remote")
		}
	}
	return names
}

func personalResumeContext(resume Resume) string {
	context := strings.TrimSpace(resume.RawText)
	if resume.SearchAnalysis != nil && strings.TrimSpace(*resume.SearchAnalysis) != "" {
		context = strings.TrimSpace(*resume.SearchAnalysis)
	}
	if resume.Annotated != nil && strings.TrimSpace(*resume.Annotated) != "" {
		context = strings.TrimSpace(*resume.Annotated)
	}
	runes := []rune(context)
	if len(runes) > 1500 {
		return string(runes[:1500])
	}
	return context
}

func personalStatement(resume Resume, p PersonalPreferences) string {
	statement := "I am seeking " + strings.Join(p.TargetRoles, ", ") + " roles at the " + string(p.Seniority) + " level. " +
		"My location preference is " + p.Locations + ", and I prefer " + strings.Join(personalWorkTypeNames(p.WorkTypes), ", ") + " work."
	if p.DesiredTraits != "" {
		statement += " I value " + p.DesiredTraits + "."
	}
	if p.AvoidedTraits != "" {
		statement += " I want to avoid " + p.AvoidedTraits + "."
	}
	statement += " My resume describes: " + personalResumeContext(resume)
	return statement
}

// SavePersonalProfile atomically saves a versioned statement, search inputs, and explicit start marker.
// Repeating an unchanged confirmation returns the current activation without a duplicate version.
func SavePersonalProfile(ctx context.Context, fs *FirestoreClient, userID string, resume Resume, p PersonalPreferences, criteriaIDs []string) (time.Time, error) {
	if fs == nil || userID == "" || resume.UserID != userID || resume.ID == "" || !resume.IsValid {
		return time.Time{}, fmt.Errorf("valid owner and resume are required")
	}
	if len(criteriaIDs) == 0 {
		return time.Time{}, fmt.Errorf("search criteria are required")
	}
	wantedCriteria := make(map[string]bool, len(criteriaIDs))
	for _, criteriaID := range criteriaIDs {
		if criteriaID == "" {
			return time.Time{}, fmt.Errorf("search criteria ID cannot be empty")
		}
		wantedCriteria[criteriaID] = true
	}
	query, err := BuildPersonalSteeringQuery(resume, p)
	if err != nil {
		return time.Time{}, err
	}
	statement := personalStatement(resume, p)
	if err := ValidateUserStatement(userID, statement); err != nil {
		return time.Time{}, err
	}
	fingerprintCriteria := append([]string(nil), criteriaIDs...)
	sort.Strings(fingerprintCriteria)
	fingerprintData, err := json.Marshal(struct {
		ResumeID    string
		Preferences PersonalPreferences
		CriteriaIDs []string
	}{ResumeID: resume.ID, Preferences: p, CriteriaIDs: fingerprintCriteria})
	if err != nil {
		return time.Time{}, fmt.Errorf("encode personal profile: %w", err)
	}
	fingerprintBytes := sha256.Sum256(fingerprintData)
	fingerprint := hex.EncodeToString(fingerprintBytes[:])
	var activation time.Time
	err = fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		userRef := fs.Collection(usersCollection).Doc(userID)
		userDoc, err := tx.Get(userRef)
		if err != nil {
			return fmt.Errorf("read profile owner: %w", err)
		}
		var user User
		if err := userDoc.DataTo(&user); err != nil {
			return fmt.Errorf("decode profile owner: %w", err)
		}
		if user.ID != userID {
			return fmt.Errorf("profile owner mismatch")
		}
		currentSubscriptions := make(map[string]*firestore.DocumentRef)
		iter := tx.Documents(fs.Collection(userCriteriaCollection).Where("UserID", "==", userID))
		defer iter.Stop()
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return fmt.Errorf("read current criteria subscriptions: %w", err)
			}
			var subscription SearchCriteriaSubscription
			if err := doc.DataTo(&subscription); err != nil {
				return fmt.Errorf("decode criteria subscription: %w", err)
			}
			currentSubscriptions[subscription.CriteriaID] = doc.Ref
		}
		versionNumber := 1
		var previous *UserStatementVersion
		if user.CurrentStatementVersionID != nil {
			previousRef := userRef.Collection(userStatementsCollection).Doc(*user.CurrentStatementVersionID)
			previousDoc, err := tx.Get(previousRef)
			if err != nil {
				return fmt.Errorf("read current statement: %w", err)
			}
			previous = &UserStatementVersion{}
			if err := previousDoc.DataTo(previous); err != nil {
				return fmt.Errorf("decode current statement: %w", err)
			}
		} else {
			versionIter := tx.Documents(userRef.Collection(userStatementsCollection).OrderBy("VersionNumber", firestore.Desc).Limit(1))
			defer versionIter.Stop()
			previousDoc, err := versionIter.Next()
			if err != nil && err != iterator.Done {
				return fmt.Errorf("find latest statement: %w", err)
			}
			if err == nil {
				previous = &UserStatementVersion{}
				if err := previousDoc.DataTo(previous); err != nil {
					return fmt.Errorf("decode latest statement: %w", err)
				}
			}
		}
		if previous != nil {
			versionNumber = previous.VersionNumber + 1
			subscriptionsMatch := len(currentSubscriptions) == len(wantedCriteria)
			for criteriaID := range wantedCriteria {
				if _, found := currentSubscriptions[criteriaID]; !found {
					subscriptionsMatch = false
				}
			}
			statusReady := user.Status == UserStatusJobSearchScheduled || user.Status == UserStatusJobSearchStarted || user.Status == UserStatusJobSearchComplete
			if previous.GeneratedBy == "profile" && previous.SourceData["profile_fingerprint"] == fingerprint && user.PersonalSearchStartedAt != nil && statusReady && subscriptionsMatch {
				activation = *user.PersonalSearchStartedAt
				return nil
			}
		}
		now := time.Now().UTC()
		activation = now
		version := &UserStatementVersion{
			ID: fmt.Sprintf("%s_v%d", userID, versionNumber), VersionNumber: versionNumber,
			IsLatest: true, StatementText: statement, StatementHash: generateStatementHash(statement),
			GeneratedBy: "profile", GenerationReason: "Confirmed written preferences and uploaded resume",
			SourceData: map[string]any{
				"source_type": "profile", "profile_fingerprint": fingerprint,
				"resume_id": resume.ID, "resume_raw_text": resume.RawText,
				"resume_annotated": resume.Annotated, "resume_search_analysis": resume.SearchAnalysis,
				"criteria_ids": fingerprintCriteria,
				"preferences": map[string]any{
					"target_roles": p.TargetRoles, "locations": p.Locations, "work_types": p.WorkTypes,
					"seniority": p.Seniority, "desired_traits": p.DesiredTraits,
					"avoided_traits": p.AvoidedTraits, "timezone": p.Timezone,
				},
			},
			CreatedAt: now, ChangeType: "initial", SignificantChange: true, SystemType: "profile",
		}
		if previous != nil {
			version.PreviousVersionID = &previous.ID
			version.ChangeType = "correction"
			if err := tx.Update(userRef.Collection(userStatementsCollection).Doc(previous.ID), []firestore.Update{{Path: "IsLatest", Value: false}}); err != nil {
				return fmt.Errorf("supersede previous statement: %w", err)
			}
		}
		for criteriaID, ref := range currentSubscriptions {
			if wantedCriteria[criteriaID] {
				continue
			}
			if err := tx.Delete(ref); err != nil {
				return fmt.Errorf("remove stale owner subscription: %w", err)
			}
			if err := tx.Update(fs.Collection(searchCriteriaCollection).Doc(criteriaID), []firestore.Update{{Path: "SubscriberCount", Value: firestore.Increment(-1)}}); err != nil {
				return fmt.Errorf("decrement stale criteria subscribers: %w", err)
			}
		}
		for criteriaID := range wantedCriteria {
			if _, found := currentSubscriptions[criteriaID]; found {
				continue
			}
			ref := fs.Collection(userCriteriaCollection).Doc(funcs.Slugify(fmt.Sprintf("%s_%s", userID, criteriaID)))
			if err := tx.Create(ref, SearchCriteriaSubscription{UserID: userID, CriteriaID: criteriaID, SubscribedAt: now}); err != nil {
				return fmt.Errorf("subscribe owner to criteria: %w", err)
			}
			if err := tx.Update(fs.Collection(searchCriteriaCollection).Doc(criteriaID), []firestore.Update{{Path: "SubscriberCount", Value: firestore.Increment(1)}}); err != nil {
				return fmt.Errorf("increment criteria subscribers: %w", err)
			}
		}
		if err := tx.Create(userRef.Collection(userStatementsCollection).Doc(version.ID), version); err != nil {
			return fmt.Errorf("create profile statement: %w", err)
		}
		if err := tx.Update(userRef, []firestore.Update{
			{Path: "SteeringQuery", Value: query},
			{Path: "LocationPreference", Value: p.Locations},
			{Path: "WorkPreferences", Value: p.Locations},
			{Path: "Timezone", Value: p.Timezone},
			{Path: "Statement", Value: statement},
			{Path: "CurrentStatementVersionID", Value: version.ID},
			{Path: "StatementLastUpdated", Value: now},
			{Path: "PersonalSearchStartedAt", Value: now},
			{Path: "Status", Value: UserStatusJobSearchScheduled},
			{Path: "StatusUpdatedAt", Value: now},
			{Path: "Error", Value: nil},
		}); err != nil {
			return fmt.Errorf("save personal profile: %w", err)
		}
		return nil
	})
	if err != nil {
		return time.Time{}, err
	}
	fs.EvictUserCache(userID)
	return activation, nil
}
