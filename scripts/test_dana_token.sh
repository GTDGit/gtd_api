#!/bin/sh
# Test DANA token acquisition directly inside container
# Run: docker exec -i gtd-api sh < /tmp/test_dana_token.sh

# Check if dana key exists
ls -la /app/keys/dana/

# Try to get DANA token directly
DANA_BASE_URL="https://api.saas.dana.id"
DANA_CLIENT_ID="2025080113571777143806"
DANA_TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%S+07:00")

echo "Timestamp: $DANA_TIMESTAMP"
echo "Client ID: $DANA_CLIENT_ID"
echo "Base URL: $DANA_BASE_URL"

# Just check if key is readable
head -3 /app/keys/dana/private_key.pem 2>&1 || echo "Cannot read dana key"
echo "Key size: $(wc -c < /app/keys/dana/private_key.pem 2>/dev/null || echo 0) bytes"
