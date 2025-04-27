#!/usr/bin/env python3

import os
from dotenv import load_dotenv
from google.cloud import firestore
from firebase_admin import credentials, initialize_app
from google.oauth2 import service_account

def main():
    # Load environment variables from .env file
    load_dotenv()
    
    # Get Firebase project ID
    project_id = os.getenv("FIREBASE_PROJECT_ID")
    if not project_id:
        raise ValueError("FIREBASE_PROJECT_ID must be set in .env file")
    
    # Get path to credentials file
    creds_file = "/Users/chris/dev/talent-rodeo/.secrets/talent-rodeo-dev-firebase-adminsdk-fbsvc-847b5213c4.json"
    if not creds_file or not os.path.isfile(creds_file):
        raise ValueError("FIREBASE_CREDENTIALS_FILE must be set in .env and point to a valid file")
    
    # Initialize Firebase Admin
    cred = credentials.Certificate(creds_file)
    initialize_app(cred, {'projectId': project_id})
    
    # Create google.oauth2 credentials for Firestore
    firestore_creds = service_account.Credentials.from_service_account_file(creds_file)
    db = firestore.Client(project=project_id, credentials=firestore_creds)
    
    # Get all documents from geo_ids collection
    geo_ids_collection = db.collection('geo_ids')
    docs = geo_ids_collection.stream()
    
    # Store documents for processing
    documents = []
    for doc in docs:
        doc_data = doc.to_dict()
        if 'value' in doc_data:
            documents.append({
                'id': doc.id,
                'value': doc_data.get('value', '')
            })
        else:
            print(f"Warning: Document {doc.id} does not have a 'Value' field")
    
    # Process documents in batch
    batch = db.batch()
    deleted_count = 0
    created_count = 0
    
    print(f"Found {len(documents)} documents to process")
    
    # First pass: Create new documents with reversed ID/Value
    for doc in documents:
        old_id = doc['id']
        new_id = doc['value']
        new_value = old_id
        
        # Skip if new_id is empty
        if not new_id:
            print(f"Skipping document {old_id} because Value is empty")
            continue
            
        # Create new document with ID = original Value
        new_doc_ref = geo_ids_collection.document(new_id)
        batch.set(new_doc_ref, {'Value': new_value})
        created_count += 1
        
        # Delete the original document
        old_doc_ref = geo_ids_collection.document(old_id)
        batch.delete(old_doc_ref)
        deleted_count += 1
        
        # Commit batch when it reaches a certain size
        if deleted_count % 500 == 0:
            batch.commit()
            batch = db.batch()
            print(f"Processed {deleted_count} documents")
    
    # Commit any remaining operations
    if deleted_count % 500 != 0:
        batch.commit()
    
    print(f"Successfully reversed {created_count} documents")
    print(f"Deleted {deleted_count} original documents")

if __name__ == "__main__":
    main() 