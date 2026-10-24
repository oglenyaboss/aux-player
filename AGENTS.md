# AUX

Самостоятельное приложение `aux` (модуль `github.com/oglenyaboss/aux-player`), выросло из форка `simonepelosi/vibez` (Apple Music TUI на Go/Bubble Tea). Сцена — `internal/stage/`, подборщик — `internal/vibeai/`, хост сцены и мост к агенту — `internal/tui/` (`model.go`, `stage.go`, `vibeai.go`). Плеер и Apple-провайдер — код vibez: правим минимально, фиксы из `upstream` переносим вручную (пути модуля разошлись).

## GitHub

Все issues и PR — только в `oglenyaboss/aux-player`. Remote `upstream` (`simonepelosi/vibez`) — источник фиксов, туда ничего не создаём. Если `gh repo view` показывает другой репо — `gh repo set-default oglenyaboss/aux-player`.

## Vibes AI

- Решения по подбору — `lab/SPEC.md` и `lab/PLAN.md`; меняются только с согласия владельца.
- Живые данные владельца для оценки изменений: логи сессий `~/.cache/aux/vibe/runs/`, сигналы `~/.config/aux/vibe-signals.jsonl`, память `~/.config/aux/vibe-memory.md`.
- `~/.config/aux/config.json` и `lab/*.env` содержат токены и ключи — читаем точечно, в вывод не печатаем.
- Тексты для владельца, промпт, брифы, доки `lab/` — по-русски. Код апстрима — в его стиле, по-английски.
- Брифы воркерам — `lab/briefs/NNx-*.md`.

## Agent skills

### Issue tracker

GitHub Issues `oglenyaboss/aux-player` через `gh`. See `docs/agents/issue-tracker.md`.

### Triage labels

Пять стандартных меток: needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` + `docs/adr/` в корне. See `docs/agents/domain.md`.
