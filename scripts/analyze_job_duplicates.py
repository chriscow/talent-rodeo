#!/usr/bin/env python3

import os
import sys
import hashlib
import json
import base64
from collections import defaultdict
from dotenv import load_dotenv
from google.cloud import firestore
from firebase_admin import credentials, initialize_app
from google.oauth2 import service_account

JOB_DETAILS_COLLECTION = "job_details"

def setup_firebase(env_file=".env"):
    load_dotenv(env_file)
    project_id = os.getenv("FIREBASE_PROJECT_ID")
    if not project_id:
        raise ValueError("FIREBASE_PROJECT_ID must be set in .env file")

    firebase_credentials = os.getenv("FIREBASE_CREDENTIALS")
    if not firebase_credentials:
        raise ValueError("FIREBASE_CREDENTIALS must be set in .env file (base64 encoded JSON)")

    try:
        # Decode base64 credentials
        credentials_json = base64.b64decode(firebase_credentials).decode('utf-8')
        credentials_dict = json.loads(credentials_json)
    except Exception as e:
        raise ValueError(f"Failed to decode FIREBASE_CREDENTIALS: {e}")

    # Initialize Firebase Admin
    cred = credentials.Certificate(credentials_dict)
    initialize_app(cred, {'projectId': project_id})
    
    # Create Firestore client
    firestore_creds = service_account.Credentials.from_service_account_info(credentials_dict)
    db = firestore.Client(project=project_id, credentials=firestore_creds)
    return db

def hash_description(description):
    """Create a SHA-256 hash of the job description for duplicate detection."""
    if not description:
        return None
    # Normalize whitespace and convert to lowercase for better duplicate detection
    normalized = ' '.join(description.strip().lower().split())
    return hashlib.sha256(normalized.encode('utf-8')).hexdigest()

def analyze_job_duplicates(db):
    """
    Analyze job details for duplicates based on hashed descriptions.
    
    Returns statistics about duplicates found.
    """
    job_details_ref = db.collection(JOB_DETAILS_COLLECTION)
    
    print("Fetching all job details...")
    docs = list(job_details_ref.stream())
    print(f"Found {len(docs)} job detail documents")
    
    # Track hashes and their associated job details
    hash_to_jobs = defaultdict(list)
    no_description = 0
    processed = 0
    
    for doc in docs:
        try:
            data = doc.to_dict()
            job_id = doc.id
            
            description = data.get("Description", "")
            if not description:
                no_description += 1
                continue
            
            # Hash the description
            desc_hash = hash_description(description)
            if desc_hash:
                job_info = {
                    'id': job_id,
                    'provider': data.get('Provider', 'unknown'),
                    'provider_job_id': data.get('ProviderJobID', 'unknown'),
                    'title': data.get('Title', 'untitled'),
                    'company': data.get('Company', 'unknown'),
                    'location': data.get('Location', 'unknown'),
                    'posted_date': data.get('PostedDate', 'unknown')
                }
                hash_to_jobs[desc_hash].append(job_info)
            
            processed += 1
            if processed % 100 == 0:
                print(f"Processed {processed} job details...")
                
        except Exception as e:
            print(f"Error processing job detail {doc.id}: {e}")
    
    # Analyze results
    total_jobs = processed
    unique_descriptions = len(hash_to_jobs)
    duplicate_groups = {h: jobs for h, jobs in hash_to_jobs.items() if len(jobs) > 1}
    duplicate_groups_count = len(duplicate_groups)
    total_duplicates = sum(len(jobs) - 1 for jobs in duplicate_groups.values())
    
    print(f"\n=== Duplicate Analysis Results ===")
    print(f"Total job details processed: {total_jobs}")
    print(f"Jobs without descriptions: {no_description}")
    print(f"Unique descriptions: {unique_descriptions}")
    print(f"Duplicate groups found: {duplicate_groups_count}")
    print(f"Total duplicate jobs: {total_duplicates}")
    
    if duplicate_groups_count > 0:
        duplicate_percentage = (total_duplicates / total_jobs) * 100
        print(f"Duplicate percentage: {duplicate_percentage:.2f}%")
        
        print(f"\n=== Top 10 Duplicate Groups ===")
        # Sort by number of duplicates (descending)
        sorted_groups = sorted(duplicate_groups.items(), 
                             key=lambda x: len(x[1]), reverse=True)
        
        for i, (desc_hash, jobs) in enumerate(sorted_groups[:10]):
            print(f"\nGroup {i+1}: {len(jobs)} identical jobs")
            print(f"  Hash: {desc_hash[:16]}...")
            
            # Show details for each job in this group
            for j, job in enumerate(jobs):
                print(f"    Job {j+1}:")
                print(f"      ID: {job['id']}")
                print(f"      Provider: {job['provider']} (ID: {job['provider_job_id']})")
                print(f"      Title: {job['title']}")
                print(f"      Company: {job['company']}")
                print(f"      Location: {job['location']}")
                print(f"      Posted: {job['posted_date']}")
        
        # Show distribution of duplicate group sizes
        print(f"\n=== Duplicate Group Size Distribution ===")
        size_distribution = defaultdict(int)
        for jobs in duplicate_groups.values():
            size_distribution[len(jobs)] += 1
        
        for size in sorted(size_distribution.keys()):
            count = size_distribution[size]
            print(f"  Groups with {size} identical jobs: {count}")
    
    return {
        'total_jobs': total_jobs,
        'no_description': no_description,
        'unique_descriptions': unique_descriptions,
        'duplicate_groups': duplicate_groups_count,
        'total_duplicates': total_duplicates,
        'duplicate_percentage': (total_duplicates / total_jobs) * 100 if total_jobs > 0 else 0
    }

def main():
    # Parse command line arguments
    env_file = ".env"  # default
    
    for arg in sys.argv[1:]:
        if arg.endswith(".env"):
            env_file = arg
        elif arg in ["-h", "--help"]:
            print("Usage: python analyze_job_duplicates.py [env_file]")
            print("  env_file: Path to environment file (default: .env)")
            return
    
    print(f"Using environment file: {env_file}")
    
    try:
        db = setup_firebase(env_file)
        results = analyze_job_duplicates(db)
        
        print(f"\n=== Summary ===")
        print(f"Analysis complete. Found {results['duplicate_groups']} duplicate groups")
        print(f"affecting {results['total_duplicates']} jobs ({results['duplicate_percentage']:.2f}% of total)")
        
    except Exception as e:
        print(f"Error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main() 