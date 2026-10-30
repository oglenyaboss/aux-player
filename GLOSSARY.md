# AUX picker

Music picking from a free-text request: the model names candidates, live data checks and filters them.

## Request

**Request**:
The listener's free text about what they want to hear right now.
_Avoid_: vibe, prompt, search query

**Referent**:
An artist, album or track from the request that sets the direction of the selection; how much of it plays depends on the request.
_Avoid_: seed

**Route**:
The way to find candidates from the referent: through artists who are neighbors by shared audience, or through shared tags of albums or tracks.
_Avoid_: strategy, algorithm

**Mood**:
The part of the request about the character of the music or the situation of listening, without artist names.
_Avoid_: vibe

**Exclusion**:
An artist or genre that the request explicitly banned for the whole stream.
_Avoid_: blacklist, filter

## Familiarity

**Known track**:
A track that is already in the listener's library, including other versions of the same track.
_Avoid_: played track

**Known artist**:
An artist with at least one track starred (Favorites).
_Avoid_: favorite artist

**New artist**:
Any artist that is not known, even if their tracks are in the library.

## Output

**Stream**:
An endless sequence of tracks from one request without repeats.
_Avoid_: station, radio

**Drift**:
The stream moving on to artists similar to the ones similar to the referent; no further than one step (no longer in the prompt).

**Portion**:
The tracks the agent adds to the stream in one turn; 5 for now.
_Avoid_: batch, pack

**Window**:
The last 20 tracks of the stream, over which the selection limits are counted: per artist, per referent, per known artist.

**Card**:
An agent question where every option is a track with artwork; the chosen track plays at once.
_Avoid_: poll, questionnaire

## Memory

**Memory**:
Entries about the listener's preferences that the picker accumulates between sessions from their explicit signals: what they like, what they do not, which connections between music hold.
_Avoid_: profile, history
