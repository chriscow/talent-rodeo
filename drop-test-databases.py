#!/usr/bin/env python3
import re
import os
import subprocess
import psycopg2
from psycopg2.extensions import ISOLATION_LEVEL_AUTOCOMMIT
from dotenv import load_dotenv

def drop_test_databases():
    """
    Drop all PostgreSQL databases matching the pattern 'talent_rodeo_test_' followed by numbers.
    Connection parameters are loaded from .env file.
    """
    # Load environment variables from .env file
    load_dotenv()
    
    # Get database connection parameters from environment variables
    host = os.getenv('DB_HOST', 'localhost')
    port = os.getenv('DB_PORT', '5432')
    user = os.getenv('DB_USER', 'postgres')
    password = os.getenv('DB_PASSWORD', '')
    
    # Connect to the postgres database to get a list of all databases
    conn_params = {
        'dbname': 'postgres',  # Connect to postgres database to list/drop other DBs
        'user': user,
        'host': host,
        'port': port
    }
    
    if password:
        conn_params['password'] = password
    
    try:
        # Connect to PostgreSQL
        print(f"Connecting to PostgreSQL server at {host}:{port} as {user}...")
        conn = psycopg2.connect(**conn_params)
        conn.set_isolation_level(ISOLATION_LEVEL_AUTOCOMMIT)  # Required for database operations
        cursor = conn.cursor()
        
        # Query to find databases matching our pattern
        cursor.execute("SELECT datname FROM pg_database WHERE datname ~ '^talent_rodeo_test_[0-9]+$'")
        db_names = [row[0] for row in cursor.fetchall()]
        
        if not db_names:
            print("No matching databases found.")
            cursor.close()
            conn.close()
            return
        
        print(f"Found {len(db_names)} databases matching the pattern:")
        for db in db_names:
            print(f"  - {db}")
        
        # Ask for confirmation before dropping
        confirm = input("\nAre you sure you want to drop these databases? (y/n): ")
        if confirm.lower() != 'y':
            print("Operation cancelled.")
            cursor.close()
            conn.close()
            return
        
        # Drop each database
        for db_name in db_names:
            try:
                print(f"Dropping database: {db_name}")
                cursor.execute(f"DROP DATABASE {db_name}")
                print(f"  ✓ Successfully dropped {db_name}")
            except Exception as e:
                print(f"  ✗ Error dropping {db_name}: {str(e)}")
        
        print("\nOperation completed.")
        cursor.close()
        conn.close()
        
    except Exception as e:
        print(f"Error: {str(e)}")


if __name__ == "__main__":
    drop_test_databases()