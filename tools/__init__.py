"""
Talent Rodeo project management tools package.

This package provides utilities for managing various aspects of the Talent Rodeo project,
including UI testing, Go testing, database utilities, and feature analysis.
"""

# Import and expose key functions from each module
from .ui_tests import run_ui_test
from .go_tests import run_go_tests, test_tier_system, analyze_tier_consumer_issues, setup_test_candidate
from .db_utils import query_db, store_memory, get_memories
from .feature_analysis import analyze_feature_dependencies
from .feature_analyzer import analyze_feature_dependencies as analyze_feature_deps

# Define __all__ to control what's imported with 'from tools import *'
__all__ = [
    'run_ui_test',
    'run_go_tests',
    'test_tier_system', 
    'analyze_tier_consumer_issues',
    'setup_test_candidate',
    'query_db',
    'store_memory',
    'get_memories',
    'analyze_feature_dependencies',
    'analyze_feature_deps'
] 