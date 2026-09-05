#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$ROOT/tmp/integration"
CONTAINER="workmail-mcp-greenmail"
IMAGE="greenmail/standalone:2.1.13"
BIN="$TMP/workmail-mcp"

cleanup() {
  docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
  rm -rf "$TMP"
}
trap cleanup EXIT

rm -rf "$TMP"
mkdir -p "$TMP"

cat > "$TMP/server.cnf" <<'EOF'
[req]
distinguished_name = dn
prompt = no

[dn]
CN = localhost

[v3_req]
subjectAltName = @alt_names
basicConstraints = critical,CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth

[alt_names]
DNS.1 = localhost
IP.1 = 127.0.0.1
EOF

openssl req -x509 -newkey rsa:2048 -nodes -days 1 \
  -keyout "$TMP/ca.key" \
  -out "$TMP/ca.crt" \
  -subj "/CN=workmail-mcp integration CA" \
  -addext "basicConstraints=critical,CA:TRUE" \
  -addext "keyUsage=critical,keyCertSign,cRLSign" \
  >/dev/null 2>&1

openssl req -newkey rsa:2048 -nodes \
  -keyout "$TMP/server.key" \
  -out "$TMP/server.csr" \
  -config "$TMP/server.cnf" \
  >/dev/null 2>&1

openssl x509 -req \
  -in "$TMP/server.csr" \
  -CA "$TMP/ca.crt" \
  -CAkey "$TMP/ca.key" \
  -CAcreateserial \
  -days 1 \
  -out "$TMP/server.crt" \
  -extfile "$TMP/server.cnf" \
  -extensions v3_req \
  >/dev/null 2>&1

openssl pkcs12 -export \
  -out "$TMP/greenmail.p12" \
  -inkey "$TMP/server.key" \
  -in "$TMP/server.crt" \
  -certfile "$TMP/ca.crt" \
  -name greenmail \
  -passout pass:changeit \
  >/dev/null 2>&1
chmod 0644 "$TMP/greenmail.p12"

docker pull "$IMAGE" >/dev/null

docker run -d --rm \
  --name "$CONTAINER" \
  -p 3025:3025 \
  -p 3993:3993 \
  -v "$TMP/greenmail.p12:/home/greenmail/greenmail.p12:ro" \
  -e 'GREENMAIL_OPTS=-Dgreenmail.setup.test.smtp -Dgreenmail.setup.test.imaps -Dgreenmail.hostname=0.0.0.0 -Dgreenmail.tls.keystore.file=/home/greenmail/greenmail.p12 -Dgreenmail.tls.keystore.password=changeit -Dgreenmail.users=integration:integration-secret@example.com' \
  "$IMAGE" >/dev/null

python3 - <<'PY'
import socket
import time

for port in (3025, 3993):
    for _ in range(60):
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=1):
                break
        except OSError:
            time.sleep(0.5)
    else:
        raise SystemExit(f"GreenMail port {port} did not become ready")
PY

python3 "$ROOT/integration/seed_mail.py"

go build -ldflags "-X main.version=integration" -o "$BIN" ./cmd/workmail

export WORKMAIL_IMAP_HOST=localhost
export WORKMAIL_IMAP_PORT=3993
export WORKMAIL_IMAP_USERNAME=integration
export WORKMAIL_IMAP_PASSWORD=integration-secret
export WORKMAIL_DEFAULT_FOLDER=INBOX
export WORKMAIL_OPERATION_TIMEOUT=10s
export SSL_CERT_FILE="$TMP/ca.crt"

doctor_output="$($BIN doctor --latest-subject)"
printf '%s\n' "$doctor_output"
printf '%s\n' "$doctor_output" | grep -F "OK: IMAPS TLS, authentication, and folder listing succeeded" >/dev/null
printf '%s\n' "$doctor_output" | grep -F "OK: latest subject in INBOX" >/dev/null
printf '%s\n' "$doctor_output" | grep -F "Re: Integration thread" >/dev/null

python3 "$ROOT/integration/mcp_smoke.py" "$BIN"

echo "OK: GreenMail IMAPS integration test passed"
