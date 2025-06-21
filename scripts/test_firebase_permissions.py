#!/usr/bin/env python3

import os
import sys
import json
from dotenv import load_dotenv
from google.cloud import firestore
from firebase_admin import credentials, initialize_app
from google.oauth2 import service_account
import traceback

def main():
    print("🔥 Firebase Permission Test - Focused on 403 Errors\n")
    
    # Check for environment file argument
    env_file = ".env"  # default
    if len(sys.argv) > 1:
        env_file = sys.argv[1]
    
    print(f"Using environment file: {env_file}")
    load_dotenv(env_file)
    
    project_id = os.getenv("FIREBASE_PROJECT_ID")
    creds_file = os.getenv("FIREBASE_CREDENTIALS_FILE")
    
    if not project_id or not creds_file:
        print("❌ Missing environment variables. Check your .env file.")
        return False
    
    print(f"Project ID: {project_id}")
    print(f"Credentials File: {creds_file}")
    
    # Check credentials file content
    try:
        with open(creds_file, 'r') as f:
            creds_data = json.load(f)
        
        print(f"Service Account Email: {creds_data['client_email']}")
        print(f"Project ID in creds: {creds_data['project_id']}")
        
        # Check if project IDs match
        if creds_data['project_id'] != project_id:
            print("⚠️  WARNING: Project ID in credentials doesn't match FIREBASE_PROJECT_ID!")
            print(f"   Env var: {project_id}")
            print(f"   Creds file: {creds_data['project_id']}")
        
    except Exception as e:
        print(f"❌ Error reading credentials: {e}")
        return False
    
    # Test Firestore client with detailed error reporting
    try:
        print("\n🔍 Testing Firestore Client...")
        firestore_creds = service_account.Credentials.from_service_account_file(creds_file)
        db = firestore.Client(project=project_id, credentials=firestore_creds)
        print("✅ Firestore client created successfully")
        
        # Test basic read operation on users collection
        print("\n🔍 Testing Users Collection Read...")
        users_ref = db.collection("users")
        
        # Try to count documents first (minimal permission)
        try:
            print("   Attempting to list users collection...")
            docs = list(users_ref.limit(1).stream())
            print(f"✅ Successfully read {len(docs)} user(s)")
            
            if docs:
                print(f"   Sample user ID: {docs[0].id}")
        
        except Exception as read_error:
            print(f"❌ FAILED to read users collection: {read_error}")
            print(f"   Error type: {type(read_error).__name__}")
            
            # Check if it's specifically a 403 error
            if "403" in str(read_error) or "PermissionDenied" in str(read_error):
                print("\n🚨 PERMISSION DENIED ERROR DETECTED!")
                print("   This indicates your service account lacks proper permissions.")
                print("\n💡 Possible solutions:")
                print("   1. Grant 'Cloud Datastore User' role to the service account")
                print("   2. Grant 'Firebase Admin SDK Administrator Service Agent' role")
                print("   3. Grant 'Editor' or 'Owner' role (broader permissions)")
                print("   4. Check Firestore Security Rules (if using)")
                print("\n🔧 To fix permissions in Firebase Console:")
                print("   1. Go to Firebase Console > Project Settings > Service Accounts")
                print("   2. Or go to Google Cloud Console > IAM & Admin > IAM")
                print(f"   3. Find service account: {creds_data['client_email']}")
                print("   4. Add the 'Cloud Datastore User' role")
            
            return False
        
        # Test write operation
        print("\n🔍 Testing Write Permissions...")
        test_collection = "permission_test"
        test_doc_id = "test_doc"
        
        try:
            test_data = {"test": True, "timestamp": firestore.SERVER_TIMESTAMP}
            db.collection(test_collection).document(test_doc_id).set(test_data)
            print("✅ Write permission working")
            
            # Clean up
            db.collection(test_collection).document(test_doc_id).delete()
            print("✅ Delete permission working")
            
        except Exception as write_error:
            print(f"❌ Write operation failed: {write_error}")
            if "403" in str(write_error):
                print("   Write permissions are insufficient")
        
    except Exception as e:
        print(f"❌ Firestore client error: {e}")
        print(f"   Error type: {type(e).__name__}")
        print(f"   Full traceback:\n{traceback.format_exc()}")
        return False
    
    print("\n✅ Test completed successfully!")
    return True

if __name__ == "__main__":
    success = main()
    sys.exit(0 if success else 1) 