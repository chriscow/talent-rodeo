#!/usr/bin/env python3

import os
import sys
import json
from dotenv import load_dotenv
from google.cloud import firestore
from firebase_admin import credentials, initialize_app
from google.oauth2 import service_account
import datetime

def json_serializer(obj):
    if isinstance(obj, (datetime.datetime, datetime.date)):
        return obj.isoformat()
    return str(obj)

def main():
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <user_id>")
        sys.exit(1)
    user_id = sys.argv[1]

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

    appointments_ref = db.collection('users').document(user_id).collection('appointments')
    docs = appointments_ref.stream()

    appointments = []
    for doc in docs:
        data = doc.to_dict()
        data['id'] = doc.id
        appointments.append(data)

    print(json.dumps(appointments, indent=2, default=json_serializer))

if __name__ == "__main__":
    main() 