#!/usr/bin/env python3

import os
import json
import argparse
from dotenv import load_dotenv
from google.cloud import firestore
from firebase_admin import credentials, initialize_app
from google.oauth2 import service_account
import datetime

def json_serializer(obj):
    if isinstance(obj, (datetime.datetime, datetime.date)):
        return obj.isoformat()
    return str(obj)

def setup_firebase():
    load_dotenv()
    project_id = os.getenv("FIREBASE_PROJECT_ID")
    if not project_id:
        raise ValueError("FIREBASE_PROJECT_ID must be set in .env file")

    creds_file = os.getenv("FIREBASE_CREDENTIALS_FILE")
    if not creds_file or not os.path.isfile(creds_file):
        print(f"FIREBASE_CREDENTIALS_FILE is set to {creds_file}")
        raise ValueError("FIREBASE_CREDENTIALS_FILE must be set in .env and point to a valid file")

    cred = credentials.Certificate(creds_file)
    initialize_app(cred, {'projectId': project_id})
    firestore_creds = service_account.Credentials.from_service_account_file(creds_file)
    db = firestore.Client(project=project_id, credentials=firestore_creds)
    return db

def export_geoids(db, output_file):
    # Query the geo-ids collection
    geoid_collection = 'geo_ids'  # From the Go code: const geoIDCollection = "geo_ids"
    docs = db.collection(geoid_collection).stream()

    geoids = []
    for doc in docs:
        data = doc.to_dict()
        # Ensure ID is included (from the ID of the document)
        data['id'] = doc.id
        geoids.append(data)

    # Write to file
    with open(output_file, 'w') as f:
        json.dump(geoids, indent=2, default=json_serializer, fp=f)
    
    print(f"Exported {len(geoids)} GeoIDs to {output_file}")

def import_geoids(db, input_file, dry_run=False):
    with open(input_file, 'r') as f:
        geoids = json.load(f)
    
    geoid_collection = 'geo_ids'
    batch = db.batch()
    count = 0
    batch_size = 500  # Firestore allows up to 500 operations per batch
    
    print(f"Importing {len(geoids)} GeoIDs from {input_file}")
    if dry_run:
        print("DRY RUN: No data will be written to Firestore")
    
    for geoid in geoids:
        doc_id = geoid.pop('id', None)  # Extract the ID, remove from data to be written
        if not doc_id:
            print(f"Warning: Skipping record without ID: {geoid}")
            continue
            
        # Create document reference
        doc_ref = db.collection(geoid_collection).document(doc_id)
        
        # Add to batch
        batch.set(doc_ref, geoid)
        count += 1
        
        # If we've reached batch size, commit and start a new batch
        if count % batch_size == 0:
            if not dry_run:
                batch.commit()
                print(f"Committed {count} records")
            batch = db.batch()
    
    # Commit any remaining records
    if count % batch_size > 0 and not dry_run:
        batch.commit()
    
    print(f"{'Would import' if dry_run else 'Imported'} {count} GeoID records")

def main():
    parser = argparse.ArgumentParser(description='Export or import GeoIDs from/to Firestore')
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument('-e', '--export', action='store_true', help='Export GeoIDs from Firestore to JSON')
    group.add_argument('-i', '--import', dest='import_data', action='store_true', help='Import GeoIDs from JSON to Firestore')
    parser.add_argument('-f', '--file', default='geoids.json', help='JSON file path (default: geoids.json)')
    parser.add_argument('--dry-run', action='store_true', help='For import: validate but do not write to database')
    args = parser.parse_args()
    
    db = setup_firebase()
    
    if args.export:
        export_geoids(db, args.file)
    elif args.import_data:
        import_geoids(db, args.file, args.dry_run)

if __name__ == "__main__":
    main()
