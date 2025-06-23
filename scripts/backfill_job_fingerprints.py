#!/usr/bin/env python3

import os
import sys
import hashlib
import json
import base64
from collections import defaultdict
from dotenv import load_dotenv
from google.cloud import firestore
from google.api_core.exceptions import Aborted
from firebase_admin import credentials, initialize_app, _apps
from google.oauth2 import service_account
from datetime import datetime

JOB_DETAILS_COLLECTION = "job_details"
FINGERPRINTS_COLLECTION = "job_detail_fingerprints"

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

def hash_description(description):
    if not description:
        return None
    normalized = ' '.join(description.strip().lower().split())
    return hashlib.sha256(normalized.encode('utf-8')).hexdigest()

def backfill_job_fingerprints(db, dry_run=True):
    job_details_ref = db.collection(JOB_DETAILS_COLLECTION)
    
    print("Fetching all job details...")
    docs = list(job_details_ref.stream())
    print(f"Found {len(docs)} job detail documents to process.")
    
    # In-memory map to build fingerprints before writing to Firestore
    hash_to_fingerprint = defaultdict(lambda: {"canonical_id": None, "provider_ids": set(), "jobs": []})
    
    for doc in docs:
        data = doc.to_dict()
        desc_hash = hash_description(data.get("Description"))
        
        if not desc_hash:
            continue
            
        job_info = {
            "id": doc.id,
            "provider_job_id": data.get("ProviderJobID"),
            "created_at": data.get("CreatedAt")
        }
        
        # Group jobs by hash
        hash_to_fingerprint[desc_hash]["jobs"].append(job_info)
        hash_to_fingerprint[desc_hash]["provider_ids"].add(job_info["provider_job_id"])

    print(f"Found {len(hash_to_fingerprint)} unique job descriptions.")
    
    fingerprints_ref = db.collection(FINGERPRINTS_COLLECTION)
    
    updated_jobs = 0
    created_fingerprints = 0
    
    for desc_hash, data in hash_to_fingerprint.items():
        # Determine the canonical job (oldest one)
        sorted_jobs = sorted(data["jobs"], key=lambda x: x["created_at"] or datetime.max)
        canonical_job = sorted_jobs[0]
        canonical_id = canonical_job["id"]
        
        print(f"\nProcessing hash {desc_hash[:10]}... ({len(sorted_jobs)} jobs)")
        print(f"  Canonical Job ID: {canonical_id}")
        
        # --- 1. Create/Update Fingerprint ---
        fingerprint_payload = {
            "CanonicalJobID": canonical_id,
            "ProviderJobIDs": list(data["provider_ids"]),
            "UpdatedAt": firestore.SERVER_TIMESTAMP
        }

        if not dry_run:
            fingerprint_doc_ref = fingerprints_ref.document(desc_hash)
            # Use set with merge to create or update
            fingerprint_doc_ref.set(fingerprint_payload, merge=True)
        
        created_fingerprints += 1
        
        # --- 2. Update all associated Job Details ---
        for job in sorted_jobs:
            job_ref = job_details_ref.document(job["id"])
            is_canonical = job["id"] == canonical_id
            
            job_update_payload = {
                "DescriptionHash": desc_hash,
                "CanonicalJobID": None if is_canonical else canonical_id
            }
            
            if dry_run:
                print(f"  DRY RUN: Would update job {job['id']} with CanonicalJobID: {job_update_payload['CanonicalJobID']}")
            else:
                try:
                    job_ref.update(job_update_payload)
                    print(f"  Updated job {job['id']}")
                except Aborted:
                    print(f"  Retrying update for job {job['id']}...")
                    job_ref.update(job_update_payload) # simple retry
            
            updated_jobs += 1
            
    print("\n--- Migration Summary ---")
    print(f"Total jobs processed: {len(docs)}")
    print(f"Unique descriptions (fingerprints created/updated): {created_fingerprints}")
    print(f"Job documents updated: {updated_jobs}")
    if dry_run:
        print("\nDRY RUN COMPLETE. No actual changes were made.")
        print("Run with --apply to execute the migration.")

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
        print("\n!!! APPLYING MIGRATION - THIS WILL MODIFY PRODUCTION DATA !!!")
        confirm = input("Are you sure you want to proceed? (yes/no): ")
        if confirm.lower() != "yes":
            print("Migration cancelled.")
            return
    else:
        print("\n--- DRY RUN MODE ---")
    
    backfill_job_fingerprints(db, dry_run)

if __name__ == "__main__":
    main() 