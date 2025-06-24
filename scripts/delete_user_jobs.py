#!/usr/bin/env python3

import os
import sys
import json
import base64
from dotenv import load_dotenv
from google.cloud import firestore
from firebase_admin import credentials, initialize_app
from google.oauth2 import service_account
from tqdm import tqdm

def setup_firebase(env_file=".env"):
    load_dotenv(env_file)
    project_id = os.getenv("FIREBASE_PROJECT_ID")
    if not project_id:
        raise ValueError("FIREBASE_PROJECT_ID must be set in .env file")

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

    firestore_creds = service_account.Credentials.from_service_account_info(credentials_dict)
    print(f"Initializing Firestore client for project: {project_id}")
    db = firestore.Client(project=project_id, credentials=firestore_creds)
    return db

def delete_user_jobs(db, user_id, apply=False):
    user_ref = db.collection("users").document(user_id)
    user_doc = user_ref.get()
    if not user_doc.exists:
        print(f"User '{user_id}' does not exist.")
        return

    user_job_ref = user_ref.collection("user_jobs")
    jobs = list(user_job_ref.stream())
    print(f"Found {len(jobs)} user_job documents for user {user_id}.")

    deleted = 0
    skipped = 0
    
    if not apply:
        print("DRY RUN MODE - No actual deletions will be performed")
    
    for job in tqdm(jobs, desc="Processing user jobs", unit="job"):
        job_data = job.to_dict()
        job_status = job_data.get("Status", "")
        
        if job_status == "saved":
            skipped += 1
            continue
            
        if apply:
            job.reference.delete()
        deleted += 1

    if apply:
        print(f"Deleted {deleted} user_job documents for user {user_id}. Skipped {skipped} saved jobs.")
    else:
        print(f"DRY RUN: Would delete {deleted} user_job documents for user {user_id}. Would skip {skipped} saved jobs.")
        print("Use --apply flag to perform actual deletions.")

def list_all_users(db):
    users_ref = db.collection("users")
    print("Listing all users:")
    users = list(users_ref.stream())
    if not users:
        print("No users found.")
        return
    for user in users:
        data = user.to_dict()
        name = data.get("Name", "<no name>")
        print(f"UserID: {user.id} | Name: {name}")

def main():
    env_file = ".env"
    user_id = None
    apply = False

    for arg in sys.argv[1:]:
        if arg.endswith(".env"):
            env_file = arg
        elif arg.startswith("--user="):
            user_id = arg.split("=", 1)[1]
        elif arg == "--apply":
            apply = True
        elif not arg.startswith("-"):
            user_id = arg
        elif arg in ["-h", "--help"]:
            print("This script deletes all user jobs for a given user. It will not delete 'saved' jobs.")
            print("")
            print("Usage: python delete_user_jobs.py [--user=USER_ID] [--apply] [env_file]")
            print("  --user=USER_ID: User ID to process (optional)")
            print("  --apply: Actually perform deletions (default is dry run)")
            print("  env_file: Path to environment file (default: .env)")
            return

    print(f"Using environment file: {env_file}")
    try:
        db = setup_firebase(env_file)
        if not user_id:
            list_all_users(db)
            return
        print(f"Processing user: {user_id}")
        delete_user_jobs(db, user_id, apply)
    except Exception as e:
        print(f"Error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
