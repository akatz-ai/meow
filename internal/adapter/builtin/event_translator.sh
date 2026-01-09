#!/bin/bash
# Claude Code event translator for MEOW
# Translates Claude Code hooks to MEOW events
#
# This script is called by Claude Code hooks and forwards events
# to the MEOW orchestrator via the meow CLI.
#
# Usage: event-translator.sh <hook_type> [args...]
#
# Hook types:
#   Stop       - Called when Claude reaches prompt (waiting for input)
#   PreToolUse - Called before a tool is invoked (arg: tool name)
#   PostToolUse - Called after a tool completes (arg: tool name)

set -e

HOOK_TYPE="$1"
shift

case "$HOOK_TYPE" in
  Stop)
    # Agent reached prompt - either completed work or waiting for input
    meow event agent-stopped 2>/dev/null || true
    ;;
  PreToolUse)
    # Agent is about to use a tool
    TOOL_NAME="${1:-unknown}"
    meow event tool-starting --data tool="$TOOL_NAME" 2>/dev/null || true
    ;;
  PostToolUse)
    # Agent finished using a tool
    TOOL_NAME="${1:-unknown}"
    meow event tool-completed --data tool="$TOOL_NAME" 2>/dev/null || true
    ;;
  *)
    # Unknown hook type - log but don't fail
    meow event unknown --data type="$HOOK_TYPE" 2>/dev/null || true
    ;;
esac

# Always exit successfully - don't block Claude on event delivery failures
exit 0
