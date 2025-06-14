#!/usr/bin/env python3

import os
import sys
from dotenv import load_dotenv
from google.cloud import firestore
from firebase_admin import credentials, initialize_app
from google.oauth2 import service_account

USER_ID = "7e2dca72-aacc-44db-8fdb-bb6702e4e1a8"
USER_COLLECTION = "users"
USER_JOBS_SUBCOLLECTION = "user_jobs"

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

def main():
    db = setup_firebase()
    user_jobs_ref = db.collection(USER_COLLECTION).document(USER_ID).collection(USER_JOBS_SUBCOLLECTION)
    docs = list(user_jobs_ref.stream())
    print(f"Found {len(docs)} user_jobs for user {USER_ID}")

    updates = 0
    unchanged = 0
    skipped = 0
    for doc in docs:
        data = doc.to_dict()
        job_match_analysis = data.get("JobMatchAnalysis")
        if not job_match_analysis:
            print(f"Skipping {doc.id}: No 'JobMatchAnalysis' field present.")
            skipped += 1
            continue
        rating = job_match_analysis.get("Rating")
        if not rating or "NumericRating" not in rating:
            print(f"Skipping {doc.id}: No 'Rating.NumericRating' in 'JobMatchAnalysis'.")
            skipped += 1
            continue
        numeric_rating = rating["NumericRating"]
        current_top_rating = data.get("Rating")
        if current_top_rating == numeric_rating:
            unchanged += 1
            continue
        # Update the top-level Rating field
        user_jobs_ref.document(doc.id).update({"Rating": numeric_rating})
        print(f"Updated {doc.id}: Set top-level Rating to {numeric_rating} (was {current_top_rating})")
        updates += 1

    print(f"\nDone. Updated {updates} documents. {unchanged} unchanged. {skipped} skipped.")

if __name__ == "__main__":
    main()
