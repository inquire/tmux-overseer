#!/bin/bash
# Setup script to configure Claude Code CLI hooks for status detection
# Run this once to enable hook-based status detection in claude-tmux
#
# Registers all 11 lifecycle events with a single hook command (no $1 arg).
# The hook script reads hook_event_name from stdin JSON.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SETTINGS_FILE="${HOME}/.claude/settings.json"

# Prefer compiled Go hook binary (fast, <10ms), fall back to bash script (~60ms)
if command -v claude-hook &>/dev/null; then
    HOOKS_SCRIPT="$(command -v claude-hook)"
    echo "Using compiled Go hook binary: $HOOKS_SCRIPT"
else
    HOOKS_SCRIPT="$SCRIPT_DIR/status-hook.sh"
    if [ ! -f "$HOOKS_SCRIPT" ]; then
        echo "Error: Hook script not found at $HOOKS_SCRIPT"
        echo "Install the Go binary with: cd tmux-overseer && make install-hook"
        exit 1
    fi
    chmod +x "$HOOKS_SCRIPT"
    echo "Using bash hook script: $HOOKS_SCRIPT"
    echo "  Tip: Install claude-hook for 6x faster hooks: make install-hook"
fi

echo "Claude-tmux Hook Setup"
echo "====================="
echo ""
echo "This will add status hooks to your Claude Code settings."
echo "Hook script: $HOOKS_SCRIPT"
echo "Settings file: $SETTINGS_FILE"
echo ""

ALL_EVENTS="SessionStart SessionEnd UserPromptSubmit Stop PreToolUse PostToolUse PostToolUseFailure SubagentStart SubagentStop Notification PreCompact TaskCompleted TeammateIdle InstructionsLoaded"

mkdir -p "${HOME}/.claude"

if [ -f "$SETTINGS_FILE" ]; then
    echo "Existing settings found. Creating backup..."
    cp "$SETTINGS_FILE" "${SETTINGS_FILE}.backup.$(date +%s)"

    if grep -q "status-hook.sh" "$SETTINGS_FILE" 2>/dev/null; then
        echo ""
        echo "Hooks already configured! Updating with all events..."
    fi
fi

if ! command -v jq &>/dev/null; then
    echo "jq is required. Please install jq and re-run."
    exit 1
fi

# Build hooks config for all events (command hooks only).
# The hook script itself forwards to the overseer's HTTP server when it's
# running (fire-and-forget), so no URL hooks are needed here.
# SessionEnd gets a generous timeout since Claude Code v2.1.74 fixed the 1.5s kill bug.
HOOKS_CONFIG="{}"
for event in $ALL_EVENTS; do
    TIMEOUT_MS=5000
    if [ "$event" = "SessionEnd" ]; then
        TIMEOUT_MS=8000
    fi
    HOOKS_CONFIG=$(echo "$HOOKS_CONFIG" | jq \
        --arg event "$event" \
        --arg cmd "$HOOKS_SCRIPT" \
        --argjson timeout "$TIMEOUT_MS" \
        '.[$event] = [{"hooks": [{"type": "command", "command": $cmd, "timeout": $timeout}]}]')
done

if [ -f "$SETTINGS_FILE" ]; then
    EXISTING_HOOKS=$(jq -r '.hooks // {}' "$SETTINGS_FILE")
    # Remove old status-hook.sh entries, then merge new ones
    CLEANED=$(echo "$EXISTING_HOOKS" | jq 'with_entries(select(.value | tostring | contains("status-hook.sh") | not))')
    MERGED_HOOKS=$(echo "$CLEANED" "$HOOKS_CONFIG" | jq -s 'add')
    jq --argjson hooks "$MERGED_HOOKS" '.hooks = $hooks' "$SETTINGS_FILE" > "${SETTINGS_FILE}.tmp"
    mv "${SETTINGS_FILE}.tmp" "$SETTINGS_FILE"
else
    echo "{}" | jq --argjson hooks "$HOOKS_CONFIG" '{hooks: $hooks}' > "$SETTINGS_FILE"
fi

echo "Done! Hooks registered for all events:"
for event in $ALL_EVENTS; do
    echo "  - $event"
done
echo ""
echo "The hook script forwards events to the overseer TUI automatically"
echo "when it's running (no re-setup needed)."
echo ""
echo "Tip: Add CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS=8000 to your shell"
echo "     environment to ensure SessionEnd cleanup always completes."
echo ""
echo "Note: Restart Claude Code for hooks to take effect."
