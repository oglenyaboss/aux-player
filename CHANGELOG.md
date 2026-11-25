# Changelog

AUX started as a fork of [vibez](https://github.com/simonepelosi/vibez) by Simone Pelosi.
The vibez history up to the fork is in git.

## [1.4.1] — 2026-11-25

Press `Enter` on an album in search to play it from the first track. This build includes the [1.4 features](https://github.com/oglenyaboss/aux-player/blob/v1.4.1/CHANGELOG.md#140--2026-11-25) and fixes a Windows test that read listening history from outside its temporary directory.

## [1.4.0] — 2026-11-25

- Press `Enter` on an album in search to play it from the first track. Use `Ctrl+K` → `Open` to browse its tracks without interrupting playback, or `Ctrl+E` to queue the album. `Esc` returns from the tracklist to the selected search result.
- The picker can search Apple Music before choosing tracks and keeps the conversation between portions of a stream.
- Ask for a playlist to save a finished set to Apple Music instead of starting a stream.
- Settings now let you edit the picker's memory and track ratings, switch between provider profiles, and sign in with a ChatGPT subscription.

## [1.3.0] — 2026-11-14

- `aux update` checks for a newer release and installs it right away, without waiting for the once-a-day check.

## [1.2.0] — 2026-11-14

- Music screen: search the catalog and your library, browse Favorites, albums and playlists, and play them without the agent. Open it with `ctrl+o` on the intro or `m` in the player; `ctrl+e` appends a track to the queue. Without the agent a queue ends where it ends.
- The agent is optional: Apple Music alone is enough to listen, and the agent can be set up later.
- The agent can continue after your choice: `ctrl+n` on the Music screen turns it on, and the agent's tracks join once your album, playlist or track has played. Your own picks never count as ratings or go into the agent's memory.
- Add the playing or a recently played track to the Apple Music library (`l`) or Favorites (`f`); `h` lists what played this launch.
- Your library is kept on disk and checked in the background, so the Music screen and the agent start faster.
- Replays come from an encrypted on-disk audio cache (2 GB by default, `audio_cache_mb` in `config.json`); the license is still requested on every play.

## [1.1.0] — 2026-11-04

- The scan is now a turntable: each kept track arrives as a sleeve with its real cover, its record is laid on the platter, and the needle drops into the player once the side is full.
- Rejected picks show their real covers, and picks past a full portion stay off the screen.
- Playback no longer stops after a long pause: an expired license is renewed on the fly instead of failing the track.
- Fixed a stream crash on the short closing segment of some tracks.
- Fixed a broken timecode when seeking forward in a track of unknown length, and a crash when playback started over an empty queue.

## [1.0.0] — 2026-10-31

First public release.

- An AI agent picks the music: describe a vibe in free text, answer an optional question with track cards, and listen to portions of five tracks that adapt to your skips, finishes and ratings.
- The agent keeps a memory of your stable preferences between sessions and builds bridges to your Apple Music favorites.
- Save the queue as an Apple Music playlist.
- Animated stage with album Easter eggs (can be switched off in settings).
- Playback on Linux without a browser, through an in-process Widevine CDM and GStreamer.
- No Apple Developer account needed: AUX uses the Apple Music web player's public developer token.
- English and Russian interface; the agent answers in the interface language.
- Optional Last.fm scrobbling, MPRIS and Discord Rich Presence.
- Prebuilt releases for Linux amd64 and arm64 with a checksum-verified install script and self-update.
