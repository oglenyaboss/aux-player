# Changelog

AUX started as a fork of [vibez](https://github.com/simonepelosi/vibez) by Simone Pelosi.
The vibez history up to the fork is in git.

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
