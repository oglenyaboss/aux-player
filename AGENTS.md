# vibezai

Форк `simonepelosi/vibez` (Apple Music TUI на Go/Bubble Tea). Наша работа — Vibes AI: подборщик `internal/vibeai/` и его экран в `internal/tui/` (`vibeai.go`, `views/vibeai.go`). Плеер и Apple-провайдер — код апстрима: правим минимально, чтобы фиксы из `upstream` ложились чисто.

## GitHub

Все issues и PR — только в `oglenyaboss/vibezai`. Remote `upstream` — источник фиксов, туда ничего не создаём. Если `gh repo view` показывает не форк — `gh repo set-default oglenyaboss/vibezai`.

## Vibes AI

- Решения по подбору — `lab/SPEC.md` и `lab/PLAN.md`; меняются только с согласия владельца.
- Живые данные владельца для оценки изменений: логи сессий `~/.cache/vibez/vibe/runs/`, сигналы `~/.config/vibez/vibe-signals.jsonl`, память `~/.config/vibez/vibe-memory.md`.
- `~/.config/vibez/config.json` и `lab/*.env` содержат токены и ключи — читаем точечно, в вывод не печатаем.
- Тексты для владельца, промпт, брифы, доки `lab/` — по-русски. Код апстрима — в его стиле, по-английски.
- Брифы воркерам — `lab/briefs/NNx-*.md`.

## Agent skills

### Issue tracker

GitHub Issues форка `oglenyaboss/vibezai` через `gh`. See `docs/agents/issue-tracker.md`.

### Triage labels

Пять стандартных меток: needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` + `docs/adr/` в корне. See `docs/agents/domain.md`.
