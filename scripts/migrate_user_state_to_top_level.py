#!/usr/bin/env python3

import os
import sys
from dotenv import load_dotenv
from google.cloud import firestore
from firebase_admin import credentials, initialize_app
from google.oauth2 import service_account
from datetime import datetime

USER_COLLECTION = "users"

def setup_firebase(env_file=".env"):
    load_dotenv(env_file)
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

def migrate_user_state_to_top_level(db, dry_run=True):
    """
    Migrate User documents from the old State structure to the new flat structure.
    
    Old structure:
    {
        "ID": "...",
        "Name": "...",
        "State": {
            "Status": 5,
            "Started": timestamp,
            "Error": "error message" or null
        }
    }
    
    New structure:
    {
        "ID": "...",
        "Name": "...",
        "Status": 5,
        "StatusUpdatedAt": timestamp,
        "Error": "error message" or null
    }
    """
    users_ref = db.collection(USER_COLLECTION)
    docs = list(users_ref.stream())
    print(f"Found {len(docs)} user documents")
    
    migrated = 0
    already_migrated = 0
    no_state_field = 0
    errors = 0
    
    for doc in docs:
        try:
            data = doc.to_dict()
            user_id = doc.id
            
            # Check if already migrated (has Status but no State)
            if "Status" in data and "State" not in data:
                print(f"User {user_id} already migrated (has Status, no State)")
                already_migrated += 1
                continue
            
            # Check if State field exists
            if "State" not in data:
                print(f"User {user_id} has no State field, skipping")
                no_state_field += 1
                continue
            
            state = data["State"]
            if not isinstance(state, dict):
                print(f"User {user_id} has invalid State field (not a dict), skipping")
                errors += 1
                continue
            
            # Prepare updates
            updates = {}
            
            # Move Status from State.Status to top level
            if "Status" in state:
                updates["Status"] = state["Status"]
                print(f"User {user_id}: Moving Status {state['Status']} to top level")
            
            # Move Started from State.Started to StatusUpdatedAt
            if "Started" in state:
                updates["StatusUpdatedAt"] = state["Started"]
                print(f"User {user_id}: Moving Started to StatusUpdatedAt")
            elif "Status" in state:
                # If no Started field but we have Status, set StatusUpdatedAt to now
                updates["StatusUpdatedAt"] = datetime.now()
                print(f"User {user_id}: No Started field, setting StatusUpdatedAt to now")
            
            # Move Error from State.Error to top level
            if "Error" in state:
                updates["Error"] = state["Error"]
                print(f"User {user_id}: Moving Error to top level")
            
            # Remove the State field
            updates["State"] = firestore.DELETE_FIELD
            
            if not updates:
                print(f"User {user_id}: No updates needed")
                continue
            
            if dry_run:
                print(f"DRY RUN - Would update User {user_id} with: {updates}")
                migrated += 1
            else:
                # Perform the update
                users_ref.document(user_id).update(updates)
                print(f"Updated User {user_id}")
                migrated += 1
                
        except Exception as e:
            print(f"Error processing user {doc.id}: {e}")
            errors += 1
    
    print(f"\nMigration Summary:")
    print(f"  Migrated: {migrated}")
    print(f"  Already migrated: {already_migrated}")
    print(f"  No State field: {no_state_field}")
    print(f"  Errors: {errors}")
    print(f"  Total processed: {migrated + already_migrated + no_state_field + errors}")

def main():
    # Parse command line arguments
    env_file = ".env"  # default
    dry_run = True
    
    for arg in sys.argv[1:]:
        if arg == "--apply":
            dry_run = False
        elif arg.endswith(".env"):
            env_file = arg
    
    print(f"Using environment file: {env_file}")
    db = setup_firebase(env_file)
    
    # Check if this is a dry run or actual migration
    if not dry_run:
        dry_run = False
        print("APPLYING MIGRATION - THIS WILL MODIFY DATA")
        confirm = input("Are you sure you want to proceed? (yes/no): ")
        if confirm.lower() != "yes":
            print("Migration cancelled")
            return
    else:
        print("DRY RUN MODE - No changes will be made")
        print("Use --apply flag to actually perform the migration")
    
    migrate_user_state_to_top_level(db, dry_run)

if __name__ == "__main__":
    main() 