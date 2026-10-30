# AUX

`aux` (module `github.com/oglenyaboss/aux-player`) grew out of a fork of `simonepelosi/vibez`, an Apple Music TUI in Go/Bubble Tea. The stage lives in `internal/stage/`, the music picker agent in `internal/vibeai/`, and the stage host and the bridge to the agent in `internal/tui/` (`model.go`, `stage.go`, `vibeai.go`). The player and the Apple provider are inherited code: change them minimally and port fixes from `upstream` by hand (the module paths have diverged).

## GitHub

Issues and PRs go to `oglenyaboss/aux-player` only. The `upstream` remote (`simonepelosi/vibez`) is a source of fixes; nothing is created there. If `gh repo view` shows another repo, run `gh repo set-default oglenyaboss/aux-player`.

## Picker agent

- Live data for judging a change to the picker: session logs in `~/.cache/aux/vibe/runs/`, signals in `~/.config/aux/vibe-signals.jsonl`, memory in `~/.config/aux/vibe-memory.md`.
- `~/.config/aux/config.json` and `lab/*.env` hold tokens and keys: read single fields, never print them.
- `lab/scene` renders the stage without a player (`go run ./lab/scene -demo`, `-egg "artist title"`, `-shots 3,10`).
- Code, comments, docs and commit messages are in English. User-facing strings go through `stage.Tr(ru, en)`.

## Agent skills

### Issue tracker

GitHub Issues on `oglenyaboss/aux-player` through `gh`. See `docs/agents/issue-tracker.md`.

### Triage labels

Five labels: needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix. See `docs/agents/triage-labels.md`.

### Domain docs

Single context: `GLOSSARY.md` and `docs/adr/` at the root. See `docs/agents/domain.md`.
