#!/bin/bash
set -e

START_TIME=$(date +%s)
echo "{\"severity\":\"INFO\",\"body\":\"cronjob started\",\"attributes\":{\"k8s.cronjob.name\":\"product-analytics-clone-keycloak\"},\"timestamp\":\"$(date -Iseconds)\"}"

xata init --organization "$XATA_ORGANIZATIONID" --project "$XATA_PROJECTID" --database "$XATA_DATABASENAME" --branch "$XATA_BRANCHID"
xata status --json

TARGET_URL="$(xata branch url)"

MV_SQL=$(cat <<'SQL'
SELECT
    MIN(id) AS id,
    group_id,
    MAX(value) FILTER (WHERE name = 'displayName')     AS "displayName",
    MAX(value) FILTER (WHERE name = 'billingStatus')   AS "billingStatus",
    MAX(value) FILTER (WHERE name = 'disabledByAdmin') AS "disabledByAdmin",
    MAX(value) FILTER (WHERE name = 'billingReason')   AS "billingReason",
    MAX(value::timestamptz) FILTER (WHERE name = 'lastUpdated') AS "lastUpdated"
FROM public.group_attribute
GROUP BY group_id
SQL
)

echo "{\"severity\":\"INFO\",\"body\":\"dropping materialized view\",\"attributes\":{\"k8s.cronjob.name\":\"product-analytics-clone-keycloak\"},\"timestamp\":\"$(date -Iseconds)\"}"
psql "$TARGET_URL" -c "DROP MATERIALIZED VIEW IF EXISTS public.group_attribute_mv;"

echo "{\"severity\":\"INFO\",\"body\":\"starting clone\",\"attributes\":{\"k8s.cronjob.name\":\"product-analytics-clone-keycloak\"},\"timestamp\":\"$(date -Iseconds)\"}"

if xata clone start \
  --source-url "$XATA_CLI_SOURCE_POSTGRES_URL" \
  --validation-mode=relaxed \
  --filter-tables="public.keycloak_group public.group_attribute public.offline_client_session public.org public.org_domain public.user_entity public.user_group_membership"; then

  echo "{\"severity\":\"INFO\",\"body\":\"recreating materialized view\",\"attributes\":{\"k8s.cronjob.name\":\"product-analytics-clone-keycloak\"},\"timestamp\":\"$(date -Iseconds)\"}"
  psql "$TARGET_URL" <<MV
CREATE MATERIALIZED VIEW public.group_attribute_mv AS $MV_SQL;
MV

  END_TIME=$(date +%s)
  DURATION=$((END_TIME - START_TIME))
  echo "{\"severity\":\"INFO\",\"body\":\"cronjob completed successfully\",\"attributes\":{\"k8s.cronjob.name\":\"product-analytics-clone-keycloak\",\"duration_ms\":$((DURATION * 1000))},\"timestamp\":\"$(date -Iseconds)\"}"
else
  EXIT_CODE=$?
  END_TIME=$(date +%s)
  DURATION=$((END_TIME - START_TIME))
  echo "{\"severity\":\"ERROR\",\"body\":\"cronjob failed\",\"attributes\":{\"k8s.cronjob.name\":\"product-analytics-clone-keycloak\",\"error.code\":$EXIT_CODE,\"duration_ms\":$((DURATION * 1000))},\"timestamp\":\"$(date -Iseconds)\"}"
  exit $EXIT_CODE
fi
