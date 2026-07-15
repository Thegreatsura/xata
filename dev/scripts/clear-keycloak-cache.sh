#!/bin/bash
set -e

# Usage message
if [ "$1" == "-h" ] || [ "$1" == "--help" ]; then
  echo "Usage: $0 [REALM_NAME] [NAMESPACE]"
  echo "Clears the user cache for the specified Keycloak realm."
  echo "Defaults: REALM_NAME=xata, NAMESPACE=xata"
  exit 0
fi

REALM="${1:-xata}"
NAMESPACE="${2:-xata}"
KEYCLOAK_URL="http://localhost:8080"

echo "Fetching admin credentials from secret 'auth-keycloak-initial-admin' in namespace '${NAMESPACE}'..."
ADMIN_USER=$(kubectl get secret auth-keycloak-initial-admin -n "${NAMESPACE}" -o jsonpath='{.data.username}' | base64 --decode)
ADMIN_PASS=$(kubectl get secret auth-keycloak-initial-admin -n "${NAMESPACE}" -o jsonpath='{.data.password}' | base64 --decode)

if [ -z "$ADMIN_USER" ] || [ -z "$ADMIN_PASS" ]; then
    echo "Error: Failed to retrieve Keycloak admin credentials from Kubernetes."
    exit 1
fi

echo "Authenticating via ${KEYCLOAK_URL}..."
# Get an access token using the admin-cli client
TOKEN_RESPONSE=$(curl -s -X POST "${KEYCLOAK_URL}/realms/master/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "username=${ADMIN_USER}" \
  -d "password=${ADMIN_PASS}" \
  -d "grant_type=password" \
  -d "client_id=admin-cli")

TOKEN=$(echo "$TOKEN_RESPONSE" | sed -n 's|.*"access_token":"\([^"]*\)".*|\1|p')

if [ -z "$TOKEN" ]; then
    echo "Error: Failed to fetch Keycloak access token. Response:"
    echo "$TOKEN_RESPONSE"
    exit 1
fi

echo "Clearing user cache for realm '${REALM}'..."
# Trigger the clear user cache endpoint
RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${KEYCLOAK_URL}/admin/realms/${REALM}/clear-user-cache" \
  -H "Authorization: Bearer ${TOKEN}")

if [ "$RESPONSE" == "204" ] || [ "$RESPONSE" == "200" ]; then
    echo "Success! Cache cleared for realm '${REALM}' (HTTP ${RESPONSE})."
else
    echo "Failed to clear cache. HTTP Status: ${RESPONSE}"
    # Print the actual error output for debugging
    curl -s -X POST "${KEYCLOAK_URL}/admin/realms/${REALM}/clear-user-cache" \
      -H "Authorization: Bearer ${TOKEN}"
    echo ""
    exit 1
fi
