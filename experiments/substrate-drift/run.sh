#!/bin/bash
set -euo pipefail

ANTHROPIC_API_KEY=$(optakt secret ANTHROPIC_API_KEY) \
OPENAI_API_KEY=$(optakt secret OPENAI_API_KEY) \
XAI_API_KEY=$(optakt secret XAI_API_KEY) \
DEEPSEEK_API_KEY=$(optakt secret DEEPSEEK_API_KEY) \
MOONSHOT_API_KEY=$(optakt secret MOONSHOT_API_KEY) \
ZAI_API_KEY=$(optakt secret ZAI_API_KEY) \
DASHSCOPE_API_KEY=$(optakt secret DASHSCOPE_API_KEY) \
GEMINI_API_KEY=$(optakt secret GEMINI_API_KEY) \
MINIMAX_API_KEY=$(optakt secret MINIMAX_API_KEY) \
  go run ../../cmd/calibrate-drift/main.go \
    -probes probes.lm \
    -config providers.json \
    -db drift.db \
    -out results.jsonl \
    -runs "${1:-3}" \
    -seed "${2:-42}"
