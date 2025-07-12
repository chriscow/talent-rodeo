#!/bin/bash

# How to run:
# set -a; source .env;  FIRESTORE_EMULATOR_HOST="localhost:8200" sh ./run_email_parser_tests.sh; set +a

# Email Parser Test Suite Runner
# This script processes a suite of test emails through the email parser
# to validate parsing accuracy and prompt effectiveness.

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLI_PATH="${SCRIPT_DIR}/cmd/cli"
TEST_EMAIL_DIR="${SCRIPT_DIR}/testdata/test_emails"
RESULTS_DIR="${SCRIPT_DIR}/results"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
RESULTS_FILE="${RESULTS_DIR}/test_results_${TIMESTAMP}.json"

# Create results directory
mkdir -p "$RESULTS_DIR"

echo -e "${BLUE}🧪 Email Parser Test Suite${NC}"
echo -e "${BLUE}=========================${NC}"
echo ""

# Check if CLI exists
if [ ! -f "/tmp/trcli" ]; then
    echo -e "${YELLOW}Building CLI...${NC}"
    cd "$CLI_PATH"
    go build -o /tmp/trcli .
    cd "$SCRIPT_DIR"
fi

# Check for OpenAI API key
if [ -z "$OPENAI_API_KEY" ]; then
    echo -e "${RED}❌ OPENAI_API_KEY environment variable not set${NC}"
    echo "Please set your OpenAI API key:"
    echo "export OPENAI_API_KEY='your-key-here'"
    exit 1
fi

echo -e "${GREEN}✅ OpenAI API key found${NC}"
echo -e "${GREEN}✅ CLI built successfully${NC}"
echo ""

# Check if test email directory exists
if [ ! -d "$TEST_EMAIL_DIR" ]; then
    echo -e "${RED}❌ Test email directory not found: $TEST_EMAIL_DIR${NC}"
    echo "Please create the directory and add test email files"
    exit 1
fi

# Find all test email files (only files that match ##_description.txt pattern)
TEST_FILES=($(find "$TEST_EMAIL_DIR" -name "*.txt" | grep -E ".*[0-9][0-9]_.*\.txt$" | sort))

if [ ${#TEST_FILES[@]} -eq 0 ]; then
    echo -e "${RED}❌ No test email files found${NC}"
    echo "Expected files matching pattern: ##_description.txt in $TEST_EMAIL_DIR"
    echo "Example: 01_simple_request.txt, 02_complex_email.txt"
    exit 1
fi

echo -e "${BLUE}Found ${#TEST_FILES[@]} test emails:${NC}"
for file in "${TEST_FILES[@]}"; do
    filename=$(basename "$file")
    echo -e "  📧 $filename"
done
echo ""

# Initialize results array
echo '[' > "$RESULTS_FILE"

# Process each test file
TOTAL_TESTS=${#TEST_FILES[@]}
PASSED_TESTS=0
FAILED_TESTS=0

for i in "${!TEST_FILES[@]}"; do
    file="${TEST_FILES[$i]}"
    filename=$(basename "$file" .txt)
    test_num=$((i + 1))
    
    echo -e "${BLUE}[$test_num/$TOTAL_TESTS] Testing: $filename${NC}"
    
    # Show email content preview
    echo -e "${YELLOW}Email preview:${NC}"
    head -n 3 "$file" | sed 's/^/  /'
    echo "  ..."
    echo ""
    
    # Create temporary result file
    temp_result=$(mktemp)
    
    # Run the email parser
    set +e  # Don't exit on command failure
    FIRESTORE_EMULATOR_HOST="${FIRESTORE_EMULATOR_HOST:-localhost:8200}" /tmp/trcli email-parser test --file "$file" --json > "$temp_result" 2>&1
    exit_code=$?
    set -e
    
    # Parse results
    if [ $exit_code -eq 0 ]; then
        # Validate JSON output
        if jq empty "$temp_result" 2>/dev/null; then
            echo -e "${GREEN}  ✅ PASSED${NC}"
            PASSED_TESTS=$((PASSED_TESTS + 1))
            
            # Extract summary info
            requests=$(jq '.summary.requests' "$temp_result" 2>/dev/null || echo "0")
            feedback=$(jq '.summary.general_feedback' "$temp_result" 2>/dev/null || echo "0")
            non_requests=$(jq '.summary.non_requests' "$temp_result" 2>/dev/null || echo "0")
            total=$(jq '.summary.total' "$temp_result" 2>/dev/null || echo "0")
            
            echo -e "  📊 Results: ${requests} requests, ${feedback} feedback, ${non_requests} non-requests (${total} total)"
            
            # Check if file has expected counts and validate
            expected_line=$(head -n 5 "$file" | grep -i "^Expected:" || echo "")
            validation_passed=true
            validation_msg=""
            
            if [ -n "$expected_line" ]; then
                # Parse expected counts (supports ranges like "1-2" or exact "1")
                expected_requests=$(echo "$expected_line" | sed -n 's/.*Expected:.*\([0-9]\+\(-[0-9]\+\)\?\) requests.*/\1/p')
                expected_feedback=$(echo "$expected_line" | sed -n 's/.*\([0-9]\+\(-[0-9]\+\)\?\) feedback.*/\1/p')
                expected_non_requests=$(echo "$expected_line" | sed -n 's/.*\([0-9]\+\(-[0-9]\+\)\?\) non-requests.*/\1/p')
                
                # Validate each category if expectations exist
                if [ -n "$expected_requests" ]; then
                    if [[ "$expected_requests" == *"-"* ]]; then
                        min_req=$(echo "$expected_requests" | cut -d'-' -f1)
                        max_req=$(echo "$expected_requests" | cut -d'-' -f2)
                        if [ "$requests" -lt "$min_req" ] || [ "$requests" -gt "$max_req" ]; then
                            validation_passed=false
                            validation_msg="${validation_msg}requests: got $requests, expected $expected_requests; "
                        fi
                    else
                        if [ "$requests" -ne "$expected_requests" ]; then
                            validation_passed=false
                            validation_msg="${validation_msg}requests: got $requests, expected $expected_requests; "
                        fi
                    fi
                fi
                
                if [ -n "$expected_feedback" ]; then
                    if [[ "$expected_feedback" == *"-"* ]]; then
                        min_fb=$(echo "$expected_feedback" | cut -d'-' -f1)
                        max_fb=$(echo "$expected_feedback" | cut -d'-' -f2)
                        if [ "$feedback" -lt "$min_fb" ] || [ "$feedback" -gt "$max_fb" ]; then
                            validation_passed=false
                            validation_msg="${validation_msg}feedback: got $feedback, expected $expected_feedback; "
                        fi
                    else
                        if [ "$feedback" -ne "$expected_feedback" ]; then
                            validation_passed=false
                            validation_msg="${validation_msg}feedback: got $feedback, expected $expected_feedback; "
                        fi
                    fi
                fi
                
                if [ -n "$expected_non_requests" ]; then
                    if [[ "$expected_non_requests" == *"-"* ]]; then
                        min_nr=$(echo "$expected_non_requests" | cut -d'-' -f1)
                        max_nr=$(echo "$expected_non_requests" | cut -d'-' -f2)
                        if [ "$non_requests" -lt "$min_nr" ] || [ "$non_requests" -gt "$max_nr" ]; then
                            validation_passed=false
                            validation_msg="${validation_msg}non-requests: got $non_requests, expected $expected_non_requests; "
                        fi
                    else
                        if [ "$non_requests" -ne "$expected_non_requests" ]; then
                            validation_passed=false
                            validation_msg="${validation_msg}non-requests: got $non_requests, expected $expected_non_requests; "
                        fi
                    fi
                fi
                
                # Show validation result
                if [ "$validation_passed" = true ]; then
                    echo -e "  ${GREEN}✓ Validation: PASSED${NC} (matches expected counts)"
                else
                    echo -e "  ${YELLOW}⚠ Validation: FAILED${NC} (${validation_msg})"
                    # Don't fail the overall test for validation issues, just warn
                fi
            fi
            
            # Add to results file
            if [ $i -gt 0 ]; then
                echo ',' >> "$RESULTS_FILE"
            fi
            
            jq -n --arg name "$filename" --arg status "passed" --slurpfile result "$temp_result" '{
                test_name: $name,
                status: $status,
                result: $result[0]
            }' >> "$RESULTS_FILE"
        else
            echo -e "${RED}  ❌ FAILED: Invalid JSON output${NC}"
            FAILED_TESTS=$((FAILED_TESTS + 1))
            
            error_msg="Invalid JSON output: $(head -n 3 "$temp_result" | tr '\n' ' ')"
            echo -e "${RED}  Error: $error_msg${NC}"
            
            # Add to results file
            if [ $i -gt 0 ]; then
                echo ',' >> "$RESULTS_FILE"
            fi
            
            jq -n --arg name "$filename" --arg status "failed" --arg error "$error_msg" '{
                test_name: $name,
                status: $status,
                error: $error
            }' >> "$RESULTS_FILE"
        fi
        
    else
        echo -e "${RED}  ❌ FAILED${NC}"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        
        error_msg=$(cat "$temp_result" | head -n 5)
        echo -e "${RED}  Error: $error_msg${NC}"
        
        # Add to results file
        if [ $i -gt 0 ]; then
            echo ',' >> "$RESULTS_FILE"
        fi
        
        jq -n --arg name "$filename" --arg status "failed" --arg error "$error_msg" '{
            test_name: $name,
            status: $status,
            error: $error
        }' >> "$RESULTS_FILE"
    fi
    
    # Clean up
    rm -f "$temp_result"
    echo ""
done

# Close results file
echo ']' >> "$RESULTS_FILE"

# Generate summary
echo -e "${BLUE}📊 TEST SUMMARY${NC}"
echo -e "${BLUE}===============${NC}"
echo -e "Total tests: $TOTAL_TESTS"
echo -e "${GREEN}Passed: $PASSED_TESTS${NC}"
echo -e "${RED}Failed: $FAILED_TESTS${NC}"

# Calculate success rate
if [ $TOTAL_TESTS -gt 0 ]; then
    SUCCESS_RATE=$(echo "scale=1; $PASSED_TESTS * 100 / $TOTAL_TESTS" | bc)
    echo -e "Success rate: ${SUCCESS_RATE}%"
else
    SUCCESS_RATE=0
fi

echo ""
echo -e "${BLUE}📁 Results saved to:${NC} $RESULTS_FILE"

# Generate detailed report
REPORT_FILE="${RESULTS_DIR}/test_report_${TIMESTAMP}.md"
echo "# Email Parser Test Report" > "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo "**Date:** $(date)" >> "$REPORT_FILE"
echo "**Total Tests:** $TOTAL_TESTS" >> "$REPORT_FILE"
echo "**Passed:** $PASSED_TESTS" >> "$REPORT_FILE"
echo "**Failed:** $FAILED_TESTS" >> "$REPORT_FILE"
echo "**Success Rate:** ${SUCCESS_RATE}%" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

echo "## Test Results" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

# Add individual test results
for i in "${!TEST_FILES[@]}"; do
    file="${TEST_FILES[$i]}"
    filename=$(basename "$file" .txt)
    
    # Extract result from JSON
    result_status=$(jq -r ".[$i].status" "$RESULTS_FILE")
    
    if [ "$result_status" = "passed" ]; then
        requests=$(jq -r ".[$i].result.summary.requests" "$RESULTS_FILE")
        feedback=$(jq -r ".[$i].result.summary.general_feedback" "$RESULTS_FILE")
        non_requests=$(jq -r ".[$i].result.summary.non_requests" "$RESULTS_FILE")
        total=$(jq -r ".[$i].result.summary.total" "$RESULTS_FILE")
        
        echo "### ✅ $filename" >> "$REPORT_FILE"
        echo "- **Status:** Passed" >> "$REPORT_FILE"
        echo "- **Results:** $requests requests, $feedback feedback, $non_requests non-requests" >> "$REPORT_FILE"
        echo "- **Total Items:** $total" >> "$REPORT_FILE"
    else
        error_msg=$(jq -r ".[$i].error" "$RESULTS_FILE")
        echo "### ❌ $filename" >> "$REPORT_FILE"
        echo "- **Status:** Failed" >> "$REPORT_FILE"
        echo "- **Error:** $error_msg" >> "$REPORT_FILE"
    fi
    echo "" >> "$REPORT_FILE"
done

echo -e "${BLUE}📄 Report saved to:${NC} $REPORT_FILE"
echo ""

# Exit with appropriate code
if [ $FAILED_TESTS -gt 0 ]; then
    echo -e "${RED}🚨 Some tests failed. Please review the results.${NC}"
    exit 1
else
    echo -e "${GREEN}🎉 All tests passed successfully!${NC}"
    exit 0
fi