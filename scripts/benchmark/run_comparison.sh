#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
RESULTS_DIR="$REPO_ROOT/results"
PROMPTS="$SCRIPT_DIR/tier0_prompts.json"
TIMESTAMP=$(date +%Y%m%d_%H%M)

mkdir -p "$RESULTS_DIR"

echo "=== Running T0 on Qwen 3.6 27B (vLLM, port 8001) ==="
go run "$REPO_ROOT/cmd/benchmark/main.go" \
  --prompts "$PROMPTS" \
  --endpoint http://localhost:8001/v1 \
  --model qwen-3.6-27b \
  --temperature 1.0 \
  --system-first \
  --thinking \
  --output "$RESULTS_DIR/t0_qwen_${TIMESTAMP}.json" \
  --judge http://100.81.106.97:8090/v1 \
  --judge-model claude-sonnet-4-20250514

echo ""
echo "=== Running T0 on Gemma 4 26B (llama.cpp, port 8300) ==="
go run "$REPO_ROOT/cmd/benchmark/main.go" \
  --prompts "$PROMPTS" \
  --endpoint http://localhost:8300/v1 \
  --model gemma-26b \
  --temperature 1.0 \
  --output "$RESULTS_DIR/t0_gemma26b_${TIMESTAMP}.json" \
  --judge http://100.81.106.97:8090/v1 \
  --judge-model claude-sonnet-4-20250514

echo ""
echo "=== Comparison ==="
go run "$REPO_ROOT/cmd/benchmark/main.go" \
  --compare "$RESULTS_DIR/t0_qwen_${TIMESTAMP}.json,$RESULTS_DIR/t0_gemma26b_${TIMESTAMP}.json"
