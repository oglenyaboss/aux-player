# AUX

A terminal music player for Apple Music: describe a vibe, an AI agent picks the tracks.

You type what you want to hear ("late-night coding, like Westside Gunn but calmer"). The agent may ask one quick question with real tracks to choose from, then plays portions of five tracks, keeps the next portion ready while the current one plays, and learns from what you skip, finish, and rate. It remembers your stable preferences between sessions.

## Requirements

- Linux (amd64 or arm64). macOS and Windows builds are not published yet.
- An Apple Music subscription.
- GStreamer 1.0 with an AAC decoder: `gstreamer`, `gst-plugins-base`, `gst-plugins-good` and `gst-libav` from your package manager.
- An OpenAI-compatible LLM endpoint with tool calling, and its API key.

No Apple Developer account is needed: AUX uses the public developer token of the Apple Music web player. The Widevine CDM that decrypts playback is downloaded on first run into `~/.cache/aux/cdm`.

## Install

From a release:

```sh
curl --proto '=https' --tlsv1.2 -sSf \
  https://raw.githubusercontent.com/oglenyaboss/aux-player/main/scripts/install.sh | sh
```

The script installs `aux` into `~/.local/bin` (override with `AUX_INSTALL_DIR`), verifies the release checksum and adds a desktop entry and icon. Read it before running.

From source, with Go 1.26+ and the GStreamer development packages:

```sh
git clone https://github.com/oglenyaboss/aux-player
cd aux-player
make build      # produces ./aux
make install    # builds and copies aux to ~/.local/bin (PREFIX=/usr/local to change)
```

`make build` embeds Last.fm API keys when `LASTFM_API_KEY` and `LASTFM_API_SECRET` are set in the environment or a local `.env`.

## First run

Run `aux`. The onboarding walks through four screens:

1. **Language**: English or Russian. `←`/`→` to choose, `enter` to continue.
2. **Apple Music**: `enter` opens a browser to sign in with your Apple ID; the screen waits for the sign-in to finish.
3. **Agent**: enter the API address and key of your LLM provider, pick a model that supports tools, then the reasoning effort. The screen checks the provider and fetches its model list.
4. **Last.fm**: optional scrobbling. `enter` connects with API credentials from `last.fm/api/account/create`; `tab` skips the step.

`tab` on the intro reopens these screens as settings. Settings also hold the **Easter eggs** switch (saved as `vibe_eggs` in the config).

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
| `?` | Toggle the key help |
| `esc` | Close help / step back |
| `tab` | Settings on the intro; skip the Last.fm step during onboarding |
| `q`, `ctrl+c` | Quit |

When the agent asks a question with track cards:

| Key | Action |
|-----|--------|
| `←` `→` `↑` `↓` or `h` `l` `k` `j` | Move between options |
| `1`–`9` | Pick an option by number |
| `enter`, `space` | Choose |
| `a` | Take all options |
| `/` | Answer in your own words |
| `esc` | None of these |

## Command line

| Command / flag | What it does |
|----------------|--------------|
| `aux auth login`, `logout`, `status` | Manage the Apple Music session |
| `aux auth lastfm` | Manage Last.fm scrobbling |
| `aux version` | Print the version |
| `--demo` | Built-in fake tracks, no account or internet needed |
| `--local`, `--music-dir` | Play local music files instead of Apple Music |
| `--no-update` | Skip the update check |
| `--no-discord` | Disable Discord Rich Presence |
| `--debug` | Debug logging |
| `--config` | Use another config file |

Release builds check GitHub for a newer release once a day and update themselves; `--no-update` turns this off. Builds from source report version `dev` and are never replaced.

## Data

- `~/.config/aux/config.json`: settings and tokens, written with `0600` permissions.
- `~/.config/aux/vibe-signals.jsonl`: your reactions (skips, finishes, ratings) that the agent learns from.
- `~/.config/aux/vibe-memory.md`: what the agent remembers about your taste.
- `~/.cache/aux/`: library cache, the developer token, agent session logs under `vibe/runs`, and the Widevine CDM under `cdm`.

## Credits

AUX is based on [vibez](https://github.com/simonepelosi/vibez) by Simone Pelosi, released under the MIT license. See `LICENSE` and `NOTICE`.

Some Easter egg scenes quote album artwork: *Blonde* by Frank Ocean, *My Beautiful Dark Twisted Fantasy* by Kanye West (paintings by George Condo), *The Dark Side of the Moon* by Pink Floyd (design by Hipgnosis), *Unknown Pleasures* by Joy Division (design by Peter Saville) and *IGOR* by Tyler, the Creator. All rights belong to their owners.

Apple Music is a trademark of Apple Inc. AUX is not affiliated with or endorsed by Apple.
