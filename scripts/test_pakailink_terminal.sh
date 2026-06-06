#!/bin/sh
# Test Pakailink QRIS with different terminalId values
API="http://localhost:8080"
API_KEY="gb_live_6669d845b020b0602e968706184ba133f7a0d5be663531d8e5a92d38fea6705b"
CLIENT_ID="test"
TS=$(date +%s)

# Switch provider to pakailink
docker run --rm postgres:16-alpine sh -c \
  "PGPASSWORD=GTD12345 psql -h dev-postgres.ctwq6wii0ika.ap-southeast-1.rds.amazonaws.com -U postgres -d gtd \
   -c \"UPDATE payment_methods SET provider='pakailink' WHERE type='QRIS' AND code='MPM';\"" 2>/dev/null

test_pakailink() {
  TERMINAL="$1"
  LABEL="$2"
  echo ""
  echo "=== Pakailink QRIS terminalId=${LABEL} ==="
  
  # Update terminal ID in container env and restart would be needed...
  # Instead, test via direct check of current result since terminal is set
  REF="QRIS-PKL-${TERMINAL}-${TS}"
  if [ ${#REF} -gt 25 ]; then
    REF="${REF:0:25}"
  fi
  
  RESULT=$(curl -s -X POST "${API}/v1/payment/create" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${API_KEY}" \
    -H "X-Client-Id: ${CLIENT_ID}" \
    -d "{
      \"referenceId\": \"${REF}\",
      \"paymentMethod\": {\"type\": \"QRIS\", \"code\": \"MPM\"},
      \"amount\": 10000,
      \"customer\": {\"name\": \"John Doe\"},
      \"description\": \"Test Pakailink ${LABEL}\"
    }")
  
  SUCCESS=$(echo "$RESULT" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('success'))")
  MSG=$(echo "$RESULT" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('message',''))")
  ERR=$(echo "$RESULT" | python3 -c "import json,sys; d=json.load(sys.stdin); e=d.get('error') or {}; print(e.get('message',''))")
  
  echo "success: $SUCCESS | message: $MSG | error: $ERR"
  
  if [ "$SUCCESS" = "True" ]; then
    QR=$(echo "$RESULT" | python3 -c "
import json,sys
d=json.load(sys.stdin)
data=d.get('data') or {}
detail=data.get('paymentDetail') or {}
if isinstance(detail,str):
    try: detail=json.loads(detail)
    except: detail={}
print(detail.get('qrString','NO QR') if isinstance(detail,dict) else 'NO QR')
")
    echo "QR: ${QR:0:80}..."
  fi
}

# Current terminal = PDN0000006218 (merchantId)
echo "Current PAKAILINK_TERMINAL_ID: $(grep PAKAILINK_TERMINAL_ID /proc/1/environ 2>/dev/null | cut -d= -f2 || echo 'check docker')"

test_pakailink "PDN0000006218" "merchantId"

echo ""
echo "Check logs for actual Pakailink error:"
docker logs gtd-api 2>&1 | grep "pakailink.*error\|provider error" | tail -3
