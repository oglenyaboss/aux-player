# AUX

A terminal music player for Apple Music: describe a vibe, an AI agent picks the tracks.

## Install

Build from source with Go 1.26+:

```sh
git clone https://github.com/oglenyaboss/aux-player
cd aux-player
make build      # produces ./aux
make install    # builds with an embedded Apple developer token and installs to ~/.local/bin/aux
```

On Linux, `make build` checks for the GStreamer 1.0, GTK 3 and WebKit2GTK 4.1 (or 4.0) development packages first; install them through your distro's package manager. `make install` additionally needs `APPLE_KEY_ID`, `APPLE_TEAM_ID` and `APPLE_PRIVATE_KEY` in the environment or a local `.env`. Override the install location with `make install PREFIX=/usr/local`.

## First run

The onboarding walks through four screens before playback starts:

1. **Language** — `←`/`→` to choose, `enter` to continue.
2. **Apple Music** — `enter` opens a browser to sign in with your Apple ID; the screen waits for the sign-in to finish.
3. **Agent** — pick an OpenAI-compatible provider, enter its API address and key when asked, choose a model that supports tools, then choose the reasoning effort. The screen verifies the provider and fetches its model list.
4. **Last.fm** — optional scrobbling. `enter` connects, using API credentials from `last.fm/api/account/create`; `tab` skips the step.

## Keys

| Key | Action |
|-----|--------|
| `space` | Play / pause |
| `n` | Next track |
| `p` | Previous track |
| `←` / `→` | Seek backward / forward 10 s |
| `/` | Talk to the agent |
| `+` | More like this |
| `-` | Less like this |
| `w` | Save the queue as a playlist |
| `r` | Start over |
| `q` | Quit |
| `?` | Toggle the key help |
| `esc` | Close help / step back |
| `tab` | Settings on the intro; skip the Last.fm step during onboarding |
| `ctrl+c` | Quit |

## Data

- `~/.config/aux/config.json` — settings and tokens, written with `0600` permissions.
- `~/.config/aux/vibe-signals.jsonl` — the agent's feedback signals.
- `~/.config/aux/vibe-memory.md` — what the agent remembers about your taste.
- `~/.cache/aux/` — library cache, agent session logs under `vibe/runs`, and the browser.

## Credits

AUX is based on [vibez](https://github.com/simonepelosi/vibez) by Simone Pelosi, released under the MIT license. See `LICENSE` and `NOTICE`.
