package internal

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chriscow/minds/tools"
	"google.golang.org/api/iterator"
)

const geoIDCollection = "geo_ids"

// GeoID represents a geographic location identifier
type GeoID struct {
	ID string
	// Value is the display name or description of the GeoID
	Value string
}

func CreateGeoID(ctx context.Context, fs *FirestoreClient, geoID GeoID) error {
	// Ensure the ID field is not empty
	if geoID.ID == "" {
		return fmt.Errorf("geoID ID cannot be empty")
	}
	if geoID.Value == "" {
		return fmt.Errorf("geoID Value cannot be empty")
	}
	// Create or update the GeoID document in Firestore
	_, err := fs.Collection(geoIDCollection).Doc(geoID.ID).Set(ctx, geoID)
	if err != nil {
		return fmt.Errorf("failed to create or update GeoID: %w", err)
	}
	return nil
}

func GetGeoID(ctx context.Context, fs *FirestoreClient, id string) (GeoID, error) {
	doc, err := fs.Collection(geoIDCollection).Doc(id).Get(ctx)
	if err != nil {
		return GeoID{}, err
	}

	var geoID GeoID
	if err := doc.DataTo(&geoID); err != nil {
		return GeoID{}, err
	}
	return geoID, nil
}

func ListGeoIDs(ctx context.Context, fs *FirestoreClient) ([]GeoID, error) {
	geoIDs := []GeoID{}
	iter := fs.Collection(geoIDCollection).Documents(ctx)

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var geoID GeoID
		if err := doc.DataTo(&geoID); err != nil {
			return nil, err
		}
		geoID.ID = doc.Ref.ID
		geoIDs = append(geoIDs, geoID)
	}
	return geoIDs, nil
}

func GetGeoIDKeys(ctx context.Context, fs *FirestoreClient) ([]string, error) {
	geoIDs, err := ListGeoIDs(ctx, fs)
	if err != nil {
		return nil, err
	}
	keys := []string{}
	for _, geoID := range geoIDs {
		keys = append(keys, geoID.ID)
	}
	return keys, nil
}

func ExtractGeoID(ctx context.Context, fs *FirestoreClient, data string) (string, error) {
	geoids, err := ListGeoIDs(ctx, fs)
	if err != nil {
		return "", err
	}

	if len(geoids) == 0 {
		return "", fmt.Errorf("no geoids found")
	}

	geoidJSON, err := json.Marshal(geoids)
	if err != nil {
		return "", err
	}

	type result struct {
		ID string `json:"id"`
	}

	prompt := `Evaaluate the following statement and determine the ID of the location
	closest and most relevant to the statement. If you cannot determine the location,
	or it explicitly asks for report, select ID "103644278" which is "Remote"

	Geoids:
	%s

	Statement: %s
	`

	res, err := tools.StructuredAsk[result](ctx, "location-id", fmt.Sprintf(prompt, string(geoidJSON), data), tools.WithModel(tools.GPT41Mini))
	if err != nil {
		return "", err
	}

	return res.ID, nil
}
