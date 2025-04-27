package internal

import (
	"context"

	"google.golang.org/api/iterator"
)

const geoIDCollection = "geo-ids"

// GeoID represents a geographic location identifier
// Note: After the reversal operation, ID contains what was previously in Value field
// and Value contains what was previously the document ID
type GeoID struct {
	ID    string
	Value string
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
