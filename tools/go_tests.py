#!/usr/bin/env python3
"""
Go testing utilities for the Talent Rodeo project.

This module provides functions for running and analyzing Go tests,
including tier system tests and handling common test failures.
"""

import os
import sys
import subprocess


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


def test_tier_system(package_path="./internal/tasks/handlers/...", tier_test=None, with_isolation=True):
    """Run tier system search scheduler tests with proper isolation.
    
    This function helps run the tier system tests with the right configuration to
    prevent test interference due to shared NATS consumers.
    
    Args:
        package_path (str): Path to the package containing tier tests
        tier_test (str, optional): Specific tier test to run (e.g. "TestSearchScheduler_TierLimits")
        with_isolation (bool): Whether to run tests with isolation to prevent consumer overlap
    
    Returns:
        bool: True if tests passed, False otherwise
    """
    cmd = ["go", "test", "-v"]
    
    # Add test filter if specified
    if tier_test:
        cmd.extend(["-run", tier_test])
    
    # Add package path
    cmd.append(package_path)
    
    # For testing with isolation, run one test at a time
    if with_isolation and not tier_test:
        cmd.append("-parallel=1")
    
    print(f"Running tier system tests: {' '.join(cmd)}")
    
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
            print(f"Tier tests failed with exit code: {result.returncode}")
            return False
            
        print("All tier system tests passed successfully")
        return True
            
    except subprocess.CalledProcessError as e:
        print(f"Error running tier tests: {e}", file=sys.stderr)
        return False


def analyze_tier_consumer_issues(test_output):
    """Analyze test output for common tier system consumer issues.
    
    Args:
        test_output (str): The output from a failed tier system test
        
    Returns:
        dict: Analysis results with issue type and suggested fixes
    """
    issues = {
        "consumer_name_conflicts": False,
        "delivery_policy_issues": False,
        "message_overlap": False,
        "nil_pointer": False,
        "suggestions": []
    }
    
    # Check for consumer name conflicts
    if "consumer name already in use" in test_output or "consumer name contains invalid characters" in test_output:
        issues["consumer_name_conflicts"] = True
        issues["suggestions"].append(
            "Use unique consumer names for each test case and sanitize special characters like '/' in the names."
        )
    
    # Check for message overlap issues
    if "unexpected number of messages" in test_output or "got messages, want" in test_output:
        issues["message_overlap"] = True
        issues["suggestions"].append(
            "Use DeliverNewPolicy instead of DeliverAllPolicy to prevent message overlap between tests."
        )
    
    # Check for nil pointer dereferences
    if "nil pointer dereference" in test_output:
        issues["nil_pointer"] = True
        issues["suggestions"].append(
            "Ensure the MockTaskProcessor is properly initialized with database connection and logger."
        )
    
    # Check for delivery policy issues
    if "failed to create consumer" in test_output:
        issues["delivery_policy_issues"] = True
        issues["suggestions"].append(
            "Check consumer configuration and ensure FilterSubject matches the stream name prefix."
        )
    
    print("\nTier System Test Analysis:")
    print("="*80)
    for issue, present in issues.items():
        if issue != "suggestions" and present:
            print(f"✗ {issue.replace('_', ' ').title()}: Detected")
    
    if issues["suggestions"]:
        print("\nSuggested fixes:")
        for i, suggestion in enumerate(issues["suggestions"], 1):
            print(f"{i}. {suggestion}")
    else:
        print("\nNo common issues detected. Check test output for more details.")
    
    return issues


def setup_test_candidate(tier='free', last_page=None, last_search_at=None, subscription_end=None):
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
    code = """
// Create test user and candidate
user, err := env.db.CreateTestUser(t)
if err != nil {
    t.Fatalf("failed to create test user: %v", err)
}

candidateID, err := env.db.CreateTestCandidate(t, user.ID)
if err != nil {
    t.Fatalf("failed to create test candidate: %v", err)
}

// Update candidate with specific tier settings
_, err = env.db.Exec(`
    UPDATE candidates 
    SET tier = $1::tier_type"""
    
    # Add last_page if specified
    if last_page is not None:
        code += ",\n        last_page = $2"
    
    # Add last_search_at if specified
    if last_search_at is not None:
        next_param = 3 if last_page is not None else 2
        code += f",\n        last_search_at = ${next_param}"
    
    # Add subscription_end_date if specified
    if subscription_end is not None:
        next_param = 2
        if last_page is not None:
            next_param += 1
        if last_search_at is not None:
            next_param += 1
        code += f",\n        subscription_end_date = ${next_param}"
    
    code += """
    WHERE id = $"""
    
    next_param = 2
    if last_page is not None:
        next_param += 1
    if last_search_at is not None:
        next_param += 1
    if subscription_end is not None:
        next_param += 1
    
    code += str(next_param) + "`"
    
    # Add parameters array
    params = [tier]
    if last_page is not None:
        params.append(str(last_page))
    if last_search_at is not None:
        params.append(last_search_at)
    if subscription_end is not None:
        params.append(subscription_end)
    params.append("candidateID")
    
    # Join parameters for the SQL query
    param_str = ", ".join(params)
    code += f",\n    {param_str})"
    
    code += """
if err != nil {
    t.Fatalf("failed to update candidate: %v", err)
}
"""
    
    print(code)
    return code 