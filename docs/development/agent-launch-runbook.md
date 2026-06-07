# Local Multi-Agent Launch Runbook

Status: Active runbook · Last updated: 2026-06-03
Companion: root `CLAUDE.md` (conventions)

This runbook is for running multiple Claude Code agents **from your own machine** (WSL/Ubuntu)
using git worktrees so several sessions work the same repo concurrently without colliding.
Each agent gets its own worktree, its own branch, and one task brief.

> Verify flags against your installed CLI with `claude --help` before a big run — the CLI
> evolves. The plain `git worktree` mechanics below are stable regardless of CLI version.

## 0) One-time setup

```bash
# From your repo root
cd ~/HardwareOps
git fetch origin
git checkout main && git pull origin main

# Keep worktrees out of version control
grep -qxF '.worktrees/' .gitignore || echo '.worktrees/' >> .gitignore

mkdir -p .worktrees tasks results
```

## 1) Write one task brief per item

One file per roadmap item. Keep each brief self-contained: the roadmap item, the grounding
file paths, the conventions, and an explicit "open a PR, do not merge" instruction.

`tasks/f1-transport-seam.md` (example):

```
You are working ONLY on roadmap item F1 (see docs/development/roadmap.md).

Goal: extract the agent's HTTP/mTLS client into a `Transport` interface with NO behavior
change. The current client becomes `HTTPTransport`.

Files: agent/internal/client/client.go (NewWithTLS, CheckIn, GetArtifact, PresignArtifact,
apply-result post); call sites agent/cmd/agent/main.go and
agent/internal/artifacts/apply.go.

Rules:
- Stay inside the agent/ module. Do not touch control-plane/ or ui/.
- `cd agent && go test ./...` must stay green.
- Flip the F1 status marker ⬜ → 🟡 in docs/development/roadmap.md.
- Commit, push the branch, open a PR, and STOP. Do not merge.
```

## 2) Create a worktree + branch per item

```bash
cd ~/HardwareOps
git worktree add -b claude/f1-transport-seam  .worktrees/f1 main
git worktree add -b claude/g1-foundation      .worktrees/g1 main

git worktree list   # confirm each worktree is on its own branch
```

Each `.worktrees/<x>` is a full, independent checkout. Edits in one never touch another.

## 3a) Option A — interactive, one terminal per agent (recommended first time)

Open one terminal per worktree. In each:

```bash
cd ~/HardwareOps/.worktrees/f1
claude            # then paste the contents of tasks/f1-transport-seam.md
```

You watch each agent, approve actions, and keep full control. Best when you want to
supervise the first run and learn how each task behaves.

## 3b) Option B — headless/parallel (automation)

Headless print mode runs each agent to completion unattended. Launch all as background jobs:

```bash
cd ~/HardwareOps

run_agent () {           # run_agent <worktree-dir> <branch> <task-file>
  local dir="$1" branch="$2" task="$3"
  ( cd "$dir" && claude -p "$(cat "$task")" \
      --model opus \
      --permission-mode acceptEdits \
      --allowedTools "Read,Edit,Write,Bash(go test ./...),Bash(npm run test:rbac),Bash(git add:*),Bash(git commit:*),Bash(git push:*)" \
      --output-format json \
    > "results/${branch//\//_}.json" 2>&1 )
  echo "done: $branch"
}

run_agent .worktrees/f1 claude/f1-transport-seam tasks/f1-transport-seam.md &
run_agent .worktrees/g1 claude/g1-foundation     tasks/g1-foundation.md     &
wait
echo "all agents finished"
```

Flag notes (confirm with `claude --help`):
- `-p "<prompt>"` — headless print mode; runs and exits.
- `--permission-mode acceptEdits` — auto-approves file edits and pre-listed Bash; still
  prompts/denies anything not allow-listed. Safer than a full bypass on your own machine.
- `--allowedTools "…"` — pre-approve exactly the tools/commands each agent needs. Scope Bash
  to specific commands (e.g. `Bash(go test ./...)`) rather than a blanket allow.
- `--model opus` — pick the model (`opus` / `sonnet` / `haiku`).
- `--output-format json` — machine-readable result for `results/` collection.
- `--dangerously-skip-permissions` exists but skips **all** prompts — use only in a throwaway
  VM/container, never on a workstation with real credentials. Prefer `acceptEdits` + allowlist.

## 4) Watch progress and collect results

```bash
git worktree list                                   # where each agent is working
tail -f results/claude_f1-transport-seam.json       # one agent's stream
( cd .worktrees/f1 && git log --oneline -5 )        # what it committed
```

## 5) Review, push, PR, merge — then clean up

For interactive runs, each agent pushes its branch and opens a PR per the brief. For headless,
verify the branch/PR exist, then review as normal. After a PR merges:

```bash
git worktree remove .worktrees/f1
git branch -D claude/f1-transport-seam             # local
git push origin --delete claude/f1-transport-seam  # remote
```

## 6) Parallelization rules

**Safe to run concurrently:** items in different modules (`agent/` vs `control-plane/` vs
`docs/`). Doc-only tasks never collide with code tasks.

**Must serialize:** items that both touch `control-plane/internal/store/store.go`, the
`memory`/`postgres` store implementations, or the sequential migration numbering
(`control-plane/migrations/` — next free slot is `0037`). Run whichever claims the migration
first, merge it, then rebase the next item on updated `main` before starting.

**Never run concurrently:** two items that both add a migration. Coordinate explicitly: agree
on ordering, assign migration numbers in the task briefs, and rebase in that order.
