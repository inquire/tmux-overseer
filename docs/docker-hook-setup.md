# Docker Hook Setup for tmux-overseer

tmux-overseer reads Claude Code hook data from `~/.claude-tmux/` on the host.
When Claude runs inside a Docker sandbox, the hook fires inside the container
and writes to the *container's* `~/.claude-tmux/`, which the host never sees.

Until `~/.claude-tmux` is mounted into the container, Docker sessions show
with minimal metadata: no agent mode, no prompt/tool counts, no subagent
tracking, and no sandbox badge.

## What you get once it's mounted

- `[⬡ docker]` sandbox badge on the session row
- Live agent mode (`[AGENT]`, `[PLAN]`)
- Prompt and tool counts
- Subagent tracking (count + individual subagent rows)
- Cost (from hook data rather than terminal scraping)

## Setup

The two requirements are:

1. Mount `~/.claude-tmux` from the host into the container (read-write)
2. Make the `claude-hook` binary available inside the container

### Step 1 — mount `~/.claude-tmux`

Map the host directory into the container at the same path so the hook binary
inside Docker writes directly to the directory that tmux-overseer watches:

```bash
docker run \
  -v ~/.claude-tmux:/root/.claude-tmux \
  ...
```

Or add it to your Docker Compose / sandbox config:

```yaml
volumes:
  - ~/.claude-tmux:/root/.claude-tmux
```

### Step 2 — bake in the hook binary

The `claude-hook` binary must be available inside the container. Cross-compile
it for the container's architecture and copy it in:

```bash
# Example: cross-compile for linux/arm64
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags '-s -w' \
  -o claude-hook-linux-arm64 ./cmd/claude-hook
```

Place the binary at `~/.claude/hooks/claude-hook` inside the container
(matching the path configured in `~/.claude/settings.json`).

### Step 3 — ensure TMUX_PANE is forwarded

The `TMUX_PANE` environment variable must be set inside the container to the
pane ID of the host tmux pane that launched Docker. The hook binary uses it as
the filename key so tmux-overseer can correlate the status file with the
correct session row.

Pass it explicitly when starting the container:

```bash
docker run -e TMUX_PANE="$TMUX_PANE" ...
```

## How it works end-to-end

```
Claude (inside Docker)
  └─ fires hook → /root/.claude/hooks/claude-hook
       └─ writes ~/.claude-tmux/status-<pane>.json
            └─ mounted from host ~/.claude-tmux/ (read-write)
                 └─ tmux-overseer reads it on the host
```

## Verifying it works

Start Claude inside the container and look for a status file on the host:

```bash
ls ~/.claude-tmux/status-*.json
cat ~/.claude-tmux/status-<pane>.json | jq .sandbox_type
# should print "docker"
```

tmux-overseer will then show the 🐳 badge and full hook metadata on the
session row.
