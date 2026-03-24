#!/bin/bash
# Setup script for Cursor IDE hooks
# This installs hooks that enable tmux-overseer to detect Cursor sessions
#
# What it does:
# 1. Copies cursor-status-hook.sh to ~/.cursor/hooks/
# 2. Creates or merges ~/.cursor/hooks.json with hook configuration
#
# Events tracked: sessionStart, sessionEnd, beforeSubmitPrompt, stop,
#                 preToolUse, postToolUse, subagentStop, subagentStart,
#                 preCompact, afterAgentResponse, afterAgentThought,
#                 afterFileEdit, afterShellExecution

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CURSOR_HOOKS_DIR="${HOME}/.cursor/hooks"
CURSOR_CONFIG="${HOME}/.cursor/hooks.json"
HOOK_SCRIPT="${SCRIPT_DIR}/cursor-status-hook.sh"
INSTALLED_HOOK="${CURSOR_HOOKS_DIR}/status-hook.sh"

echo "Cursor Hooks Setup"
echo "=================="
echo ""
echo "This will configure Cursor to report session status."
echo "Hook script: ${HOOK_SCRIPT}"
echo "Config file: ${CURSOR_CONFIG}"
echo ""

# Check if hook script exists
if [ ! -f "$HOOK_SCRIPT" ]; then
    echo "Error: Hook script not found at ${HOOK_SCRIPT}"
    exit 1
fi

# Check jq is available (needed by hook script)
if ! command -v jq &>/dev/null; then
    echo "Error: jq is required but not installed."
    echo "Install with: brew install jq"
    exit 1
fi

# Create hooks directory
mkdir -p "$CURSOR_HOOKS_DIR"

# Copy hook script to Cursor hooks directory
cp "$HOOK_SCRIPT" "$INSTALLED_HOOK"
chmod +x "$INSTALLED_HOOK"
echo "Installed hook script to ${INSTALLED_HOOK}"

# Use absolute path in hooks config for reliability
HOOK_CMD="${INSTALLED_HOOK}"

# All events we register hooks for (Cursor 2.6 valid types only)
ALL_EVENTS="sessionStart sessionEnd beforeSubmitPrompt stop preToolUse postToolUse subagentStop subagentStart preCompact afterAgentResponse afterAgentThought afterFileEdit afterShellExecution"

# Build the jq fragment that adds our hooks to all events.
# Only command hooks are registered; the hook script itself forwards to
# the overseer's HTTP server when it's running (fire-and-forget).
ADD_HOOKS_JQ='.version = 1'
for evt in $ALL_EVENTS; do
    ADD_HOOKS_JQ="$ADD_HOOKS_JQ | .hooks.${evt} = (.hooks.${evt} // []) + [{\"command\": \$script}]"
done

# Check if hooks.json already exists
if [ -f "$CURSOR_CONFIG" ]; then
    echo ""
    echo "Existing hooks.json found. Creating backup..."
    BACKUP="${CURSOR_CONFIG}.backup.$(date +%s)"
    cp "$CURSOR_CONFIG" "$BACKUP"
    echo "Backup saved to: ${BACKUP}"
    
    # Check if our hooks are already configured
    if grep -q "status-hook.sh" "$CURSOR_CONFIG" 2>/dev/null; then
        echo ""
        echo "Hooks already configured. Updating with latest configuration..."
        
        TEMP_CONFIG=$(mktemp)
        jq --arg script "$HOOK_CMD" "
            (.hooks // {}) |= with_entries(
                .value |= map(select(
                    (.command // null) as \$c |
                    if \$c then (\$c | test(\"status-hook\") | not) else true end
                ))
            ) |
            (.hooks // {}) |= with_entries(
                .value |= map(select(.url // null | not))
            ) |
            $ADD_HOOKS_JQ
        " "$CURSOR_CONFIG" > "$TEMP_CONFIG"
        
        mv "$TEMP_CONFIG" "$CURSOR_CONFIG"
    else
        echo ""
        echo "Merging hooks into existing configuration..."
        
        TEMP_CONFIG=$(mktemp)
        jq --arg script "$HOOK_CMD" "$ADD_HOOKS_JQ" "$CURSOR_CONFIG" > "$TEMP_CONFIG"
        
        mv "$TEMP_CONFIG" "$CURSOR_CONFIG"
    fi
else
    echo "Creating new hooks.json..."
    jq -n --arg script "$HOOK_CMD" "$ADD_HOOKS_JQ" > "$CURSOR_CONFIG"
fi

echo ""
echo "Setup complete!"
echo ""
echo "Events tracked:"
echo "  $ALL_EVENTS" | fmt -w 60
echo ""
echo "Status files written to: ~/.claude-tmux/cursor-*.json"
echo "Event log written to:    ~/.claude-tmux/cursor-*.events.jsonl"
echo "The hook script forwards events to the overseer TUI automatically"
echo "when it's running (no re-setup needed)."
echo ""
echo "You may need to restart Cursor for changes to take effect."
