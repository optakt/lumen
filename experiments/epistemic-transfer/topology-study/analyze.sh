#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

go run ../../../cmd/analyze-transfer-study \
	-episodes episodes \
	-results results.jsonl \
	-providers providers.json \
	-static-results ../../substrate-drift/results.jsonl
