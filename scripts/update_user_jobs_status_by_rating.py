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

# Rating to status mapping
RATING_TO_STATUS = {
    0: "unprocessed",
    1: "archived",
    2: "archived",
    3: "new",
    4: "new",
    5: "new",
}

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
    for doc in docs:
        data = doc.to_dict()
        rating = data.get("Rating")
        if rating is None:
            print(f"Skipping {doc.id}: No 'Rating' field present.")
            continue
        new_status = RATING_TO_STATUS.get(rating)
        if not new_status:
            print(f"Skipping {doc.id}: Unexpected rating value {rating}")
            continue
        current_status = data.get("Status")
        if current_status == new_status:
            unchanged += 1
            continue
        # Update the status field
        user_jobs_ref.document(doc.id).update({"Status": new_status})
        print(f"Updated {doc.id}: Rating {rating} -> Status '{new_status}' (was '{current_status}')")
        updates += 1

    print(f"\nDone. Updated {updates} documents. {unchanged} unchanged.")

if __name__ == "__main__":
    main() 