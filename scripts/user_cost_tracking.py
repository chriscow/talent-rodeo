#!/usr/bin/env python3

import os
import sys
import json
import base64
from datetime import datetime, timedelta
from dotenv import load_dotenv
from google.cloud import firestore
from firebase_admin import credentials, initialize_app, _apps
from google.oauth2 import service_account

COST_TRANSACTIONS_COLLECTION = "cost_transactions"


def setup_firebase(env_file=".env"):
    """Setup Firebase/Firestore using the same pattern as cleanup_duplicate_user_jobs.py"""
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

def analyze_cost_transactions(db, days_back=7):
    """Analyze cost transactions from the last N days"""
    print(f"--- Analyzing Cost Transactions (Last {days_back} Days) ---")
    
    # Calculate date range
    end_time = datetime.now()
    start_time = end_time - timedelta(days=days_back)
    
    print(f"Date range: {start_time.strftime('%Y-%m-%d %H:%M')} to {end_time.strftime('%Y-%m-%d %H:%M')}")
    
    # Query cost transactions
    cost_ref = db.collection(COST_TRANSACTIONS_COLLECTION)
    query = cost_ref.where("created_at", ">=", start_time).where("created_at", "<=", end_time)
    
    try:
        transactions = list(query.stream())
    except Exception as e:
        print(f"Error querying transactions: {e}")
        print("Note: This is expected if no cost transactions exist yet.")
        return
    
    if not transactions:
        print("No cost transactions found in the specified time range.")
        print("This is expected if the cost tracking hasn't been used yet.")
        return
    
    print(f"\nFound {len(transactions)} cost transactions")
    
    # Analyze transactions
    total_cost = 0.0
    by_user = {}
    by_type = {}
    by_model = {}
    by_feature = {}
    
    print("\n--- Recent Transactions ---")
    for i, doc in enumerate(transactions[:10]):  # Show first 10
        data = doc.to_dict()
        cost = data.get('cost_usd', 0.0)
        total_cost += cost
        
        user_id = data.get('user_id', 'Unknown')
        trans_type = data.get('type', 'Unknown')
        model = data.get('model')
        feature = data.get('feature_area', 'Unknown')
        
        # Aggregate stats
        by_user[user_id] = by_user.get(user_id, 0.0) + cost
        by_type[trans_type] = by_type.get(trans_type, 0.0) + cost
        if model:
            by_model[model] = by_model.get(model, 0.0) + cost
        by_feature[feature] = by_feature.get(feature, 0.0) + cost
        
        print(f"{i+1}. User: {user_id}")
        print(f"   Type: {trans_type}, Cost: ${cost:.6f}")
        print(f"   Description: {data.get('description', 'No description')}")
        if model:
            print(f"   Model: {model}")
            tokens_in = data.get('prompt_tokens', 0)
            tokens_out = data.get('completion_tokens', 0)
            print(f"   Tokens: {tokens_in} input + {tokens_out} output")
            if data.get('discount_applied'):
                print(f"   Discount: {data.get('discount_percent', 0)}% off")
        print(f"   Feature: {feature}")
        print(f"   Time: {data.get('created_at')}")
        print()
    
    # Summary stats
    print("--- Summary Statistics ---")
    print(f"Total Cost: ${total_cost:.6f}")
    
    if by_user:
        print(f"\nTop Users by Cost:")
        for user_id, cost in sorted(by_user.items(), key=lambda x: x[1], reverse=True)[:5]:
            print(f"  {user_id}: ${cost:.6f}")
    
    if by_type:
        print(f"\nCost by Type:")
        for trans_type, cost in sorted(by_type.items(), key=lambda x: x[1], reverse=True):
            print(f"  {trans_type}: ${cost:.6f}")
    
    if by_model:
        print(f"\nCost by Model:")
        for model, cost in sorted(by_model.items(), key=lambda x: x[1], reverse=True):
            print(f"  {model}: ${cost:.6f}")
    
    if by_feature:
        print(f"\nCost by Feature:")
        for feature, cost in sorted(by_feature.items(), key=lambda x: x[1], reverse=True):
            print(f"  {feature}: ${cost:.6f}")

def main():
    env_file = ".env"
    days_back = 7
    
    for arg in sys.argv[1:]:
        if arg.endswith(".env"):
            env_file = arg
        elif arg.startswith("--days="):
            days_back = int(arg.split("=")[1])
    
    print(f"Using environment file: {env_file}")
    
    try:
        db = setup_firebase(env_file)
        analyze_cost_transactions(db, days_back)
    except Exception as e:
        print(f"Error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main() 