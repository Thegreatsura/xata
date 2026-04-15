#!/bin/bash
set -e

START_TIME=$(date +%s)
echo "{\"severity\":\"INFO\",\"body\":\"cronjob started\",\"attributes\":{\"k8s.cronjob.name\":\"product-analytics-clone-metastore\"},\"timestamp\":\"$(date -Iseconds)\"}"

xata init --organization "$XATA_ORGANIZATIONID" --project "$XATA_PROJECTID" --database "$XATA_DATABASENAME" --branch "$XATA_BRANCHID"
xata status --json

echo "{\"severity\":\"INFO\",\"body\":\"starting clone\",\"attributes\":{\"k8s.cronjob.name\":\"product-analytics-clone-metastore\"},\"timestamp\":\"$(date -Iseconds)\"}"

if xata clone start \
  --source-url "$XATA_CLI_SOURCE_POSTGRES_URL" \
  --validation-mode=relaxed \
  --filter-tables="public.regions public.cells public.projects public.branches"; then
  END_TIME=$(date +%s)
  DURATION=$((END_TIME - START_TIME))
  echo "{\"severity\":\"INFO\",\"body\":\"cronjob completed successfully\",\"attributes\":{\"k8s.cronjob.name\":\"product-analytics-clone-metastore\",\"duration_ms\":$((DURATION * 1000))},\"timestamp\":\"$(date -Iseconds)\"}"
else
  EXIT_CODE=$?
  END_TIME=$(date +%s)
  DURATION=$((END_TIME - START_TIME))
  echo "{\"severity\":\"ERROR\",\"body\":\"cronjob failed\",\"attributes\":{\"k8s.cronjob.name\":\"product-analytics-clone-metastore\",\"error.code\":$EXIT_CODE,\"duration_ms\":$((DURATION * 1000))},\"timestamp\":\"$(date -Iseconds)\"}"
  exit $EXIT_CODE
fi
