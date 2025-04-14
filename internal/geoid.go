package internal

import (
	"context"

	"cloud.google.com/go/firestore"
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
	ID    string
	Value string
}

func GetGeoID(fs *firestore.Client, id string) (GeoID, error) {
	doc, err := fs.Collection("geo_ids").Doc(id).Get(context.Background())
	if err != nil {
		return GeoID{}, err
	}
	return GeoID{ID: doc.Data()["id"].(string), Value: doc.Data()["value"].(string)}, nil
}

func ListGeoIDs(fs *firestore.Client) ([]GeoID, error) {
	geoIDs := []GeoID{}
	iter := fs.Collection("geo_ids").Documents(context.Background())
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		geoID := GeoID{
			ID:    doc.Data()["id"].(string),
			Value: doc.Data()["value"].(string),
		}
		geoIDs = append(geoIDs, geoID)
	}
	return geoIDs, nil
}

func GetGeoIDKeys(fs *firestore.Client) ([]string, error) {
	geoIDs, err := ListGeoIDs(fs)
	if err != nil {
		return nil, err
	}
	keys := []string{}
	for _, geoID := range geoIDs {
		keys = append(keys, geoID.ID)
	}
	return keys, nil
}
