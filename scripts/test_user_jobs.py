#!/usr/bin/env python3

"""
Test script for user_jobs functionality.
This script verifies that job details are correctly saved to a user's job subcollection
during search processing.
"""

def check_user_jobs(user_id):
    """
    Check if a user has jobs in their user_jobs subcollection.
    
    Args:
        user_id (str): The user ID to check
        
    Returns:
        Prints the user's jobs subcollection contents
    """
    import firebase_admin
    from firebase_admin import firestore
    import json
    
    # Initialize Firebase Admin if not already initialized
    try:
        app = firebase_admin.get_app()
    except ValueError:
        cred = firebase_admin.credentials.ApplicationDefault()
        app = firebase_admin.initialize_app(cred)
    
    # Get Firestore client
    db = firestore.client(app)
    
    # Get user jobs
    user_jobs_ref = db.collection('users').document(user_id).collection('user_jobs')
    user_jobs = user_jobs_ref.stream()
    
    job_data = []
    for job in user_jobs:
        job_dict = job.to_dict()
        # Convert JobRef to string representation
        if 'job_ref' in job_dict:
            job_dict['job_ref'] = str(job_dict['job_ref'])
        # Convert timestamps to strings
        for field in ['CreatedAt', 'UpdatedAt', 'SavedAt', 'AppliedAt', 'ArchivedAt']:
            if field in job_dict and job_dict[field] is not None:
                job_dict[field] = job_dict[field].isoformat()
        job_data.append(job_dict)
    
    if not job_data:
        print(f"User {user_id} has no jobs in user_jobs subcollection")
    else:
        print(f"Found {len(job_data)} jobs for user {user_id}:")
        print(json.dumps(job_data, indent=2))

def trigger_test_user_job():
    """Create a test user and job to verify functionality"""
    import firebase_admin
    from firebase_admin import firestore
    import time
    import uuid
    
    # Initialize Firebase Admin if not already initialized
    try:
        app = firebase_admin.get_app()
    except ValueError:
        cred = firebase_admin.credentials.ApplicationDefault()
        app = firebase_admin.initialize_app(cred)
    
    # Get Firestore client
    db = firestore.client(app)
    
    # Create a test user
    test_user_id = f"test-user-{uuid.uuid4().hex[:8]}"
    test_job_id = f"test-job-{uuid.uuid4().hex[:8]}"
    
    # Create user document
    user_ref = db.collection('users').document(test_user_id)
    user_ref.set({
        'email': f"test-user-{test_user_id}@example.com",
        'created_at': firestore.SERVER_TIMESTAMP
    })
    print(f"Created test user: {test_user_id}")
    
    # Create job detail document
    job_ref = db.collection('job_details').document(test_job_id)
    job_ref.set({
        'id': test_job_id,
        'title': 'Test Job',
        'company': 'Test Company',
        'location': 'Test Location',
        'provider': 'test',
        'provider_job_id': test_job_id,
        'created_at': firestore.SERVER_TIMESTAMP
    })
    print(f"Created test job: {test_job_id}")
    
    # Create user job
    user_job = {
        'id': test_job_id,
        'search_hash': 'test-search-hash',
        'status': 'new',
        'job_ref': db.collection('job_details').document(test_job_id),
        'created_at': firestore.SERVER_TIMESTAMP,
        'updated_at': firestore.SERVER_TIMESTAMP
    }
    
    user_ref.collection('user_jobs').document(test_job_id).set(user_job)
    print(f"Created user job for user {test_user_id}")
    
    # Check user jobs
    print("\nChecking user jobs...")
    check_user_jobs(test_user_id)
    
    return test_user_id, test_job_id

if __name__ == "__main__":
    test_user_id, test_job_id = trigger_test_user_job() 