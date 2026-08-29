#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

ANTHROPIC_API_KEY=$(optakt secret ANTHROPIC_API_KEY) \
OPENAI_API_KEY=$(optakt secret OPENAI_API_KEY) \
XAI_API_KEY=$(optakt secret XAI_API_KEY) \
DEEPSEEK_API_KEY=$(optakt secret DEEPSEEK_API_KEY) \
	go run ../../../cmd/epistemic-transfer \
	-mode run \
	-episodes episodes \
	-providers providers.json \
	-results results.jsonl \
	-runs "${1:-2}" \
	-seed "${2:-73}"
