#!/usr/bin/env python3
"""
Feature analysis utilities for the Talent Rodeo project.

This module provides functions for analyzing feature dependencies
and suggesting optimal implementation order.
"""

import os
import re
import glob
from collections import defaultdict


def analyze_feature_dependencies(project_dir=".project"):
    """
    Analyze feature dependencies and suggest implementation order.
    
    This function scans feature specification files, extracts dependency information,
    and outputs a recommended implementation order based on dependency analysis.
    
    Args:
        project_dir (str): Path to the project directory containing feature specs
        
    Returns:
        dict: Dependency information and implementation order recommendations
    """
    print("Analyzing feature dependencies...")
    
    feature_specs_dir = os.path.join(project_dir, "01_feature_specs")
    features = {}
    dependencies = defaultdict(list)
    
    # Find all feature spec files
    for feature_dir in glob.glob(os.path.join(feature_specs_dir, "F*")):
        feature_name = os.path.basename(feature_dir)
        spec_file = os.path.join(feature_dir, "00_feature_spec.md")
        
        if not os.path.exists(spec_file):
            print(f"Warning: No specification file found for {feature_name}")
            continue
            
        # Extract feature number
        match = re.match(r"F(\d+)_(.+)", feature_name)
        if not match:
            continue
            
        feature_num = int(match.group(1))
        feature_title = match.group(2).replace("_", " ")
        
        # Parse spec file to find dependencies
        with open(spec_file, 'r') as f:
            content = f.read()
            
            # Extract implementation status
            status_match = re.search(r"## Implementation Status\s+(.+?)(?=##|\Z)", content, re.DOTALL)
            status = "Unknown"
            if status_match:
                status_text = status_match.group(1)
                if "Not Implemented" in status_text:
                    status = "Not Implemented"
                elif "Partially Implemented" in status_text:
                    status = "Partially Implemented"
                elif "Implemented" in status_text:
                    status = "Implemented"
            
            # Extract dependencies
            deps_match = re.search(r"## Dependencies\s+(.+?)(?=##|\Z)", content, re.DOTALL)
            deps = []
            if deps_match:
                deps_section = deps_match.group(1)
                # Look for F### patterns
                feature_refs = re.findall(r"F(\d+)[:\s-]", deps_section)
                deps = [int(ref) for ref in feature_refs]
        
        # Store feature info
        features[feature_num] = {
            "number": feature_num,
            "name": feature_name,
            "title": feature_title,
            "status": status,
            "dependencies": deps,
            "path": spec_file
        }
        
        # Track dependencies
        for dep in deps:
            dependencies[feature_num].append(dep)
    
    # Calculate implementation order based on dependencies
    implementation_order = []
    visited = set()
    temp_visited = set()
    
    def visit(feature_num):
        if feature_num in temp_visited:
            # This indicates a circular dependency
            raise ValueError(f"Circular dependency detected involving feature F{feature_num:03}")
        
        if feature_num in visited:
            return
        
        temp_visited.add(feature_num)
        
        # Visit dependencies first
        for dep in dependencies[feature_num]:
            visit(dep)
        
        visited.add(feature_num)
        implementation_order.append(feature_num)
        temp_visited.remove(feature_num)
    
    # Visit each feature
    for feature_num in features:
        if feature_num not in visited:
            visit(feature_num)
    
    # Format results
    ordered_features = []
    for feature_num in implementation_order:
        if feature_num in features:
            ordered_features.append(features[feature_num])
    
    # Print results
    print("\nRecommended implementation order based on dependencies:\n")
    print(f"{'Num':^5} | {'Name':<30} | {'Status':<25} | {'Dependencies'}")
    print("-" * 80)
    
    for feature in ordered_features:
        deps_str = ", ".join(f"F{dep:03}" for dep in feature["dependencies"])
        print(f"F{feature['number']:03} | {feature['title']:<30} | {feature['status']:<25} | {deps_str}")
    
    print("\nNote: This analysis is based on explicit dependencies listed in feature specifications.")
    
    return {
        "features": features,
        "dependencies": dependencies,
        "implementation_order": ordered_features
    } 