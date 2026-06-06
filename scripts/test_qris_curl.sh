#!/bin/sh
# Run this inside the VPS: bash test_qris_curl.sh
# Tests all 4 QRIS providers by switching provider in payment_methods table

API="http://localhost:8080"
API_KEY="gb_live_6669d845b020b0602e968706184ba133f7a0d5be663531d8e5a92d38fea6705b"
CLIENT_ID="test"
TS=$(date +%s)

call_create() {
  PROVIDER="$1"
  REF="QRIS-${PROVIDER}-${TS}"
  echo ""
  echo "=== Testing QRIS via ${PROVIDER} (referenceId=${REF}) ==="
  curl -s -X POST "${API}/v1/payment/create" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${API_KEY}" \
    -H "X-Client-Id: ${CLIENT_ID}" \
    -d "{
      \"referenceId\": \"${REF}\",
      \"paymentMethod\": {\"type\": \"QRIS\", \"code\": \"MPM\"},
      \"amount\": 10000,
      \"customer\": {\"name\": \"John Doe\", \"email\": \"john@example.com\", \"phone\": \"081234567890\"},
      \"description\": \"Test QRIS ${PROVIDER}\"
    }" | python3 -c "
import json,sys
d=json.load(sys.stdin)
print('success:', d.get('success'))
print('code   :', d.get('code'))
print('message:', d.get('message'))
data = d.get('data') or {}
detail = data.get('paymentDetail') or {}
if isinstance(detail, str):
    try: detail = json.loads(detail)
    except: pass
print('status :', data.get('status'))
print('provider:', data.get('provider'))
qr = detail.get('qrString','') if isinstance(detail,dict) else ''
print('qrString:', qr[:80] + '...' if len(qr)>80 else qr)
if not d.get('success'):
    err = d.get('error') or {}
    print('error  :', err)
" 2>&1
}

# Switch provider in DB, then call API
switch_provider() {
  PROVIDER="$1"
  docker exec -i gtd-api sh -c "
    PGPASSWORD=GTD12345 psql -h dev-postgres.ctwq6wii0ika.ap-southeast-1.rds.amazonaws.com -U postgres -d gtd \
    -c \"UPDATE payment_methods SET provider='${PROVIDER}' WHERE type='QRIS' AND code='MPM';\" 2>&1 || echo 'psql not found in container'
  " 2>/dev/null || true

  # Use postgres container instead
  docker run --rm postgres:16-alpine sh -c \
    "PGPASSWORD=GTD12345 psql -h dev-postgres.ctwq6wii0ika.ap-southeast-1.rds.amazonaws.com -U postgres -d gtd \
     -c \"UPDATE payment_methods SET provider='${PROVIDER}' WHERE type='QRIS' AND code='MPM'; SELECT provider FROM payment_methods WHERE type='QRIS' AND code='MPM';\"" 2>&1
}

echo "=== QRIS Provider Tests ==="
echo "Calling from VPS IP (whitelisted by Pakailink/DANA)"

# Test 1: Midtrans
switch_provider "midtrans"
call_create "midtrans"

# Test 2: Xendit
switch_provider "xendit"
call_create "xendit"

# Test 3: Pakailink
switch_provider "pakailink"
call_create "pakailink"

# Test 4: DANA
switch_provider "dana_direct"
call_create "dana"

echo ""
echo "=== Done ==="
