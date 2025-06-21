#!/usr/bin/env python3

import os
import sys
import json
from dotenv import load_dotenv
from google.cloud import firestore
from firebase_admin import credentials, initialize_app
from google.oauth2 import service_account
from google.auth import default
import traceback

def test_environment_setup(env_file=".env"):
    """Test that environment variables are properly set"""
    print("🔍 Testing Environment Setup...")
    print(f"Using environment file: {env_file}")
    
    load_dotenv(env_file)
    
    project_id = os.getenv("FIREBASE_PROJECT_ID")
    creds_file = os.getenv("FIREBASE_CREDENTIALS_FILE")
    
    if not project_id:
        print("❌ FIREBASE_PROJECT_ID not set in .env file")
        return False
    else:
        print(f"✅ FIREBASE_PROJECT_ID: {project_id}")
    
    if not creds_file:
        print("❌ FIREBASE_CREDENTIALS_FILE not set in .env file")
        return False
    else:
        print(f"✅ FIREBASE_CREDENTIALS_FILE: {creds_file}")
    
    if not os.path.isfile(creds_file):
        print(f"❌ Credentials file does not exist: {creds_file}")
        return False
    else:
        print(f"✅ Credentials file exists: {creds_file}")
    
    return True, project_id, creds_file

def test_credentials_file(creds_file):
    """Test that the credentials file is valid JSON and has required fields"""
    print("\n🔍 Testing Credentials File...")
    
    try:
        with open(creds_file, 'r') as f:
            creds_data = json.load(f)
        
        required_fields = ['type', 'project_id', 'private_key_id', 'private_key', 'client_email']
        missing_fields = [field for field in required_fields if field not in creds_data]
        
        if missing_fields:
            print(f"❌ Missing required fields in credentials: {missing_fields}")
            return False, None
        
        print("✅ Credentials file is valid JSON with required fields")
        print(f"✅ Service Account Email: {creds_data['client_email']}")
        print(f"✅ Project ID in credentials: {creds_data['project_id']}")
        
        return True, creds_data
        
    except json.JSONDecodeError as e:
        print(f"❌ Invalid JSON in credentials file: {e}")
        return False, None
    except Exception as e:
        print(f"❌ Error reading credentials file: {e}")
        return False, None

def test_firebase_admin_setup(creds_file, project_id):
    """Test Firebase Admin SDK initialization"""
    print("\n🔍 Testing Firebase Admin SDK Setup...")
    
    try:
        cred = credentials.Certificate(creds_file)
        app = initialize_app(cred, {'projectId': project_id})
        print("✅ Firebase Admin SDK initialized successfully")
        return True, app
    except Exception as e:
        print(f"❌ Firebase Admin SDK initialization failed: {e}")
        print(f"   Error type: {type(e).__name__}")
        return False, None

def test_firestore_client_setup(creds_file, project_id):
    """Test Firestore client initialization"""
    print("\n🔍 Testing Firestore Client Setup...")
    
    try:
        firestore_creds = service_account.Credentials.from_service_account_file(creds_file)
        db = firestore.Client(project=project_id, credentials=firestore_creds)
        print("✅ Firestore client initialized successfully")
        return True, db
    except Exception as e:
        print(f"❌ Firestore client initialization failed: {e}")
        print(f"   Error type: {type(e).__name__}")
        return False, None

def test_basic_firestore_operations(db):
    """Test basic Firestore operations to check permissions"""
    print("\n🔍 Testing Basic Firestore Operations...")
    
    test_collection = "connectivity_test"
    test_doc_id = "test_document"
    
    try:
        # Test: List collections (requires minimal permissions)
        print("   Testing collection listing...")
        collections = list(db.collections())
        print(f"✅ Can list collections. Found {len(collections)} collections.")
        
        # Test: Create a test document
        print("   Testing document creation...")
        test_data = {"test": True, "timestamp": firestore.SERVER_TIMESTAMP}
        db.collection(test_collection).document(test_doc_id).set(test_data)
        print("✅ Can create documents")
        
        # Test: Read the test document
        print("   Testing document reading...")
        doc = db.collection(test_collection).document(test_doc_id).get()
        if doc.exists:
            print("✅ Can read documents")
        else:
            print("❌ Document was not created or cannot be read")
            return False
        
        # Test: Update the test document
        print("   Testing document updates...")
        db.collection(test_collection).document(test_doc_id).update({"updated": True})
        print("✅ Can update documents")
        
        # Test: Delete the test document
        print("   Testing document deletion...")
        db.collection(test_collection).document(test_doc_id).delete()
        print("✅ Can delete documents")
        
        return True
        
    except Exception as e:
        print(f"❌ Firestore operation failed: {e}")
        print(f"   Error type: {type(e).__name__}")
        print(f"   Full error: {traceback.format_exc()}")
        return False

def test_users_collection_access(db):
    """Test access to the users collection specifically"""
    print("\n🔍 Testing Users Collection Access...")
    
    try:
        # Test: List users collection
        print("   Testing users collection listing...")
        users_ref = db.collection("users")
        
        # Try to get first few documents
        docs = list(users_ref.limit(1).stream())
        print(f"✅ Can access users collection. Found {len(docs)} users (limited to 1 for testing)")
        
        if len(docs) > 0:
            user_doc = docs[0]
            print(f"✅ Sample user ID: {user_doc.id}")
            
            # Test reading the user document
            user_data = user_doc.to_dict()
            print("✅ Can read user document data")
            
            # Check if it has the old State structure
            if "State" in user_data:
                print(f"✅ Found user with old State structure: {user_doc.id}")
            else:
                print(f"ℹ️  User {user_doc.id} already has new structure (no State field)")
        
        return True
        
    except Exception as e:
        print(f"❌ Users collection access failed: {e}")
        print(f"   Error type: {type(e).__name__}")
        print(f"   Full error: {traceback.format_exc()}")
        return False

def test_service_account_permissions(creds_file):
    """Test what permissions the service account has"""
    print("\n🔍 Testing Service Account Permissions...")
    
    try:
        with open(creds_file, 'r') as f:
            creds_data = json.load(f)
        
        service_account_email = creds_data['client_email']
        print(f"Service Account: {service_account_email}")
        
        # Check if it's a Firebase service account
        if "firebase-adminsdk" in service_account_email:
            print("✅ This appears to be a Firebase Admin SDK service account")
        else:
            print("⚠️  This doesn't appear to be a Firebase Admin SDK service account")
            print("   Make sure you're using the correct service account for Firebase")
        
        return True
        
    except Exception as e:
        print(f"❌ Error checking service account: {e}")
        return False

def main():
    print("🔥 Firebase/Firestore Connectivity Test\n")
    
    # Check for environment file argument
    env_file = ".env"  # default
    if len(sys.argv) > 1:
        env_file = sys.argv[1]
    
    # Test 1: Environment setup
    env_result = test_environment_setup(env_file)
    if not env_result:
        print("\n❌ Environment setup failed. Please fix the issues above.")
        return False
    
    env_ok, project_id, creds_file = env_result
    
    # Test 2: Credentials file validation
    creds_result = test_credentials_file(creds_file)
    if not creds_result:
        print("\n❌ Credentials file validation failed.")
        return False
    
    creds_ok, creds_data = creds_result
    
    # Test 3: Service account permissions check
    test_service_account_permissions(creds_file)
    
    # Test 4: Firebase Admin SDK
    admin_result = test_firebase_admin_setup(creds_file, project_id)
    if not admin_result[0]:
        print("\n❌ Firebase Admin SDK setup failed.")
        return False
    
    # Test 5: Firestore client
    client_result = test_firestore_client_setup(creds_file, project_id)
    if not client_result[0]:
        print("\n❌ Firestore client setup failed.")
        return False
    
    db = client_result[1]
    
    # Test 6: Basic Firestore operations
    if not test_basic_firestore_operations(db):
        print("\n❌ Basic Firestore operations failed.")
        return False
    
    # Test 7: Users collection access
    if not test_users_collection_access(db):
        print("\n❌ Users collection access failed.")
        return False
    
    print("\n🎉 All tests passed! Your Firebase/Firestore setup appears to be working correctly.")
    print("\nIf the migration script is still failing, the issue might be:")
    print("1. Firestore security rules blocking access in production")
    print("2. Different service account being used in production")
    print("3. Network connectivity issues in production environment")
    
    return True

if __name__ == "__main__":
    try:
        success = main()
        sys.exit(0 if success else 1)
    except KeyboardInterrupt:
        print("\n\n⚠️  Test interrupted by user")
        sys.exit(1)
    except Exception as e:
        print(f"\n❌ Unexpected error: {e}")
        print(f"Full traceback: {traceback.format_exc()}")
        sys.exit(1) 