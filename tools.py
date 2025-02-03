import subprocess
import time
import sys
from playwright.sync_api import sync_playwright
import signal
import os
import requests
from dotenv import load_dotenv
import sqlite3
from tabulate import tabulate
import json

# Load environment variables
load_dotenv()

def wait_for_server(url, timeout=30):
    """Wait for server to be ready"""
    start_time = time.time()
    while time.time() - start_time < timeout:
        try:
            response = requests.get(url, timeout=5)
            if response.status_code == 200:
                print(f"Server is ready at {url}")
                return True
        except (requests.exceptions.ConnectionError, 
                requests.exceptions.ChunkedEncodingError,
                requests.exceptions.ReadTimeout) as e:
            print(f"Waiting for server to start... ({str(e)})")
            time.sleep(1)
    return False

def linkedin_login(page):
    """Helper function to handle LinkedIn authentication"""
    username = os.getenv('LINKEDIN_USERNAME')
    password = os.getenv('LINKEDIN_PASSWORD')
    
    if not username or not password:
        raise ValueError("LinkedIn credentials not found in environment variables")
    
    # Wait for LinkedIn OAuth page
    page.wait_for_selector('input[name="session_key"]')
    page.fill('input[name="session_key"]', username)
    page.fill('input[name="session_password"]', password)
    page.click('button[type="submit"]')
    
    # Wait for OAuth consent
    page.wait_for_selector('button[data-oauth-submit="true"]', timeout=10000)
    page.click('button[data-oauth-submit="true"]')
    
    # Wait for redirect back to our app
    page.wait_for_url("**/profile*")

def run_ui_test(test_name=None):
    # Start the server in test mode
    server_process = subprocess.Popen(
        ["go", "run", "./cmd/web", "-http-port=2304"],
        env={**os.environ, "TEST_MODE": "true"},
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        preexec_fn=os.setsid  # Create new process group
    )

    try:
        # Wait for server to start
        if not wait_for_server("http://localhost:2304"):
            stdout, stderr = server_process.communicate()
            print("Failed to start server")
            print("Server stdout:", stdout)
            print("Server stderr:", stderr)
            os.killpg(os.getpgid(server_process.pid), signal.SIGTERM)
            return False

        # Initialize the browser
        with sync_playwright() as p:
            browser = p.chromium.launch(headless=False)
            context = browser.new_context()
            page = context.new_page()

            # Run all tests or a specific test
            tests = {
                "homepage": test_homepage,
                "auth_flow": test_auth_flow,
                "profile": test_profile,
                "resume_upload": test_resume_upload,
                "scheduling": test_scheduling,
                "phone_call": test_phone_call
            }

            if test_name:
                if test_name in tests:
                    print(f"\nRunning test: {test_name}")
                    try:
                        tests[test_name](page)
                        print(f"✓ {test_name} test passed")
                    except Exception as e:
                        print(f"✗ {test_name} test failed: {str(e)}")
                        # Print the current page content for debugging
                        print("\nPage content:")
                        print(page.content())
                else:
                    print(f"Test '{test_name}' not found")
            else:
                for name, test_func in tests.items():
                    print(f"\nRunning test: {name}")
                    try:
                        test_func(page)
                        print(f"✓ {name} test passed")
                    except Exception as e:
                        print(f"✗ {name} test failed: {str(e)}")
                        # Print the current page content for debugging
                        print("\nPage content:")
                        print(page.content())

            # Clean up
            context.close()
            browser.close()

    except Exception as e:
        print(f"Error during test execution: {e}")
        raise
    finally:
        print("\nShutting down server...")
        try:
            # Send SIGTERM to the entire process group
            os.killpg(os.getpgid(server_process.pid), signal.SIGTERM)
            
            # Wait for up to 5 seconds for graceful shutdown
            try:
                stdout, stderr = server_process.communicate(timeout=5)
                print("Server shutdown completed")
                if stdout:
                    print("\nServer stdout:", stdout)
                if stderr:
                    print("\nServer stderr:", stderr)
            except subprocess.TimeoutExpired:
                print("Server didn't shut down gracefully after 5s, sending SIGKILL")
                os.killpg(os.getpgid(server_process.pid), signal.SIGKILL)
                stdout, stderr = server_process.communicate()
                if stdout:
                    print("\nServer stdout:", stdout)
                if stderr:
                    print("\nServer stderr:", stderr)
        except ProcessLookupError:
            print("Server process already terminated")

    return True

def test_homepage(page):
    """Test the homepage functionality"""
    print("Starting homepage test...")
    page.goto("http://localhost:2304")
    print("Page loaded")
    
    # In test mode, we should be redirected to profile since we're logged in
    assert page.url.endswith("/profile")
    print("✓ Homepage test passed")

def test_auth_flow(page):
    """Test the authentication flow"""
    print("Starting auth flow test...")
    page.goto("http://localhost:2304")
    print("Page loaded")
    
    # In test mode, we should already be logged in and redirected to profile
    assert page.url.endswith("/profile")
    print("✓ Authentication flow test passed")

def test_profile(page):
    """Test the profile page functionality"""
    print("Loading profile page...")
    page.goto("http://localhost:2304/profile/resume/upload")
    print("Checking profile elements...")
    
    # Check for resume upload elements
    assert page.get_by_text("Let's start with your resume").is_visible()
    assert page.get_by_label("Phone Number").is_visible()
    assert page.get_by_label("Resume (PDF or DOCX)").is_visible()
    assert page.get_by_role("button", name="Continue").is_visible()
    print("✓ Profile test passed")

def test_resume_upload(page):
    """Test the resume upload functionality"""
    print("Loading resume upload page...")
    page.goto("http://localhost:2304/profile/resume/upload")
    print("Checking resume upload form...")
    
    # Check for resume upload elements
    assert page.get_by_text("Let's start with your resume").is_visible()
    assert page.get_by_label("Phone Number").is_visible()
    assert page.get_by_label("Resume (PDF or DOCX)").is_visible()
    assert page.get_by_role("button", name="Continue").is_visible()
    print("✓ Resume upload test passed")

def test_scheduling(page):
    """Test the scheduling functionality"""
    print("Loading scheduling page...")
    page.goto("http://localhost:2304/profile")
    print("Checking scheduling elements...")
    
    # Check for scheduling elements
    assert page.get_by_text("Let's schedule a quick AI chat").is_visible()
    assert page.get_by_label("Preferred Time").is_visible()
    assert page.get_by_role("button", name="Schedule Call").is_visible()
    assert page.get_by_role("button", name="Upload Different Resume").is_visible()
    print("✓ Scheduling test passed")

def test_phone_call(page):
    """Test the phone call functionality"""
    print("Loading phone call page...")
    page.goto("http://localhost:2304/profile")
    print("Checking phone call elements...")
    
    # Check for phone call elements
    assert page.get_by_text("Great! We've processed your resume").is_visible()
    assert page.get_by_text("Let's schedule a quick AI chat").is_visible()
    assert page.get_by_role("button", name="Schedule Call").is_visible()
    print("✓ Phone call test passed")

def run_go_tests(package_path=None, with_race=True, with_coverage=False, coverage_file="/tmp/coverage.out"):
    """Run Go tests with configurable options.
    
    Args:
        package_path (str, optional): Specific package to test. Tests all packages if None.
        with_race (bool): Enable race detection. Defaults to True.
        with_coverage (bool): Enable coverage reporting. Defaults to False.
        coverage_file (str): Path to coverage output file. Defaults to "/tmp/coverage.out".
    """
    cmd = ["go", "test"]
    
    if with_race:
        cmd.append("-race")
    
    cmd.append("-buildvcs")
    
    if with_coverage:
        cmd.extend([f"-coverprofile={coverage_file}"])
    
    cmd.append("-v")  # Verbose output
    
    if package_path:
        cmd.append(f"./{package_path}")
    else:
        cmd.append("./...")
        
    print(f"Running: {' '.join(cmd)}")
    
    try:
        result = subprocess.run(cmd, 
                              text=True,
                              capture_output=True)
        
        # Print output
        print(result.stdout)
        if result.stderr:
            print("Errors:", result.stderr, file=sys.stderr)
            
        # Check if tests passed
        if result.returncode != 0:
            print(f"Tests failed with exit code: {result.returncode}")
            return False
            
        print("All tests passed successfully")
        
        # Show coverage if enabled
        if with_coverage:
            print("\nGenerating coverage report...")
            subprocess.run(["go", "tool", "cover", f"-html={coverage_file}"])
            
        return True
            
    except subprocess.CalledProcessError as e:
        print(f"Error running tests: {e}", file=sys.stderr)
        return False

def query_db(query, params=None, db_path="db.sqlite", format="table"):
    """Execute a SQL query against the SQLite database and display results.
    
    Args:
        query (str): SQL query to execute
        params (tuple, optional): Query parameters if using parameterized queries
        db_path (str): Path to the SQLite database file. Defaults to "db.sqlite"
        format (str): Output format - "table", "json", or "csv". Defaults to "table"
    
    Returns:
        bool: True if query executed successfully, False otherwise
    """
    try:
        # Connect to database
        conn = sqlite3.connect(db_path)
        
        # Enable column names in results
        conn.row_factory = sqlite3.Row
        
        # Execute query
        cursor = conn.cursor()
        if params:
            cursor.execute(query, params)
        else:
            cursor.execute(query)
            
        # Fetch results
        rows = cursor.fetchall()
        
        if not rows:
            print("Query returned no results")
            return True
            
        # Get column names
        columns = [description[0] for description in cursor.description]
        
        # Convert rows to list of dicts for easier handling
        results = [dict(row) for row in rows]
        
        # Format and display results
        if format.lower() == "json":
            print(json.dumps(results, indent=2))
        elif format.lower() == "csv":
            # Print header
            print(",".join(columns))
            # Print rows
            for row in results:
                print(",".join(str(row[col]) for col in columns))
        else:  # table format
            print(tabulate(results, headers="keys", tablefmt="psql"))
            
        # Print row count
        print(f"\nRows returned: {len(results)}")
        
        return True
        
    except sqlite3.Error as e:
        print(f"Database error: {e}", file=sys.stderr)
        return False
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        return False
    finally:
        if 'conn' in locals():
            conn.close()

if __name__ == "__main__":
    # Don't run directly - import and use via python -c
    print("Please import and use these functions via python -c")
    sys.exit(1) 