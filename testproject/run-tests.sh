#!/usr/bin/env bash
set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Connection config
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-myuser}"
DB_PASSWORD="${DB_PASSWORD:-mypassword}"
DB_NAME="${DB_NAME:-referral}"
REDIS_ADDR="${REDIS_ADDR:-localhost:6379}"
KAFKA_BROKER="${KAFKA_BROKER:-localhost:9092}"
API_PORT="${API_PORT:-8000}"

FLOWTEST="../flowtest"
API_PID=""

cleanup() {
    echo -e "\n${CYAN}Cleaning up...${NC}"
    if [ -n "$API_PID" ] && kill -0 "$API_PID" 2>/dev/null; then
        kill "$API_PID" 2>/dev/null || true
        wait "$API_PID" 2>/dev/null || true
        echo "  Stopped test API server"
    fi
}
trap cleanup EXIT

# --- Step 1: Build flowtest if needed ---
echo -e "${CYAN}=== Building flowtest ===${NC}"
if [ ! -f "$FLOWTEST" ]; then
    echo "  Building flowtest binary..."
    (cd .. && go build -o flowtest ./cmd/flowtest/)
fi
echo -e "  ${GREEN}flowtest binary ready${NC}"

# --- Step 2: Build test server ---
echo -e "\n${CYAN}=== Building test API server ===${NC}"
go build -o testserver .
echo -e "  ${GREEN}testserver binary ready${NC}"

# --- Step 3: Start Docker services ---
echo -e "\n${CYAN}=== Starting Docker services ===${NC}"
docker compose up -d
echo "  Waiting for services to be healthy..."

# Wait for Postgres
echo -n "  Postgres: "
for i in $(seq 1 30); do
    if nc -z "$DB_HOST" "$DB_PORT" 2>/dev/null; then
        sleep 1  # brief settle after port opens
        echo -e "${GREEN}ready${NC}"
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo -e "${RED}timeout${NC}"
    fi
    sleep 1
done

# Wait for MySQL
echo -n "  MySQL: "
for i in $(seq 1 30); do
    if nc -z localhost 3306 2>/dev/null; then
        sleep 2  # MySQL needs extra settle time
        echo -e "${GREEN}ready${NC}"
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo -e "${RED}timeout${NC}"
    fi
    sleep 1
done

# Wait for MongoDB
echo -n "  MongoDB: "
for i in $(seq 1 30); do
    if nc -z localhost 27017 2>/dev/null; then
        sleep 1
        echo -e "${GREEN}ready${NC}"
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo -e "${RED}timeout${NC}"
    fi
    sleep 1
done

# Wait for Redis
echo -n "  Redis: "
for i in $(seq 1 30); do
    if nc -z "${REDIS_ADDR%%:*}" "${REDIS_ADDR##*:}" 2>/dev/null; then
        echo -e "${GREEN}ready${NC}"
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo -e "${RED}timeout${NC}"
    fi
    sleep 1
done

# Wait for Kafka
echo -n "  Kafka: "
for i in $(seq 1 30); do
    if nc -z "${KAFKA_BROKER%%:*}" "${KAFKA_BROKER##*:}" 2>/dev/null; then
        echo -e "${GREEN}ready${NC}"
        sleep 3  # Kafka needs extra time after port opens
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo -e "${RED}timeout — Kafka may not be fully ready${NC}"
    fi
    sleep 1
done

# --- Step 4: Start test API server ---
echo -e "\n${CYAN}=== Starting test API server ===${NC}"
DB_HOST="$DB_HOST" DB_PORT="$DB_PORT" DB_USER="$DB_USER" DB_PASSWORD="$DB_PASSWORD" DB_NAME="$DB_NAME" \
  REDIS_ADDR="$REDIS_ADDR" KAFKA_BROKER="$KAFKA_BROKER" API_PORT="$API_PORT" \
  ./testserver &
API_PID=$!

# Wait for API
echo -n "  API server: "
for i in $(seq 1 15); do
    if curl -sf "http://localhost:${API_PORT}/health" &>/dev/null; then
        echo -e "${GREEN}ready${NC}"
        break
    fi
    if [ "$i" -eq 15 ]; then
        echo -e "${RED}timeout${NC}"
        echo "  Check testserver logs above for errors"
        exit 1
    fi
    sleep 1
done

# --- Step 5: Validate all flows ---
echo -e "\n${CYAN}=== Validating flows ===${NC}"
for flow in flows/*.yaml; do
    echo -n "  $flow: "
    if "$FLOWTEST" validate "$flow" &>/dev/null; then
        echo -e "${GREEN}valid${NC}"
    else
        echo -e "${RED}invalid${NC}"
        "$FLOWTEST" validate "$flow"
        exit 1
    fi
done

# --- Step 6: Run all flows ---
echo -e "\n${CYAN}=== Running flow tests ===${NC}"
FAILED=0

# Create reports directory
REPORTS_DIR="./test-reports"
mkdir -p "$REPORTS_DIR"
rm -f "$REPORTS_DIR"/*.html

for flow in flows/*.yaml; do
    flow_name=$(basename "$flow" .yaml)
    echo -e "\n${YELLOW}--- $flow ---${NC}"
    
    # Generate HTML report for this flow
    report_file="$REPORTS_DIR/${flow_name}.html"
    
    if "$FLOWTEST" run "$flow" -v --html-report "$report_file"; then
        echo -e "${GREEN}  PASSED${NC}"
        echo -e "  📄 Report: ${CYAN}$report_file${NC}"
    else
        echo -e "${RED}  FAILED${NC}"
        echo -e "  📄 Report: ${CYAN}$report_file${NC}"
        FAILED=$((FAILED + 1))
    fi
done

# --- Summary ---
echo -e "\n${CYAN}==============================${NC}"
if [ "$FAILED" -eq 0 ]; then
    echo -e "${GREEN}All flow tests passed!${NC}"
    echo -e "\n${CYAN}📊 HTML Reports:${NC}"
    echo -e "  Location: ${YELLOW}$REPORTS_DIR/${NC}"
    echo -e "  Open in browser:"
    for report in "$REPORTS_DIR"/*.html; do
        if [ -f "$report" ]; then
            echo -e "    file://$PWD/$report"
        fi
    done
else
    echo -e "${RED}${FAILED} flow(s) failed${NC}"
    echo -e "\n${CYAN}📊 HTML Reports (including failures):${NC}"
    echo -e "  Location: ${YELLOW}$REPORTS_DIR/${NC}"
    exit 1
fi
