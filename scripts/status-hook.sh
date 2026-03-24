#!/bin/bash
# Claude Code CLI hook script for tmux-overseer status detection
#
# Called by Claude Code hooks on agent lifecycle events. Receives JSON context
# via stdin and writes:
#   1. Status file:  ~/.claude-tmux/status-{pane}.json      (polled by TUI)
#   2. Event log:    ~/.claude-tmux/status-{pane}.events.jsonl  (preview pane)
#   3. Subagent list: ~/.claude-tmux/status-{pane}.subagents.json
#   4. Counters:     ~/.claude-tmux/status-{pane}.counters
#
# Event-specific input fields:
#   SessionStart:         session_id, cwd, model, permission_mode
#   UserPromptSubmit:     prompt
#   PreToolUse:           tool_name, tool_input
#   PostToolUse:          tool_name, tool_output
#   PostToolUseFailure:   tool_name, error
#   SubagentStart:        agent_id, agent_type, description, model
#   SubagentStop:         agent_id
#   Stop:                 stop_hook_reason
#   SessionEnd:           session_id
#   Notification:         message
#   PreCompact:           (no extra fields)
#
# Install: Run scripts/setup-hooks.sh

STATUS_DIR="${HOME}/.claude-tmux"
mkdir -p "$STATUS_DIR" 2>/dev/null

INPUT=$(cat)

# Single jq call to extract all fields at once (including tool_input summary)
eval "$(echo "$INPUT" | jq -r '
  @sh "SESSION_ID=\(.session_id // "")",
  @sh "HOOK_EVENT=\(.hook_event_name // "")",
  @sh "MODEL=\(.model // "")",
  @sh "CWD=\(.cwd // "")",
  @sh "TRANSCRIPT_PATH=\(.transcript_path // "")",
  @sh "PERMISSION_MODE=\(.permission_mode // "")",
  @sh "TOOL_NAME=\(.tool_name // "")",
  @sh "STOP_REASON=\(.stop_hook_reason // "")",
  @sh "PROMPT_TEXT=\((.prompt // "")[:200])",
  @sh "AGENT_ID=\(.agent_id // "")",
  @sh "AGENT_TYPE=\(.agent_type // "")",
  @sh "PARENT_AGENT_ID=\(.parent_agent_id // "")",
  @sh "SUBAGENT_DESC=\(.description // "")",
  @sh "LAST_ASSISTANT_MSG=\((.last_assistant_message // "")[:200])",
  @sh "EFFORT_LEVEL=\(.effort // "")",
  @sh "WORKTREE_NAME=\(.worktree.name // "")",
  @sh "WORKTREE_PATH=\(.worktree.path // "")",
  @sh "WORKTREE_BRANCH=\(.worktree.branch // "")",
  @sh "ORIGINAL_REPO=\(.worktree.originalRepo // "")",
  @sh "TOOL_INPUT_SUMMARY=\(
    .tool_input // {} |
    if .command then .command[:120]
    elif .path then .path
    elif .pattern then .pattern[:80]
    elif .query then .query[:80]
    elif .search_term then .search_term[:80]
    elif .glob_pattern then .glob_pattern
    elif .url then .url[:80]
    elif .description then .description[:80]
    elif .prompt then .prompt[:80]
    elif .todos then (.todos[0].content // "")[:80]
    else (keys[:2] | join(","))
    end
  )",
  @sh "TOOL_OUTPUT_SUMMARY=\(
    (.tool_output // "") | tostring |
    if length > 300 then .[:300] + "..." else . end
  )",
  @sh "TOOL_ERROR=\(
    (.error // "") | tostring |
    if length > 200 then .[:200] + "..." else . end
  )",
  @sh "NOTIF_MSG=\((.message // "")[:200])"
' 2>/dev/null)" 2>/dev/null

# Use TMUX_PANE as primary key, fall back to session_id
PANE_ID="${TMUX_PANE}"
if [ -z "$PANE_ID" ]; then
    if [ -n "$SESSION_ID" ]; then
        PANE_ID="session-$SESSION_ID"
    else
        exit 0
    fi
fi

SAFE_PANE_ID=$(echo "$PANE_ID" | tr -c 'a-zA-Z0-9_-' '_')
STATUS_FILE="$STATUS_DIR/status-${SAFE_PANE_ID}.json"
EVENTS_FILE="$STATUS_DIR/status-${SAFE_PANE_ID}.events.jsonl"
COUNTERS_FILE="$STATUS_DIR/status-${SAFE_PANE_ID}.counters"
SUBAGENT_LIST_FILE="$STATUS_DIR/status-${SAFE_PANE_ID}.subagents.json"

TS=$(date "+%H:%M:%S")

# --- Load persistent counters ---
if [ -f "$COUNTERS_FILE" ]; then
    # shellcheck disable=SC1090
    source "$COUNTERS_FILE" 2>/dev/null
fi
PROMPT_COUNT="${PROMPT_COUNT:-0}"
TOOL_COUNT="${TOOL_COUNT:-0}"
SESSION_START_TS="${SESSION_START_TS:-$(date +%s)}"
AGENT_MODE="${AGENT_MODE:-agent}"

# --- Subagent count from cached counter (updated only on SubagentStart/Stop) ---
SUBAGENT_COUNT="${SUBAGENT_COUNT:-0}"

# --- JSON escape helper (uses builtins, no subshell needed via _JE variable) ---
json_escape_to() {
    # Sets _JE to the escaped value (avoids subshell overhead of $(...))
    _JE="$1"
    _JE="${_JE//\\/\\\\}"
    _JE="${_JE//\"/\\\"}"
    _JE="${_JE//$'\n'/\\n}"
    _JE="${_JE//$'\t'/\\t}"
}

# --- Cost tracking from transcript ---
COST="0"
if [ -n "$TRANSCRIPT_PATH" ] && [ -f "$TRANSCRIPT_PATH" ] && [ "$HOOK_EVENT" = "Stop" ]; then
    eval "$(jq -s '
      reduce .[].usage as $u ({i:0,o:0};
        .i += ($u.input_tokens // 0) | .o += ($u.output_tokens // 0)
      ) | @sh "TOTAL_INPUT_TOKENS=\(.i)", @sh "TOTAL_OUTPUT_TOKENS=\(.o)"
    ' "$TRANSCRIPT_PATH" 2>/dev/null)" 2>/dev/null
    TOTAL_OUTPUT_TOKENS="${TOTAL_OUTPUT_TOKENS:-0}"
    TOTAL_INPUT_TOKENS="${TOTAL_INPUT_TOKENS:-0}"
    if [ "$TOTAL_OUTPUT_TOKENS" != "0" ]; then
        COST=$(echo "scale=4; ($TOTAL_INPUT_TOKENS * 0.000015) + ($TOTAL_OUTPUT_TOKENS * 0.000075)" | bc 2>/dev/null || echo "0")
    fi
fi

# --- Map event to status + build JSONL event ---
EVENT_JSON=""
STATUS=""

case "$HOOK_EVENT" in
    SessionEnd)
        rm -f "$STATUS_FILE" "$EVENTS_FILE" "$COUNTERS_FILE" "$SUBAGENT_LIST_FILE" 2>/dev/null
        exit 0
        ;;
    SessionStart)
        STATUS="idle"
        SESSION_START_TS=$(date +%s)
        PROMPT_COUNT=0
        TOOL_COUNT=0
        echo "[]" > "$SUBAGENT_LIST_FILE"
        : > "$EVENTS_FILE"
        EVENT_JSON="{\"ts\":\"$TS\",\"type\":\"session_start\"}"
        ;;
    UserPromptSubmit)
        STATUS="working"
        PROMPT_COUNT=$((PROMPT_COUNT + 1))
        json_escape_to "$PROMPT_TEXT"
        EVENT_JSON="{\"ts\":\"$TS\",\"type\":\"prompt\",\"text\":\"$_JE\"}"
        ;;
    PreToolUse)
        TOOL_COUNT=$((TOOL_COUNT + 1))
        if [ "$TOOL_NAME" = "AskQuestion" ]; then
            STATUS="waiting"
        else
            STATUS="working"
        fi
        json_escape_to "$TOOL_NAME"; _TOOL="$_JE"
        json_escape_to "$TOOL_INPUT_SUMMARY"; _INPUT="$_JE"
        EVENT_JSON="{\"ts\":\"$TS\",\"type\":\"tool_start\",\"tool\":\"$_TOOL\",\"input\":\"$_INPUT\"}"
        ;;
    PostToolUse)
        STATUS="working"
        json_escape_to "$TOOL_NAME"; _TOOL="$_JE"
        json_escape_to "$TOOL_OUTPUT_SUMMARY"; _OUTPUT="$_JE"
        EVENT_JSON="{\"ts\":\"$TS\",\"type\":\"tool_result\",\"tool\":\"$_TOOL\",\"output\":\"$_OUTPUT\"}"
        ;;
    PostToolUseFailure)
        STATUS="working"
        json_escape_to "$TOOL_NAME"; _TOOL="$_JE"
        json_escape_to "$TOOL_ERROR"; _ERR="$_JE"
        EVENT_JSON="{\"ts\":\"$TS\",\"type\":\"tool_error\",\"tool\":\"$_TOOL\",\"error\":\"$_ERR\"}"
        ;;
    SubagentStart)
        STATUS="working"
        SA_TYPE="$AGENT_TYPE"
        if [ -z "$SA_TYPE" ]; then
            SA_TYPE="general"
            case "$SUBAGENT_DESC" in
                *[Ee]xplor*) SA_TYPE="explore" ;;
                *[Ss]hell*|*[Bb]ash*|*[Cc]ommand*) SA_TYPE="shell" ;;
                *[Bb]rows*) SA_TYPE="browser" ;;
                *[Rr]eview*) SA_TYPE="code-reviewer" ;;
                *[Ss]implif*) SA_TYPE="code-simplifier" ;;
                *[Pp]lan*) SA_TYPE="plan" ;;
                *[Dd]ebug*) SA_TYPE="debug" ;;
                *[Ss]earch*|*[Ff]ind*) SA_TYPE="explore" ;;
                *[Tt]est*) SA_TYPE="test" ;;
            esac
        fi
        EXISTING="[]"
        [ -f "$SUBAGENT_LIST_FILE" ] && EXISTING=$(cat "$SUBAGENT_LIST_FILE" 2>/dev/null || echo "[]")
        echo "$EXISTING" | jq --arg id "$AGENT_ID" --arg type "$SA_TYPE" --arg desc "$SUBAGENT_DESC" \
            --arg model "$MODEL" --arg ts "$TS" --arg parent "$PARENT_AGENT_ID" \
            '. + [{"id":$id,"agent_type":$type,"description":$desc,"model":$model,"started_at":$ts,"status":"working","parent_agent_id":$parent}]' \
            > "$SUBAGENT_LIST_FILE" 2>/dev/null
        SUBAGENT_COUNT=$(jq 'length' "$SUBAGENT_LIST_FILE" 2>/dev/null || echo "0")
        json_escape_to "$SUBAGENT_DESC"; _DESC="$_JE"
        json_escape_to "$MODEL"; _MODEL="$_JE"
        EVENT_JSON="{\"ts\":\"$TS\",\"type\":\"subagent_start\",\"description\":\"$_DESC\",\"model\":\"$_MODEL\",\"tool\":\"$SA_TYPE\"}"
        ;;
    SubagentStop)
        STATUS="working"
        if [ -f "$SUBAGENT_LIST_FILE" ]; then
            if [ -n "$AGENT_ID" ]; then
                jq --arg id "$AGENT_ID" '[.[] | select(.id != $id)]' "$SUBAGENT_LIST_FILE" \
                    > "$SUBAGENT_LIST_FILE.tmp" 2>/dev/null \
                    && mv "$SUBAGENT_LIST_FILE.tmp" "$SUBAGENT_LIST_FILE"
            else
                jq '.[:-1]' "$SUBAGENT_LIST_FILE" > "$SUBAGENT_LIST_FILE.tmp" 2>/dev/null \
                    && mv "$SUBAGENT_LIST_FILE.tmp" "$SUBAGENT_LIST_FILE"
            fi
            SUBAGENT_COUNT=$(jq 'length' "$SUBAGENT_LIST_FILE" 2>/dev/null || echo "0")
        fi
        json_escape_to "$LAST_ASSISTANT_MSG"; _MSG="$_JE"
        EVENT_JSON="{\"ts\":\"$TS\",\"type\":\"subagent_stop\",\"tool\":\"$AGENT_TYPE\",\"summary\":\"$_MSG\"}"
        ;;
    Stop)
        STATUS="idle"
        json_escape_to "$LAST_ASSISTANT_MSG"; _MSG="$_JE"
        EVENT_JSON="{\"ts\":\"$TS\",\"type\":\"stop\",\"reason\":\"$STOP_REASON\",\"last_message\":\"$_MSG\"}"
        ;;
    Notification)
        STATUS="waiting"
        json_escape_to "$NOTIF_MSG"
        EVENT_JSON="{\"ts\":\"$TS\",\"type\":\"notification\",\"text\":\"$_JE\"}"
        ;;
    PreCompact)
        STATUS="working"
        EVENT_JSON="{\"ts\":\"$TS\",\"type\":\"compact\"}"
        ;;
    TaskCompleted)
        STATUS="idle"
        EVENT_JSON="{\"ts\":\"$TS\",\"type\":\"task_completed\"}"
        ;;
    TeammateIdle)
        STATUS="idle"
        EVENT_JSON="{\"ts\":\"$TS\",\"type\":\"teammate_idle\"}"
        ;;
    InstructionsLoaded)
        # Fire when CLAUDE.md or .claude/rules files are loaded; emit to event log only.
        STATUS=""
        EVENT_JSON="{\"ts\":\"$TS\",\"type\":\"rules_loaded\"}"
        ;;
    *)
        exit 0
        ;;
esac

# --- Tag events with agent context when running inside a subagent ---
if [ -n "$AGENT_ID" ] && [ -n "$EVENT_JSON" ]; then
    json_escape_to "$AGENT_ID"; _AID="$_JE"
    json_escape_to "$AGENT_TYPE"; _ATYPE="$_JE"
    EVENT_JSON="${EVENT_JSON%\}},\"agent_id\":\"$_AID\",\"agent_type\":\"$_ATYPE\"}"
fi

# --- Derive agent_mode from permission_mode ---
case "$PERMISSION_MODE" in
    plan) AGENT_MODE="plan" ;;
    *)    AGENT_MODE="agent" ;;
esac

# --- Save counters ---
cat > "$COUNTERS_FILE" << EOF
PROMPT_COUNT=$PROMPT_COUNT
TOOL_COUNT=$TOOL_COUNT
SESSION_START_TS=$SESSION_START_TS
AGENT_MODE=$AGENT_MODE
SUBAGENT_COUNT=$SUBAGENT_COUNT
EOF

# --- Write status JSON (skip for events that only emit log entries) ---
if [ -n "$STATUS" ]; then
    jq -n \
        --arg pane_id "$PANE_ID" \
        --arg session_id "$SESSION_ID" \
        --arg status "$STATUS" \
        --arg event "$HOOK_EVENT" \
        --arg model "$MODEL" \
        --arg cwd "$CWD" \
        --arg permission_mode "$PERMISSION_MODE" \
        --arg agent_mode "$AGENT_MODE" \
        --arg last_tool "$TOOL_NAME" \
        --arg worktree_path "${WORKTREE_PATH:-}" \
        --arg worktree_branch "${WORKTREE_BRANCH:-}" \
        --arg original_repo "${ORIGINAL_REPO:-}" \
        --arg effort_level "${EFFORT_LEVEL:-}" \
        --argjson cost "${COST:-0}" \
        --argjson prompt_count "$PROMPT_COUNT" \
        --argjson tool_count "$TOOL_COUNT" \
        --argjson session_start_ts "$SESSION_START_TS" \
        --argjson subagent_count "$SUBAGENT_COUNT" \
        --argjson timestamp "$(date +%s)" \
        '{pane_id:$pane_id,session_id:$session_id,status:$status,event:$event,cost:$cost,model:$model,cwd:$cwd,permission_mode:$permission_mode,agent_mode:$agent_mode,last_tool:$last_tool,worktree_path:$worktree_path,worktree_branch:$worktree_branch,original_repo:$original_repo,effort_level:$effort_level,prompt_count:$prompt_count,tool_count:$tool_count,session_start_ts:$session_start_ts,subagent_count:$subagent_count,timestamp:$timestamp}' \
        > "$STATUS_FILE" 2>/dev/null
fi

# --- Append JSONL event (for rich preview) ---
if [ -n "$EVENT_JSON" ]; then
    echo "$EVENT_JSON" >> "$EVENTS_FILE" 2>/dev/null
    if [ -f "$EVENTS_FILE" ]; then
        LINE_COUNT=$(wc -l < "$EVENTS_FILE" 2>/dev/null || echo "0")
        if [ "$LINE_COUNT" -gt 200 ]; then
            TAIL=$(tail -200 "$EVENTS_FILE")
            echo "$TAIL" > "$EVENTS_FILE" 2>/dev/null
        fi
    fi
fi

# --- Forward to running overseer TUI for instant updates (fire-and-forget) ---
_PORT_FILE="$STATUS_DIR/.hookserver-port"
if [ -f "$_PORT_FILE" ]; then
    _PORT=$(cat "$_PORT_FILE" 2>/dev/null)
    if [ -n "$_PORT" ] && [ "$_PORT" -gt 0 ]; then
        ( curl -sf --max-time 1 -X POST -H "Content-Type: application/json" \
            -d "$INPUT" "http://127.0.0.1:${_PORT}/hook" >/dev/null 2>&1 & ) 2>/dev/null
    fi
fi

# --- Periodic cleanup (only on Stop events to reduce I/O) ---
if [ "$HOOK_EVENT" = "Stop" ]; then
    find "$STATUS_DIR" -name "status-*.json" -mmin +60 -delete 2>/dev/null
    find "$STATUS_DIR" -name "status-*.events.jsonl" -mmin +60 -delete 2>/dev/null
    find "$STATUS_DIR" -name "status-*.counters" -mmin +60 -delete 2>/dev/null
    find "$STATUS_DIR" -name "status-*.subagents.json" -mmin +60 -delete 2>/dev/null
fi

exit 0
