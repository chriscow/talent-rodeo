#!/usr/bin/env python3

import os
import sys
import json
import base64
from dotenv import load_dotenv
from google.cloud import firestore
from firebase_admin import credentials, initialize_app, _apps
from google.oauth2 import service_account

JOB_DETAILS_COLLECTION = "job_details"
USERS_COLLECTION = "users"
USER_JOBS_SUBCOLLECTION = "user_jobs"

def setup_firebase(env_file=".env"):
    load_dotenv(env_file)
    project_id = os.getenv("FIREBASE_PROJECT_ID")
    if not project_id:
        raise ValueError("FIREBASE_PROJECT_ID must be set in .env file")

    if not _apps:
        firebase_credentials = os.getenv("FIREBASE_CREDENTIALS")
        if not firebase_credentials:
            raise ValueError("FIREBASE_CREDENTIALS must be set in .env file (base64 encoded JSON)")
        try:
            credentials_json = base64.b64decode(firebase_credentials).decode('utf-8')
            credentials_dict = json.loads(credentials_json)
        except Exception as e:
            raise ValueError(f"Failed to decode FIREBASE_CREDENTIALS: {e}")
        
        cred = credentials.Certificate(credentials_dict)
        initialize_app(cred, {'projectId': project_id})
    
    firestore_creds_json = base64.b64decode(os.getenv("FIREBASE_CREDENTIALS")).decode('utf-8')
    firestore_creds_dict = json.loads(firestore_creds_json)
    firestore_creds = service_account.Credentials.from_service_account_info(firestore_creds_dict)
    db = firestore.Client(project=project_id, credentials=firestore_creds)
    return db

def cleanup_duplicate_user_jobs(db, dry_run=True):
    print("--- Step 1: Building map of all duplicate jobs ---")
    job_details_ref = db.collection(JOB_DETAILS_COLLECTION)
    
    # Get all jobs that are marked as duplicates
    query = job_details_ref.where("CanonicalJobID", "!=", None)
    duplicate_docs = list(query.stream())
    
    duplicate_map = {doc.id: doc.to_dict()["CanonicalJobID"] for doc in duplicate_docs}
    
    if not duplicate_map:
        print("No duplicate job details found. Nothing to do.")
        return

    print(f"Found {len(duplicate_map)} total job detail documents marked as duplicates.")
    
    print("\n--- Step 2: Processing users and their jobs ---")
    users_ref = db.collection(USERS_COLLECTION)
    all_users = list(users_ref.stream())
    
    total_deleted_count = 0
    
    for user_doc in all_users:
        user_id = user_doc.id
        user_jobs_ref = user_doc.reference.collection(USER_JOBS_SUBCOLLECTION)
        
        user_jobs = list(user_jobs_ref.stream())
        if not user_jobs:
            continue
            
        user_job_ids = {job.id for job in user_jobs}
        
        jobs_to_delete = []
        
        for job in user_jobs:
            # Check if this user job is a known duplicate
            if job.id in duplicate_map:
                canonical_id = duplicate_map[job.id]
                
                # Check if the user ALSO has the canonical version of this job
                if canonical_id in user_job_ids:
                    jobs_to_delete.append(job.id)
        
        if jobs_to_delete:
            print(f"\nUser {user_id}:")
            for job_id in jobs_to_delete:
                if dry_run:
                    print(f"  DRY RUN: Would delete duplicate UserJob '{job_id}' (points to canonical '{duplicate_map[job_id]}')")
                else:
                    try:
                        user_jobs_ref.document(job_id).delete()
                        print(f"  DELETED duplicate UserJob '{job_id}'")
                    except Exception as e:
                        print(f"  ERROR deleting UserJob '{job_id}': {e}")
                
                total_deleted_count += 1

    print("\n--- Cleanup Summary ---")
    if dry_run:
        print(f"DRY RUN: Found {total_deleted_count} duplicate UserJob entries that would be deleted.")
        print("\nRun with --apply to execute the deletion.")
    else:
        print(f"DELETED {total_deleted_count} duplicate UserJob entries.")

def main():
    env_file = ".env"
    dry_run = True
    
    for arg in sys.argv[1:]:
        if arg == "--apply":
            dry_run = False
        elif arg.endswith(".env"):
            env_file = arg
            
    print(f"Using environment file: {env_file}")
    db = setup_firebase(env_file)
    
    if not dry_run:
        print("\n!!! APPLYING CLEANUP - THIS WILL DELETE DATA !!!")
        confirm = input("Are you sure you want to delete duplicate UserJob entries? (yes/no): ")
        if confirm.lower() != "yes":
            print("Cleanup cancelled.")
            return
    else:
        print("\n--- DRY RUN MODE ---")
    
    cleanup_duplicate_user_jobs(db, dry_run)

if __name__ == "__main__":
    main() 