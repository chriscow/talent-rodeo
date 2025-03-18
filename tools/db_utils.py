#!/usr/bin/env python3
"""
Database utilities for the Talent Rodeo project.

This module provides functions for interacting with SQLite databases,
including querying data and storing/retrieving project memories.
"""

import os
import sys
import json
import sqlite3
from tabulate import tabulate


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


def store_memory(category, content, tags=None):
    """Store a memory with metadata for future reference.
    
    Args:
        category (str): Category of the memory (e.g., 'code_insight', 'progress', 'lesson_learned')
        content (str): The actual memory content
        tags (list, optional): List of tags to help with categorization and search
    
    Returns:
        bool: True if memory was stored successfully, False otherwise
    """
    try:
        # Connect to SQLite database
        conn = sqlite3.connect('memories.db')
        cursor = conn.cursor()
        
        # Create memories table if it doesn't exist
        cursor.execute('''
            CREATE TABLE IF NOT EXISTS memories (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                category TEXT NOT NULL,
                content TEXT NOT NULL,
                tags TEXT,
                timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
            )
        ''')
        
        # Convert tags list to JSON string if provided
        tags_json = json.dumps(tags) if tags else None
        
        # Insert memory
        cursor.execute(
            'INSERT INTO memories (category, content, tags) VALUES (?, ?, ?)',
            (category, content, tags_json)
        )
        
        conn.commit()
        print(f"Memory stored successfully in category: {category}")
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


def get_memories(category=None, tags=None, since=None, format="table"):
    """Retrieve memories with optional filtering.
    
    Args:
        category (str, optional): Filter by category
        tags (list, optional): Filter by tags (matches if any tag matches)
        since (str, optional): ISO format date string to filter by date (e.g., '2024-02-04')
        format (str): Output format - "table", "json", or "text". Defaults to "table"
    
    Returns:
        bool: True if query executed successfully, False otherwise
    """
    try:
        # Connect to database
        conn = sqlite3.connect('memories.db')
        conn.row_factory = sqlite3.Row
        cursor = conn.cursor()
        
        # Build query based on filters
        query = 'SELECT * FROM memories'
        conditions = []
        params = []
        
        if category:
            conditions.append('category = ?')
            params.append(category)
            
        if tags:
            tag_conditions = []
            for tag in tags:
                tag_conditions.append('tags LIKE ?')
                params.append(f'%{tag}%')
            if tag_conditions:
                conditions.append(f"({' OR '.join(tag_conditions)})")
                
        if since:
            conditions.append('timestamp >= ?')
            params.append(since)
            
        if conditions:
            query += ' WHERE ' + ' AND '.join(conditions)
            
        query += ' ORDER BY timestamp DESC'
        
        # Execute query
        cursor.execute(query, params)
        rows = cursor.fetchall()
        
        if not rows:
            print("No memories found matching the criteria")
            return True
            
        # Format and display results
        results = []
        for row in rows:
            memory = dict(row)
            # Parse tags from JSON if they exist
            if memory['tags']:
                memory['tags'] = json.loads(memory['tags'])
            results.append(memory)
            
        if format.lower() == "json":
            print(json.dumps(results, indent=2))
        elif format.lower() == "text":
            for r in results:
                print(f"\n[{r['timestamp']}] {r['category'].upper()}")
                print("-" * 80)
                print(f"Content: {r['content']}")
                if r['tags']:
                    print(f"Tags: {', '.join(r['tags'])}")
                print()
        else:  # table format
            # Prepare table data
            table_data = []
            for r in results:
                tags_str = ', '.join(r['tags']) if r['tags'] else ''
                table_data.append({
                    'Timestamp': r['timestamp'],
                    'Category': r['category'],
                    'Content': r['content'][:100] + ('...' if len(r['content']) > 100 else ''),
                    'Tags': tags_str
                })
            print(tabulate(table_data, headers="keys", tablefmt="psql"))
            
        print(f"\nTotal memories found: {len(results)}")
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