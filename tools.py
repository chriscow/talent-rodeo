#!/usr/bin/env python3
"""
Talent Rodeo project management tools.

This module provides utilities for managing feature dependencies and
implementation order for the Talent Rodeo project.
"""

import os
import sys
import time
import json
import sqlite3
import argparse
import subprocess
import signal
import requests
from datetime import datetime, timedelta
from pathlib import Path
from tabulate import tabulate
import re
import glob
from collections import defaultdict, deque

# Add the current directory to the path for explicit relative imports
sys.path.append(os.path.dirname(os.path.abspath(__file__)))

# Import functions from organized modules
from tools.ui_tests import (
    run_ui_test,
    wait_for_server,
    linkedin_login,
    print_test_summary,
    needs_authentication,
    wait_for_manual_auth,
    test_homepage,
    test_auth_flow,
    test_profile,
    test_resume_upload,
    test_scheduling,
    test_phone_call
)

from tools.go_tests import (
    run_go_tests,
    test_tier_system,
    analyze_tier_consumer_issues,
    setup_test_candidate
)

from tools.db_utils import (
    query_db,
    store_memory,
    get_memories
)

from tools.feature_analysis import (
    analyze_feature_dependencies
)

from tools.feature_analyzer import (
    analyze_feature_dependencies as analyze_feature_deps
)

# Import third-party dependencies
try:
    from dotenv import load_dotenv
    # Load environment variables
    load_dotenv()
except ImportError:
    print("python-dotenv not installed. Install with: pip install python-dotenv")

try:
    from playwright.sync_api import sync_playwright
except ImportError:
    print("Playwright not installed. Install with: pip install playwright")
    print("Then run: playwright install")

# Main Functions with descriptions
# --------------------------------------------------------------------------------

def ui_test(test_name=None):
    """
    Run UI tests against the Talent Rodeo web application.
    
    This function starts a test server instance with a fresh test database,
    then runs automated UI tests using Playwright.
    
    Args:
        test_name (str, optional): Specific test to run. If None, runs all tests.
            Valid values: "homepage", "auth_flow", "profile", "resume_upload", 
                          "scheduling", "phone_call"
    
    Returns:
        bool: True if tests passed, False otherwise
    """
    return run_ui_test(test_name)


def go_test(package_path=None, with_race=True, with_coverage=False):
    """
    Run Go tests for the Talent Rodeo backend.
    
    Args:
        package_path (str, optional): Specific package to test. Tests all packages if None.
        with_race (bool): Enable race detection. Defaults to True.
        with_coverage (bool): Enable coverage reporting. Defaults to False.
    
    Returns:
        bool: True if tests passed, False otherwise
    """
    return run_go_tests(package_path, with_race, with_coverage)


def tier_test(package_path="./internal/tasks/handlers/...", tier_test=None, with_isolation=True):
    """
    Run tier system search scheduler tests with proper isolation.
    
    Args:
        package_path (str): Path to the package containing tier tests
        tier_test (str, optional): Specific tier test to run (e.g. "TestSearchScheduler_TierLimits")
        with_isolation (bool): Whether to run tests with isolation to prevent consumer overlap
    
    Returns:
        bool: True if tests passed, False otherwise
    """
    return test_tier_system(package_path, tier_test, with_isolation)


def analyze_tier_issues(test_output):
    """
    Analyze tier system test output for common issues.
    
    Args:
        test_output (str): The output from a failed tier system test
        
    Returns:
        dict: Analysis results with issue type and suggested fixes
    """
    return analyze_tier_consumer_issues(test_output)


def test_setup_candidate(tier='free', last_page=None, last_search_at=None, subscription_end=None):
    """
    Generate Go code to set up a test candidate with specific tier settings.
    
    Args:
        tier (str): The tier to set ('free', 'mid', or 'top')
        last_page (int, optional): The last page number processed
        last_search_at (str, optional): ISO time string for last search time
        subscription_end (str, optional): ISO time string for subscription end date
        
    Returns:
        str: Go code snippet for setting up a test candidate
    """
    return setup_test_candidate(tier, last_page, last_search_at, subscription_end)


def sql_query(query, params=None, db_path="db.sqlite", format="table"):
    """
    Execute a SQL query against the SQLite database and display results.
    
    Args:
        query (str): SQL query to execute
        params (tuple, optional): Query parameters if using parameterized queries
        db_path (str): Path to the SQLite database file. Defaults to "db.sqlite"
        format (str): Output format - "table", "json", or "csv". Defaults to "table"
    
    Returns:
        bool: True if query executed successfully, False otherwise
    """
    return query_db(query, params, db_path, format)


def save_memory(category, content, tags=None):
    """
    Store a memory with metadata for future reference.
    
    Args:
        category (str): Category of the memory (e.g., 'code_insight', 'progress', 'lesson_learned')
        content (str): The actual memory content
        tags (list, optional): List of tags to help with categorization and search
    
    Returns:
        bool: True if memory was stored successfully, False otherwise
    """
    return store_memory(category, content, tags)


def recall_memories(category=None, tags=None, since=None, format="table"):
    """
    Retrieve memories with optional filtering.
    
    Args:
        category (str, optional): Filter by category
        tags (list, optional): Filter by tags (matches if any tag matches)
        since (str, optional): ISO format date string to filter by date (e.g., '2024-02-04')
        format (str): Output format - "table", "json", or "text". Defaults to "table"
    
    Returns:
        bool: True if query executed successfully, False otherwise
    """
    return get_memories(category, tags, since, format)


def feature_deps(project_dir=".project"):
    """
    Analyze feature dependencies and suggest implementation order.
    
    This function scans feature specification files, extracts dependency information,
    and outputs a recommended implementation order based on dependency analysis.
    
    Args:
        project_dir (str): Path to the project directory containing feature specs
        
    Returns:
        dict: Dependency information and implementation order recommendations
    """
    return analyze_feature_dependencies(project_dir)


def feature_analyzer(project_dir=".project"):
    """
    Analyze feature dependencies using the feature_analyzer module.
    
    This is an alternative implementation that provides the same functionality
    as feature_deps but is maintained separately.
    
    Args:
        project_dir (str): Path to the project directory containing feature specs
        
    Returns:
        dict: Dependency information and implementation order recommendations
    """
    return analyze_feature_deps(project_dir)


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


def trigger_search_for_user(user_id, keywords, location="United States"):
    """
    Manually trigger a job search for a specific user.
    
    Args:
        user_id (str): The user ID to search for
        keywords (list): List of keywords to search for
        location (str): Location to search in
        
    Returns:
        Prints the status of the search trigger
    """
    import firebase_admin
    from firebase_admin import firestore
    import time
    
    # Initialize Firebase Admin if not already initialized
    try:
        app = firebase_admin.get_app()
    except ValueError:
        cred = firebase_admin.credentials.ApplicationDefault()
        app = firebase_admin.initialize_app(cred)
    
    # Get Firestore client
    db = firestore.client(app)
    
    # Get user document
    user_ref = db.collection('users').document(user_id)
    user_doc = user_ref.get()
    
    if not user_doc.exists:
        print(f"User {user_id} does not exist")
        return
    
    # Update user's search criteria
    if isinstance(keywords, str):
        keywords = [keywords]
        
    search_data = {
        'search': {
            'criteria': {
                'keywords': keywords,
                'location': location
            },
            'status': 'pending',
            'queued_at': firestore.SERVER_TIMESTAMP
        }
    }
    
    user_ref.update(search_data)
    print(f"Updated search criteria for user {user_id}")
    print(f"Keywords: {keywords}")
    print(f"Location: {location}")
    print("Search status set to 'pending'")
    
    # Wait a bit and check search status
    time.sleep(5)
    user_doc = user_ref.get()
    search_data = user_doc.to_dict().get('search', {})
    print(f"Current search status: {search_data.get('status', 'unknown')}")
    
    # Check if there are jobs in the user's subcollection
    check_user_jobs(user_id)


# Main entry point
if __name__ == "__main__":
    # Parse command line arguments
    parser = argparse.ArgumentParser(description="Talent Rodeo project management tools")
    
    subparsers = parser.add_subparsers(dest="command", help="Command to run")
    
    # UI Test command
    ui_test_parser = subparsers.add_parser("ui-test", help="Run UI tests")
    ui_test_parser.add_argument("--test", help="Specific test to run")
    
    # Go Test command
    go_test_parser = subparsers.add_parser("go-test", help="Run Go tests")
    go_test_parser.add_argument("--package", help="Specific package to test")
    go_test_parser.add_argument("--no-race", action="store_true", help="Disable race detection")
    go_test_parser.add_argument("--coverage", action="store_true", help="Enable coverage reporting")
    
    # Tier Test command
    tier_test_parser = subparsers.add_parser("tier-test", help="Run tier system tests")
    tier_test_parser.add_argument("--test", help="Specific tier test to run")
    tier_test_parser.add_argument("--package", default="./internal/tasks/handlers/...", help="Package path")
    tier_test_parser.add_argument("--no-isolation", action="store_true", help="Disable test isolation")
    
    # Query DB command
    query_parser = subparsers.add_parser("query", help="Query SQLite database")
    query_parser.add_argument("--query", required=True, help="SQL query to execute")
    query_parser.add_argument("--db", default="db.sqlite", help="Database file path")
    query_parser.add_argument("--format", choices=["table", "json", "csv"], default="table", help="Output format")
    
    # Memory commands
    memory_parser = subparsers.add_parser("memory", help="Manage project memories")
    memory_subparsers = memory_parser.add_subparsers(dest="memory_command", help="Memory command")
    
    # Store memory
    store_parser = memory_subparsers.add_parser("store", help="Store a new memory")
    store_parser.add_argument("--category", required=True, help="Memory category")
    store_parser.add_argument("--content", required=True, help="Memory content")
    store_parser.add_argument("--tags", nargs="+", help="Memory tags")
    
    # Get memories
    get_parser = memory_subparsers.add_parser("get", help="Retrieve memories")
    get_parser.add_argument("--category", help="Filter by category")
    get_parser.add_argument("--tags", nargs="+", help="Filter by tags")
    get_parser.add_argument("--since", help="Filter by date (ISO format)")
    get_parser.add_argument("--format", choices=["table", "json", "text"], default="table", help="Output format")
    
    # Feature analysis commands
    feature_parser = subparsers.add_parser("features", help="Analyze feature dependencies")
    feature_parser.add_argument("--project-dir", default=".project", help="Project directory path")
    feature_parser.add_argument("--use-analyzer", action="store_true", help="Use feature_analyzer instead of feature_analysis")
    
    args = parser.parse_args()
    
    # Execute the appropriate command
    if args.command == "ui-test":
        ui_test(args.test)
    elif args.command == "go-test":
        go_test(args.package, not args.no_race, args.coverage)
    elif args.command == "tier-test":
        tier_test(args.package, args.test, not args.no_isolation)
    elif args.command == "query":
        sql_query(args.query, None, args.db, args.format)
    elif args.command == "memory":
        if args.memory_command == "store":
            save_memory(args.category, args.content, args.tags)
        elif args.memory_command == "get":
            recall_memories(args.category, args.tags, args.since, args.format)
    elif args.command == "features":
        if args.use_analyzer:
            feature_analyzer(args.project_dir)
        else:
            feature_deps(args.project_dir)
    else:
        # Default action if no command specified
        parser.print_help()
        print("\nAvailable functions for import:")
        print("  - ui_test: Run UI tests")
        print("  - go_test: Run Go tests")
        print("  - tier_test: Run tier system tests")
        print("  - analyze_tier_issues: Analyze tier system test failures")
        print("  - test_setup_candidate: Generate Go code for test candidates")
        print("  - sql_query: Execute SQL queries against SQLite database")
        print("  - save_memory: Store project memories")
        print("  - recall_memories: Retrieve project memories")
        print("  - feature_deps: Analyze feature dependencies")
        print("  - feature_analyzer: Alternative feature dependency analysis") 