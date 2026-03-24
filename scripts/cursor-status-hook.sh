#!/bin/bash
# Cursor IDE hook script for tmux-overseer status detection
#
# Called by Cursor hooks on agent lifecycle events. Receives JSON context
# via stdin (same schema as Claude Code hooks) and writes:
#   1. Status file:  ~/.claude-tmux/cursor-{id}.json  (polled by TUI)
#   2. Event log:    ~/.claude-tmux/cursor-{id}.events.jsonl  (preview pane)
#   3. Legacy log:   ~/.claude-tmux/cursor-{id}.log  (backward compat)
#
# Common input fields (all events):
#   session_id, transcript_path, cwd, permission_mode, hook_event_name,
#   conversation_id, model, workspace_roots, cursor_version
#
# Event-specific fields:
#   beforeSubmitPrompt:         prompt, attachments
#   preToolUse/postToolUse:     tool_name, tool_input, tool_output (post only)
#   afterAgentResponse:         response text in message content
#   afterAgentThought:          thinking text in message content
#   afterFileEdit:              file_path, edits [{old_string, new_string}]
#   afterShellExecution:        command, output, exit_code
#   stop:                       stop_hook_reason
#   subagentStart:              description, model
#   subagentStop:               (no extra fields)
#
# Install: Run scripts/setup-cursor-hooks.sh

STATUS_DIR="${HOME}/.claude-tmux"
mkdir -p "$STATUS_DIR" 2>/dev/null

INPUT=$(cat)

# Single jq call to extract all common + event-specific fields at once.
# This avoids spawning 10+ jq subprocesses per hook invocation.
eval "$(echo "$INPUT" | jq -r '
  @sh "CONVERSATION_ID=\(.conversation_id // "")",
  @sh "MODEL=\(.model // "")",
  @sh "HOOK_EVENT=\(.hook_event_name // "")",
  @sh "CURSOR_VERSION=\(.cursor_version // "")",
  @sh "SESSION_ID=\(.session_id // "")",
  @sh "CWD=\(.cwd // "")",
  @sh "WORKSPACE=\(.workspace_roots[0] // "")",
  @sh "TOOL_NAME=\(.tool_name // "")",
  @sh "STOP_REASON=\(.stop_hook_reason // "")",
  @sh "PERMISSION_MODE=\(.permission_mode // "")",
  @sh "TRANSCRIPT_PATH=\(.transcript_path // "")",
  @sh "FULL_PROMPT=\((.prompt // "")[:2000])",
  @sh "AGENT_ID=\(.agent_id // "")",
  @sh "AGENT_TYPE=\(.agent_type // "")",
  @sh "PARENT_AGENT_ID=\(.parent_agent_id // "")",
  @sh "LAST_ASSISTANT_MSG=\((.last_assistant_message // "")[:200])",
  @sh "EFFORT_LEVEL=\(.effort // "")",
  @sh "WORKTREE_NAME=\(.worktree.name // "")",
  @sh "WORKTREE_PATH=\(.worktree.path // "")",
  @sh "WORKTREE_BRANCH=\(.worktree.branch // "")",
  @sh "ORIGINAL_REPO=\(.worktree.originalRepo // "")"
' 2>/dev/null)" 2>/dev/null

# Fallback conversation ID
if [ -z "$CONVERSATION_ID" ]; then
    CONVERSATION_ID="${SESSION_ID}"
    [ -z "$CONVERSATION_ID" ] && exit 0
fi

STATUS_FILE="$STATUS_DIR/cursor-${CONVERSATION_ID}.json"
EVENTS_FILE="$STATUS_DIR/cursor-${CONVERSATION_ID}.events.jsonl"
LEGACY_LOG="$STATUS_DIR/cursor-${CONVERSATION_ID}.log"
COUNTERS_FILE="$STATUS_DIR/cursor-${CONVERSATION_ID}.counters"
SUBAGENT_LIST_FILE="$STATUS_DIR/cursor-${CONVERSATION_ID}.subagents.json"

TS=$(date "+%H:%M:%S")

# --- Session counters (prompt_count, tool_count, session_start_ts) ---
# Load existing counters or initialize
if [ -f "$COUNTERS_FILE" ]; then
    # shellcheck disable=SC1090
    source "$COUNTERS_FILE" 2>/dev/null
fi
PROMPT_COUNT="${PROMPT_COUNT:-0}"
TOOL_COUNT="${TOOL_COUNT:-0}"
SESSION_START_TS="${SESSION_START_TS:-$(date +%s)}"
AGENT_MODE="${AGENT_MODE:-agent}"

# --- Subagent list ---
SUBAGENT_COUNT=0
if [ -f "$SUBAGENT_LIST_FILE" ]; then
    SUBAGENT_COUNT=$(jq 'length' "$SUBAGENT_LIST_FILE" 2>/dev/null || echo "0")
fi

# --- Map event to status + update counters + build JSONL event ---
EVENT_JSON=""
LEGACY_LINE=""

case "$HOOK_EVENT" in
    sessionEnd)
        rm -f "$STATUS_FILE" "$EVENTS_FILE" "$LEGACY_LOG" "$COUNTERS_FILE" "$SUBAGENT_LIST_FILE" 2>/dev/null
        exit 0
        ;;
    sessionStart)
        STATUS="idle"
        SESSION_START_TS=$(date +%s)
        PROMPT_COUNT=0
        TOOL_COUNT=0
        echo "[]" > "$SUBAGENT_LIST_FILE"
        # Clear old event log for fresh session
        : > "$EVENTS_FILE"
        EVENT_JSON=$(jq -n --arg ts "$TS" '{"ts":$ts,"type":"session_start"}')
        LEGACY_LINE="[$TS] session started"
        ;;
    beforeSubmitPrompt)
        STATUS="working"
        PROMPT_COUNT=$((PROMPT_COUNT + 1))
        PROMPT_TEXT=$(echo "$INPUT" | jq -r '(.prompt // "")[:200]' 2>/dev/null || echo "")
        # Detect plan vs agent mode from system_reminder injected by Cursor
        case "$FULL_PROMPT" in
            *"Plan mode is active"*|*"Plan mode is still active"*)
                AGENT_MODE="plan" ;;
            *)
                AGENT_MODE="agent" ;;
        esac
        EVENT_JSON=$(jq -n --arg ts "$TS" --arg text "$PROMPT_TEXT" \
            '{"ts":$ts,"type":"prompt","text":$text}')
        LEGACY_LINE="[$TS] prompt submitted"
        # Detect cloud handoff: prompts starting with & are sent to cloud agents
        if [ "${PROMPT_TEXT:0:1}" = "&" ]; then
            CLOUD_HANDOFFS="$STATUS_DIR/cloud-handoffs.jsonl"
            CLOUD_PROMPT="${PROMPT_TEXT:1}"
            jq -n \
                --arg cid "$CONVERSATION_ID" \
                --arg workspace "$WORKSPACE" \
                --arg workspace_name "$(basename "$WORKSPACE" 2>/dev/null)" \
                --arg prompt "$CLOUD_PROMPT" \
                --arg model "$MODEL" \
                --argjson timestamp "$(date +%s)" \
                '{"conversation_id":$cid,"workspace":$workspace,"workspace_name":$workspace_name,"prompt":$prompt,"model":$model,"status":"CREATING","timestamp":$timestamp}' \
                >> "$CLOUD_HANDOFFS" 2>/dev/null
            # Keep only last 50 entries
            if [ -f "$CLOUD_HANDOFFS" ]; then
                LINE_COUNT=$(wc -l < "$CLOUD_HANDOFFS" 2>/dev/null || echo "0")
                if [ "$LINE_COUNT" -gt 50 ]; then
                    TAIL=$(tail -50 "$CLOUD_HANDOFFS")
                    echo "$TAIL" > "$CLOUD_HANDOFFS" 2>/dev/null
                fi
            fi
            EVENT_JSON=$(jq -n --arg ts "$TS" --arg text "$CLOUD_PROMPT" \
                '{"ts":$ts,"type":"cloud_handoff","text":$text}')
            LEGACY_LINE="[$TS] ☁ cloud handoff: ${CLOUD_PROMPT:0:60}"
        fi
        ;;
    preToolUse)
        TOOL_COUNT=$((TOOL_COUNT + 1))
        # Smart status: AskQuestion means waiting for user
        if [ "$TOOL_NAME" = "AskQuestion" ]; then
            STATUS="waiting"
        else
            STATUS="working"
        fi
        # Extract tool input (truncated for log, keep full tool_name)
        TOOL_INPUT_SUMMARY=$(echo "$INPUT" | jq -r '
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
            elif .server then "\(.server):\(.toolName // "")"
            elif .target_notebook then .target_notebook
            else (keys[:2] | join(","))
            end
        ' 2>/dev/null || echo "")
        # Extract plan title from CreatePlan
        PLAN_TITLE=""
        if [ "$TOOL_NAME" = "CreatePlan" ]; then
            PLAN_TITLE=$(echo "$INPUT" | jq -r '.tool_input.name // ""' 2>/dev/null || echo "")
        fi
        EVENT_JSON=$(jq -n --arg ts "$TS" --arg tool "$TOOL_NAME" --arg input "$TOOL_INPUT_SUMMARY" \
            '{"ts":$ts,"type":"tool_start","tool":$tool,"input":$input}')
        if [ -n "$TOOL_INPUT_SUMMARY" ]; then
            LEGACY_LINE="[$TS] ▸ $TOOL_NAME: $TOOL_INPUT_SUMMARY"
        elif [ -n "$TOOL_NAME" ]; then
            LEGACY_LINE="[$TS] ▸ $TOOL_NAME"
        else
            LEGACY_LINE="[$TS] ▸ tool use"
        fi
        ;;
    postToolUse)
        STATUS="working"
        TOOL_OUTPUT_SUMMARY=$(echo "$INPUT" | jq -r '
            (.tool_output // "") | tostring |
            if length > 300 then .[:300] + "..." else . end
        ' 2>/dev/null || echo "")
        EVENT_JSON=$(jq -n --arg ts "$TS" --arg tool "$TOOL_NAME" --arg output "$TOOL_OUTPUT_SUMMARY" \
            '{"ts":$ts,"type":"tool_result","tool":$tool,"output":$output}')
        # No legacy line (preToolUse already logged)
        ;;
    afterAgentResponse)
        STATUS="working"
        RESPONSE_TEXT=$(echo "$INPUT" | jq -r '
            (.message // .content // .text // "") | tostring |
            if length > 300 then .[:300] + "..." else . end
        ' 2>/dev/null || echo "")
        EVENT_JSON=$(jq -n --arg ts "$TS" --arg text "$RESPONSE_TEXT" \
            '{"ts":$ts,"type":"response","text":$text}')
        LEGACY_LINE="[$TS] ✦ response"
        ;;
    afterAgentThought)
        STATUS="working"
        THOUGHT_TEXT=$(echo "$INPUT" | jq -r '
            (.message // .content // .text // "") | tostring |
            if length > 200 then .[:200] + "..." else . end
        ' 2>/dev/null || echo "")
        EVENT_JSON=$(jq -n --arg ts "$TS" --arg text "$THOUGHT_TEXT" \
            '{"ts":$ts,"type":"thought","text":$text}')
        # No legacy line for thoughts (too noisy)
        ;;
    afterFileEdit)
        STATUS="working"
        EDIT_PATH=$(echo "$INPUT" | jq -r '.file_path // ""' 2>/dev/null || echo "")
        EDIT_SUMMARY=$(echo "$INPUT" | jq -r '
            (.edits // []) | length | tostring | . + " edit(s)"
        ' 2>/dev/null || echo "")
        EVENT_JSON=$(jq -n --arg ts "$TS" --arg path "$EDIT_PATH" --arg summary "$EDIT_SUMMARY" \
            '{"ts":$ts,"type":"file_edit","path":$path,"summary":$summary}')
        EDIT_BASENAME=$(basename "$EDIT_PATH" 2>/dev/null || echo "$EDIT_PATH")
        LEGACY_LINE="[$TS] ✏ $EDIT_BASENAME ($EDIT_SUMMARY)"
        ;;
    afterShellExecution)
        STATUS="working"
        SHELL_CMD=$(echo "$INPUT" | jq -r '(.command // "")[:100]' 2>/dev/null || echo "")
        SHELL_EXIT=$(echo "$INPUT" | jq -r '.exit_code // ""' 2>/dev/null || echo "")
        SHELL_OUTPUT=$(echo "$INPUT" | jq -r '
            (.output // "") | tostring |
            if length > 300 then .[:300] + "..." else . end
        ' 2>/dev/null || echo "")
        EVENT_JSON=$(jq -n --arg ts "$TS" --arg cmd "$SHELL_CMD" --arg exit "$SHELL_EXIT" --arg output "$SHELL_OUTPUT" \
            '{"ts":$ts,"type":"shell_result","command":$cmd,"exit_code":$exit,"output":$output}')
        LEGACY_LINE="[$TS] $ $SHELL_CMD -> exit $SHELL_EXIT"
        ;;
    subagentStart)
        STATUS="working"
        SUBAGENT_DESC=$(echo "$INPUT" | jq -r '.description // ""' 2>/dev/null || echo "")
        SUBAGENT_MODEL=$(echo "$INPUT" | jq -r '.model // ""' 2>/dev/null || echo "")
        # Use real agent_type from hook input; fall back to inference for older versions
        SUBAGENT_TYPE="$AGENT_TYPE"
        if [ -z "$SUBAGENT_TYPE" ]; then
            SUBAGENT_TYPE="general"
            case "$SUBAGENT_DESC" in
                *[Ee]xplor*) SUBAGENT_TYPE="explore" ;;
                *[Ss]hell*|*[Bb]ash*|*[Cc]ommand*) SUBAGENT_TYPE="shell" ;;
                *[Bb]rows*) SUBAGENT_TYPE="browser" ;;
                *[Rr]eview*) SUBAGENT_TYPE="code-reviewer" ;;
                *[Pp]lan*) SUBAGENT_TYPE="plan" ;;
                *[Dd]ebug*) SUBAGENT_TYPE="debug" ;;
                *[Ss]earch*|*[Ff]ind*) SUBAGENT_TYPE="explore" ;;
                *[Tt]est*) SUBAGENT_TYPE="test" ;;
            esac
        fi
        EXISTING="[]"
        [ -f "$SUBAGENT_LIST_FILE" ] && EXISTING=$(cat "$SUBAGENT_LIST_FILE" 2>/dev/null || echo "[]")
        echo "$EXISTING" | jq --arg id "$AGENT_ID" --arg type "$SUBAGENT_TYPE" --arg desc "$SUBAGENT_DESC" \
            --arg model "$SUBAGENT_MODEL" --arg ts "$TS" --arg parent "$PARENT_AGENT_ID" \
            '. + [{"id":$id,"agent_type":$type,"description":$desc,"model":$model,"started_at":$ts,"parent_agent_id":$parent}]' \
            > "$SUBAGENT_LIST_FILE" 2>/dev/null
        SUBAGENT_COUNT=$(jq 'length' "$SUBAGENT_LIST_FILE" 2>/dev/null || echo "0")
        EVENT_JSON=$(jq -n --arg ts "$TS" --arg desc "$SUBAGENT_DESC" --arg model "$SUBAGENT_MODEL" --arg atype "$SUBAGENT_TYPE" \
            '{"ts":$ts,"type":"subagent_start","description":$desc,"model":$model,"tool":$atype}')
        LEGACY_LINE="[$TS] ◆ $SUBAGENT_TYPE: $SUBAGENT_DESC"
        ;;
    subagentStop)
        STATUS="working"
        # Remove subagent by agent_id; fall back to LIFO for older versions
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
        SA_SUMMARY=$(echo "$INPUT" | jq -r '(.last_assistant_message // "")[:200]' 2>/dev/null || echo "")
        EVENT_JSON=$(jq -n --arg ts "$TS" --arg atype "$AGENT_TYPE" --arg summary "$SA_SUMMARY" \
            '{"ts":$ts,"type":"subagent_stop","tool":$atype,"summary":$summary}')
        LEGACY_LINE="[$TS] ◆ subagent completed"
        ;;
    preCompact)
        STATUS="working"
        EVENT_JSON=$(jq -n --arg ts "$TS" '{"ts":$ts,"type":"compact"}')
        LEGACY_LINE="[$TS] ⟳ compacting context"
        ;;
    stop)
        STATUS="idle"
        EVENT_JSON=$(jq -n --arg ts "$TS" --arg reason "$STOP_REASON" \
            '{"ts":$ts,"type":"stop","reason":$reason}')
        if [ -n "$STOP_REASON" ]; then
            LEGACY_LINE="[$TS] ■ stopped ($STOP_REASON)"
        else
            LEGACY_LINE="[$TS] ■ stopped"
        fi
        ;;
    teammateIdle)
        STATUS="idle"
        EVENT_JSON=$(jq -n --arg ts "$TS" '{"ts":$ts,"type":"teammate_idle"}')
        LEGACY_LINE="[$TS] ◇ teammate idle"
        ;;
    instructionsLoaded)
        # Emit event log entry only; do not overwrite status
        STATUS=""
        EVENT_JSON=$(jq -n --arg ts "$TS" '{"ts":$ts,"type":"rules_loaded"}')
        ;;
    *)
        exit 0
        ;;
esac

# --- Tag events with agent context when running inside a subagent ---
if [ -n "$AGENT_ID" ] && [ -n "$EVENT_JSON" ]; then
    EVENT_JSON=$(echo "$EVENT_JSON" | jq --arg aid "$AGENT_ID" --arg atype "$AGENT_TYPE" \
        '. + {agent_id: $aid, agent_type: $atype}')
fi

# --- Save counters ---
cat > "$COUNTERS_FILE" << EOF
PROMPT_COUNT=$PROMPT_COUNT
TOOL_COUNT=$TOOL_COUNT
SESSION_START_TS=$SESSION_START_TS
AGENT_MODE=$AGENT_MODE
EOF

# --- Write status JSON (skip for log-only events like instructionsLoaded) ---
if [ -n "$STATUS" ]; then
WORKSPACE_NAME=$(basename "$WORKSPACE" 2>/dev/null || echo "unknown")
jq -n \
    --arg cid "$CONVERSATION_ID" \
    --arg source "cursor" \
    --arg status "$STATUS" \
    --arg event "$HOOK_EVENT" \
    --arg model "$MODEL" \
    --arg workspace "$WORKSPACE" \
    --arg workspace_name "$WORKSPACE_NAME" \
    --arg cursor_version "$CURSOR_VERSION" \
    --arg session_id "$SESSION_ID" \
    --arg cwd "$CWD" \
    --arg last_tool "$TOOL_NAME" \
    --arg stop_reason "$STOP_REASON" \
    --arg permission_mode "$PERMISSION_MODE" \
    --arg plan_title "${PLAN_TITLE:-}" \
    --arg transcript_path "$TRANSCRIPT_PATH" \
    --arg worktree_path "${WORKTREE_PATH:-}" \
    --arg worktree_branch "${WORKTREE_BRANCH:-}" \
    --arg original_repo "${ORIGINAL_REPO:-}" \
    --arg effort_level "${EFFORT_LEVEL:-}" \
    --argjson prompt_count "$PROMPT_COUNT" \
    --argjson tool_count "$TOOL_COUNT" \
    --argjson session_start_ts "$SESSION_START_TS" \
    --argjson subagent_count "$SUBAGENT_COUNT" \
    --arg agent_mode "$AGENT_MODE" \
    --argjson timestamp "$(date +%s)" \
    '{
        conversation_id: $cid,
        source: $source,
        status: $status,
        event: $event,
        model: $model,
        workspace: $workspace,
        workspace_name: $workspace_name,
        cursor_version: $cursor_version,
        session_id: $session_id,
        cwd: $cwd,
        last_tool: $last_tool,
        stop_reason: $stop_reason,
        permission_mode: $permission_mode,
        agent_mode: $agent_mode,
        plan_title: $plan_title,
        transcript_path: $transcript_path,
        worktree_path: $worktree_path,
        worktree_branch: $worktree_branch,
        original_repo: $original_repo,
        effort_level: $effort_level,
        prompt_count: $prompt_count,
        tool_count: $tool_count,
        session_start_ts: $session_start_ts,
        subagent_count: $subagent_count,
        timestamp: $timestamp
    }' > "$STATUS_FILE" 2>/dev/null
fi

# --- Append JSONL event (for rich preview) ---
if [ -n "$EVENT_JSON" ]; then
    echo "$EVENT_JSON" >> "$EVENTS_FILE" 2>/dev/null
    # Cap at 200 events
    if [ -f "$EVENTS_FILE" ]; then
        LINE_COUNT=$(wc -l < "$EVENTS_FILE" 2>/dev/null || echo "0")
        if [ "$LINE_COUNT" -gt 200 ]; then
            TAIL=$(tail -200 "$EVENTS_FILE")
            echo "$TAIL" > "$EVENTS_FILE" 2>/dev/null
        fi
    fi
fi

# --- Append legacy plain-text log (backward compat) ---
if [ -n "$LEGACY_LINE" ]; then
    echo "$LEGACY_LINE" >> "$LEGACY_LOG" 2>/dev/null
    if [ -f "$LEGACY_LOG" ]; then
        TAIL=$(tail -50 "$LEGACY_LOG")
        echo "$TAIL" > "$LEGACY_LOG" 2>/dev/null
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

# --- Periodic cleanup (only on stop events to reduce I/O) ---
if [ "$HOOK_EVENT" = "stop" ]; then
    find "$STATUS_DIR" -name "cursor-*.json" -mmin +30 -delete 2>/dev/null
    find "$STATUS_DIR" -name "cursor-*.log" -mmin +30 -delete 2>/dev/null
    find "$STATUS_DIR" -name "cursor-*.events.jsonl" -mmin +30 -delete 2>/dev/null
    find "$STATUS_DIR" -name "cursor-*.counters" -mmin +30 -delete 2>/dev/null
    find "$STATUS_DIR" -name "cursor-*.subagents.json" -mmin +30 -delete 2>/dev/null
fi

exit 0
