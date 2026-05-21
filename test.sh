#!/usr/bin/env bash
#
# test.sh — Send a test request mirroring the exact OpenAI-compatible
# chat completion request that vaultsort sends to organize a file.
#
# Usage:
#   Edit the env vars at the top, or set VAULTSORT_LLM_* env vars.
#
# Environment variables (match config.example.toml / vaultsort env vars):
#   VAULTSORT_LLM_API_KEY     - API key (required)
#   VAULTSORT_LLM_BASE_URL    - API base URL (default: https://api.openai.com/v1)
#   VAULTSORT_LLM_MODEL       - Model name (default: gpt-4o-mini)
#   VAULTSORT_LLM_TEMPERATURE - Temperature (default: 0.1)
#   VAULTSORT_LLM_TIMEOUT     - HTTP timeout in seconds (default: 30)
#   VAULTSORT_TEST_FILE       - Path to a file to attach (optional)
#
# Examples:
#   # Text-only request
#   ./test.sh
#
#   # Attach a file (mimics send_file = true in a rule)
#   VAULTSORT_TEST_FILE=~/Downloads/report.pdf ./test.sh

# ── Configuration ─────────────────────────────────────────────────────────────
echo $VAULTSORT_LLM_API_KEY
API_KEY="${VAULTSORT_LLM_API_KEY:-}"
BASE_URL="${VAULTSORT_LLM_BASE_URL:-https://api.openai.com/v1}"
MODEL="${VAULTSORT_LLM_MODEL:-gpt-4o-mini}"
TEMPERATURE="${VAULTSORT_LLM_TEMPERATURE:-0.1}"
TIMEOUT="${VAULTSORT_LLM_TIMEOUT:-30}"
FILE_PATH="${VAULTSORT_TEST_FILE:-}"

set -euo pipefail

if [ -z "${API_KEY}" ]; then
  echo "Error: VAULTSORT_LLM_API_KEY is required" >&2
  exit 1
fi

ENDPOINT="${BASE_URL}/chat/completions"

# ── Build messages ────────────────────────────────────────────────────────────
# Mirrors BuildPrompt() from prompt.go and toRawMessages() from client.go.

SYSTEM_MSG='{"role":"system","content":"You are a file organization assistant. Respond only with valid JSON."}'

PROMPT_TEXT="You are a file organization assistant. Analyze this file and determine:
1. A clean, descriptive filename (with extension)
2. A subdirectory path under ~/Vault where it should be stored."

if [ -n "${FILE_PATH}" ]; then
  FILENAME=$(basename "${FILE_PATH}")

  if [ ! -f "${FILE_PATH}" ]; then
    echo "Error: file not found: ${FILE_PATH}" >&2
    exit 1
  fi

  FILE_SIZE=$(wc -c < "${FILE_PATH}" | tr -d ' ')

  if [ "${FILE_SIZE}" -gt 10240 ]; then
    # Same 10KB limit as vaultsort (prompt.go: BuildPrompt)
    FILE_B64=$(head -c 10240 "${FILE_PATH}" | base64 | tr -d '\n')
    echo "  File:        ${FILENAME} (truncated to 10KB)"
  else
    FILE_B64=$(base64 < "${FILE_PATH}" | tr -d '\n')
    echo "  File:        ${FILENAME} (${FILE_SIZE} bytes)"
  fi

  # Detect MIME type — same approach as prompt.go: detectMIME
  MIME_TYPE=$(file --mime-type -b "${FILE_PATH}")

  # Format matches client.go: FilePart → "base64:{mime};{base64data}"
  FILE_DATA="base64:${MIME_TYPE};${FILE_B64}"

  PROMPT_TEXT="${PROMPT_TEXT}

File: ${FILENAME}
Contents (excerpt):
(file attached as part)

Respond in JSON format:
{\"filename\": \"descriptive_name.ext\", \"subdir\": \"some/subdir\"}"

  # Build multipart user message — matches toRawMessages() output
  USER_MSG=$(jq -n \
    --arg prompt "${PROMPT_TEXT}" \
    --arg filename "${FILENAME}" \
    --arg file_data "${FILE_DATA}" \
    '{
      role: "user",
      content: [
        {type: "text", text: $prompt},
        {type: "input_file", filename: $filename, file_data: $file_data}
      ]
    }')
else
  FILENAME="quarterly_report_draft.pdf"

  PROMPT_TEXT="${PROMPT_TEXT}

File: ${FILENAME}
Contents (excerpt):
(file attached as part)

Respond in JSON format:
{\"filename\": \"descriptive_name.ext\", \"subdir\": \"some/subdir\"}"

  USER_MSG=$(jq -n \
    --arg content "${PROMPT_TEXT}" \
    '{role: "user", content: $content}')
fi

# ── Build full request body ───────────────────────────────────────────────────
# Matches client.go: ChatRequest with jsonResponseFormat()
BODY=$(jq -n \
  --arg model "${MODEL}" \
  --argjson system_msg "${SYSTEM_MSG}" \
  --argjson user_msg "${USER_MSG}" \
  --argjson temperature "${TEMPERATURE}" \
  '{
    model: $model,
    messages: [$system_msg, $user_msg],
    temperature: $temperature,
    max_tokens: 1024,
    response_format: {
      type: "json_schema",
      json_schema: {
        name: "file_organization",
        schema: {
          type: "object",
          properties: {
            filename: {type: "string"},
            subdir: {type: "string"}
          },
          required: ["filename", "subdir"],
          additionalProperties: false
        },
        strict: true
      }
    }
  }')

echo "→ POST ${ENDPOINT}"
echo "  Model:       ${MODEL}"
echo "  Temperature: ${TEMPERATURE}"
echo ""

HTTP_RESPONSE=$(curl -s -w "\n%{http_code}" \
  --max-time "${TIMEOUT}" \
  -X POST "${ENDPOINT}" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${API_KEY}" \
  -d "${BODY}")

HTTP_CODE=$(echo "${HTTP_RESPONSE}" | tail -1)
RESPONSE_BODY=$(echo "${HTTP_RESPONSE}" | sed '$d')

if [ "${HTTP_CODE}" = "200" ]; then
  echo "✓ HTTP ${HTTP_CODE}"
  echo ""
  echo "${RESPONSE_BODY}" | python3 -m json.tool 2>/dev/null || echo "${RESPONSE_BODY}"
else
  echo "✗ HTTP ${HTTP_CODE}"
  echo ""
  echo "${RESPONSE_BODY}" | python3 -m json.tool 2>/dev/null || echo "${RESPONSE_BODY}"
  exit 1
fi
