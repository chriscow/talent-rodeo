package internal

import (
	"context"

	"google.golang.org/api/iterator"
)

/*

var (
	GeoIDMap = map[string]GeoID{
		"Remote":                            "103644278",
		"United States":                     "103644278",
		"Greater New York Area":             "",
		"Greater Los Angeles Area":          "",
		"Greater Chicago Area":              "",
		"Greater Dallas-Fort Worth Area":    "",
		"Greater Houston Area":              "",
		"Greater Washington DC Area":        "",
		"Greater Philadelphia Area":         "",
		"Greater San Francisco Area":        "",
		"Greater Miami Area":                "",
		"Greater Phoenix Area":              "",
		"Greater Boston Area":               "",
		"Greater Atlanta Area":              "",
		"Greater Detroit Area":              "",
		"Greater Seattle Area":              "",
		"Greater Minneapolis-St. Paul Area": "",
		"Greater San Diego Area":            "",
		"Greater Tampa Area":                "",
		"Greater Denver Area":               "",
		"Greater Baltimore Area":            "",
	}
)
*/

type GeoID struct {
	ID    string `firestore:"id"`
	Value string `firestore:"value"`
}

func GetGeoID(ctx context.Context, fs *FirestoreClient, id string) (GeoID, error) {
	doc, err := fs.Collection("geo_ids").Doc(id).Get(ctx)
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
	iter := fs.Collection("geo_ids").Documents(ctx)

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
