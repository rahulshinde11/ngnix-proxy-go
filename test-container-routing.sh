#!/bin/bash

# Test script to verify container routing fix
# Tests that traffic goes to correct containers and NOT to wrong ones after container stops
# This simulates the EXACT bug scenario: container goes offline, traffic should NOT go to other containers

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Test results tracking
TESTS_PASSED=0
TESTS_FAILED=0
CRITICAL_FAILURES=0

echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║   Container Routing Bug Test - Offline Container Check   ║${NC}"
echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
echo ""
echo -e "${CYAN}This test verifies the critical bug fix:${NC}"
echo -e "${CYAN}When a container goes OFFLINE, its traffic should return 503${NC}"
echo -e "${CYAN}and NEVER be routed to a different container!${NC}"
echo ""

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up test containers...${NC}"
    docker rm -f test-app1 test-app2 test-app3 2>/dev/null || true
    docker network rm test-frontend 2>/dev/null || true
}

# Trap to ensure cleanup on exit
trap cleanup EXIT

# Create network
echo -e "${BLUE}1. Creating test network...${NC}"
docker network create test-frontend || true

# Start nginx-proxy-go container (assuming it's already built)
echo -e "${BLUE}2. Checking nginx-proxy-go container...${NC}"
if ! docker ps | grep -q nginx-proxy-go; then
    echo -e "${YELLOW}nginx-proxy-go container not running. Please start it first.${NC}"
    echo "Run: docker-compose up -d"
    exit 1
fi

# Ensure nginx-proxy-go is on test-frontend network
docker network connect test-frontend nginx-proxy-go 2>/dev/null || echo "Already connected"

# Wait a bit for network connection
sleep 2

# Start test container 1 - nginx with custom index
echo -e "${BLUE}3. Starting test-app1 (nginx) with VIRTUAL_HOST=app1.test...${NC}"
docker run -d --name test-app1 \
    --network test-frontend \
    -e VIRTUAL_HOST=app1.test \
    nginx:alpine sh -c 'echo "<h1>APP1 - NGINX</h1>" > /usr/share/nginx/html/index.html && nginx -g "daemon off;"'

# Start test container 2 - httpd with custom index
echo -e "${BLUE}4. Starting test-app2 (httpd) with VIRTUAL_HOST=app2.test...${NC}"
docker run -d --name test-app2 \
    --network test-frontend \
    -e VIRTUAL_HOST=app2.test \
    httpd:alpine sh -c 'echo "<h1>APP2 - HTTPD</h1>" > /usr/local/apache2/htdocs/index.html && httpd-foreground'

# Start test container 3 - another nginx instance
echo -e "${BLUE}5. Starting test-app3 (nginx) with VIRTUAL_HOST=app3.test...${NC}"
docker run -d --name test-app3 \
    --network test-frontend \
    -e VIRTUAL_HOST=app3.test \
    nginx:alpine sh -c 'echo "<h1>APP3 - ANOTHER NGINX</h1>" > /usr/share/nginx/html/index.html && nginx -g "daemon off;"'

# Wait for containers to be ready and nginx-proxy-go to process them
echo -e "${YELLOW}Waiting 10 seconds for nginx-proxy-go to detect containers...${NC}"
sleep 10

# Get nginx-proxy-go port
PROXY_PORT=$(docker port nginx-proxy-go 80 2>/dev/null | cut -d: -f2)
if [ -z "$PROXY_PORT" ]; then
    PROXY_PORT=80
fi

echo -e "${BLUE}Using proxy port: ${PROXY_PORT}${NC}"
echo ""

# Test function
test_request() {
    local host=$1
    local expected=$2
    local description=$3
    
    echo -e "${YELLOW}Testing: ${description}${NC}"
    echo -e "  ${CYAN}→ curl -H \"Host: ${host}\" http://localhost:${PROXY_PORT}/${NC}"
    
    response=$(curl -s -H "Host: ${host}" http://localhost:${PROXY_PORT}/ 2>/dev/null || echo "ERROR")
    
    if echo "$response" | grep -q "$expected"; then
        echo -e "  ${GREEN}✓ PASS${NC} - Got expected: ${expected}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "  ${RED}✗ FAIL${NC} - Expected '${expected}' but got:"
        echo "  $response"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

# Critical test function - checks that we DON'T get wrong container content
test_no_wrong_content() {
    local host=$1
    local forbidden_pattern=$2
    local description=$3
    
    echo -e "${YELLOW}CRITICAL TEST: ${description}${NC}"
    echo -e "  ${CYAN}→ curl -H \"Host: ${host}\" http://localhost:${PROXY_PORT}/${NC}"
    
    response=$(curl -s -H "Host: ${host}" http://localhost:${PROXY_PORT}/ 2>/dev/null || echo "CONNECTION_ERROR")
    
    if echo "$response" | grep -q "$forbidden_pattern"; then
        echo -e "  ${RED}✗✗✗ CRITICAL FAIL ✗✗✗${NC}"
        echo -e "  ${RED}BUG DETECTED: Got forbidden content: ${forbidden_pattern}${NC}"
        echo -e "  ${RED}This means traffic was routed to the WRONG container!${NC}"
        echo "  Response: $response"
        CRITICAL_FAILURES=$((CRITICAL_FAILURES + 1))
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    else
        echo -e "  ${GREEN}✓ PASS${NC} - Did NOT get wrong container content"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    fi
}

echo -e "${BLUE}=== Phase 1: All Containers Running ===${NC}"
echo ""

# Test all three apps work correctly
test_request "app1.test" "APP1 - NGINX" "app1.test should return APP1"
test_request "app2.test" "APP2 - HTTPD" "app2.test should return APP2"
test_request "app3.test" "APP3 - ANOTHER NGINX" "app3.test should return APP3"

echo ""
echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Phase 2: STOP APP1 - This is THE Critical Bug Test      ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════════╝${NC}"
echo ""

# Stop app1
echo -e "${RED}⦿ STOPPING test-app1 (simulating container going offline)...${NC}"
docker stop test-app1
echo -e "${GREEN}  Container test-app1 is now OFFLINE${NC}"

# Wait for nginx-proxy-go to process the container die event
echo ""
echo -e "${YELLOW}⏱  Waiting 8 seconds for nginx-proxy-go to detect and process container die event...${NC}"
for i in {8..1}; do
    echo -ne "\r  ${CYAN}${i} seconds remaining...${NC}"
    sleep 1
done
echo ""
echo ""

# THE CRITICAL TEST - app1 should return 503, NOT app2 or app3 content!
echo -e "${RED}════════════════════════════════════════════════════════════${NC}"
echo -e "${RED}  CRITICAL BUG TEST #1: Check app1.test after it went offline${NC}"
echo -e "${RED}════════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "${CYAN}Expected behavior: HTTP 503 (Service Unavailable)${NC}"
echo -e "${CYAN}BUG behavior: Shows APP2 or APP3 content (wrong container!)${NC}"
echo ""

response=$(curl -s -w "\n%{http_code}" -H "Host: app1.test" http://localhost:${PROXY_PORT}/ 2>/dev/null)
http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | head -n-1)

echo -e "${YELLOW}Testing: app1.test (OFFLINE)${NC}"
echo -e "  ${CYAN}→ curl -s -w '\\n%{http_code}' -H \"Host: app1.test\" http://localhost:${PROXY_PORT}/${NC}"
echo -e "  HTTP Status Code: ${http_code}"
echo -e "  Response Body: ${body:0:100}..."
echo ""

# Check for the bug
if echo "$body" | grep -qi "APP2"; then
    echo -e "  ${RED}✗✗✗ CRITICAL BUG DETECTED ✗✗✗${NC}"
    echo -e "  ${RED}Traffic for app1.test went to APP2 (test-app2) container!${NC}"
    echo -e "  ${RED}This is the exact bug that was reported!${NC}"
    CRITICAL_FAILURES=$((CRITICAL_FAILURES + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
elif echo "$body" | grep -qi "APP3"; then
    echo -e "  ${RED}✗✗✗ CRITICAL BUG DETECTED ✗✗✗${NC}"
    echo -e "  ${RED}Traffic for app1.test went to APP3 (test-app3) container!${NC}"
    echo -e "  ${RED}This is the exact bug that was reported!${NC}"
    CRITICAL_FAILURES=$((CRITICAL_FAILURES + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
elif [ "$http_code" = "503" ]; then
    echo -e "  ${GREEN}✓✓✓ PASS - Bug is FIXED! ✓✓✓${NC}"
    echo -e "  ${GREEN}Got expected 503 (no backend available)${NC}"
    echo -e "  ${GREEN}Traffic was NOT routed to wrong container${NC}"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "  ${YELLOW}⚠ WARNING${NC} - Got unexpected status ${http_code}"
    echo -e "  Expected: 503, Got: ${http_code}"
    if [ "$http_code" = "502" ]; then
        echo -e "  ${YELLOW}502 might indicate upstream still has dead container${NC}"
    fi
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

echo ""
echo -e "${RED}════════════════════════════════════════════════════════════${NC}"
echo ""

# Additional critical tests - make sure we don't get wrong content
test_no_wrong_content "app1.test" "APP2" "app1.test should NOT return APP2 content"
test_no_wrong_content "app1.test" "APP3" "app1.test should NOT return APP3 content"
test_no_wrong_content "app1.test" "HTTPD" "app1.test should NOT return HTTPD content"

echo ""
echo -e "${CYAN}Verifying other containers still work correctly:${NC}"

# Verify app2 and app3 still work
test_request "app2.test" "APP2 - HTTPD" "app2.test should still return APP2"
test_request "app3.test" "APP3 - ANOTHER NGINX" "app3.test should still return APP3"

echo ""
echo -e "${BLUE}=== Phase 3: Stop APP2, Verify APP3 Still Works ===${NC}"
echo ""

# Stop app2
echo -e "${YELLOW}Stopping test-app2...${NC}"
docker stop test-app2

# Wait for update
echo -e "${YELLOW}Waiting 5 seconds for nginx-proxy-go to update config...${NC}"
sleep 5

# Test app3 still works (should NOT get app1 or app2 content)
test_request "app3.test" "APP3 - ANOTHER NGINX" "app3.test should still return APP3 (not app1 or app2)"

# Test app2 returns 503
echo -e "${YELLOW}Testing: app2.test after stopping (should return 503)${NC}"
response=$(curl -s -w "\n%{http_code}" -H "Host: app2.test" http://localhost:${PROXY_PORT}/ 2>/dev/null)
http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | head -n-1)

if [ "$http_code" = "503" ]; then
    echo -e "  ${GREEN}✓ PASS${NC} - Got expected 503"
elif echo "$body" | grep -q "APP1\|APP3"; then
    echo -e "  ${RED}✗ CRITICAL FAIL${NC} - Got wrong app content!"
    exit 1
else
    echo -e "  ${YELLOW}⚠ WARNING${NC} - Got unexpected status ${http_code}"
fi

echo ""
echo -e "${BLUE}=== Phase 4: Restart APP1, Verify Correct Routing ===${NC}"
echo ""

# Restart app1
echo -e "${YELLOW}Restarting test-app1...${NC}"
docker start test-app1

# Wait for container to be ready and nginx-proxy-go to process it
echo -e "${YELLOW}Waiting 8 seconds for container startup and config update...${NC}"
sleep 8

# Test app1 works again with correct content
test_request "app1.test" "APP1 - NGINX" "app1.test should return APP1 again (not APP3)"
test_request "app3.test" "APP3 - ANOTHER NGINX" "app3.test should still return APP3"

echo ""
echo -e "${BLUE}=== Phase 5: Check Nginx Config ===${NC}"
echo ""

echo -e "${YELLOW}Checking generated nginx configuration...${NC}"
docker exec nginx-proxy-go cat /etc/nginx/conf.d/default.conf | grep -A 5 "upstream\|server_name" | head -50

echo ""
echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║                     TEST SUMMARY                          ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════════╝${NC}"
echo ""

echo -e "${CYAN}Tests Passed: ${GREEN}${TESTS_PASSED}${NC}"
echo -e "${CYAN}Tests Failed: ${RED}${TESTS_FAILED}${NC}"
echo -e "${CYAN}Critical Failures: ${RED}${CRITICAL_FAILURES}${NC}"
echo ""

if [ $CRITICAL_FAILURES -gt 0 ]; then
    echo -e "${RED}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║          ✗✗✗ CRITICAL BUG DETECTED ✗✗✗                   ║${NC}"
    echo -e "${RED}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${RED}Traffic was routed to WRONG containers!${NC}"
    echo -e "${RED}The container routing bug still exists.${NC}"
    echo ""
    echo -e "${YELLOW}What this means:${NC}"
    echo -e "  • When a container goes offline, its traffic went to another container"
    echo -e "  • Users would see the wrong application"
    echo -e "  • This is a critical security and data integrity issue"
    echo ""
    exit 1
elif [ $TESTS_FAILED -gt 0 ]; then
    echo -e "${YELLOW}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${YELLOW}║               ⚠ SOME TESTS FAILED ⚠                      ║${NC}"
    echo -e "${YELLOW}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${YELLOW}Some tests failed, but no critical bugs detected${NC}"
    exit 1
else
    echo -e "${GREEN}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║          ✓✓✓ ALL TESTS PASSED ✓✓✓                       ║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${GREEN}✓ Bug is FIXED! Containers route correctly${NC}"
    echo -e "${GREEN}✓ Stopped containers return 503 (not other container's content)${NC}"
    echo -e "${GREEN}✓ No cross-contamination between virtual hosts${NC}"
    echo -e "${GREEN}✓ Restarted containers work correctly${NC}"
    echo -e "${GREEN}✓ Traffic isolation verified${NC}"
    echo ""
    echo -e "${CYAN}The container routing bug fix is working correctly!${NC}"
fi
