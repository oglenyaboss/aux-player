# The picker informs, the model decides

The picker used to be a generator with a filter: the model named tracks from memory, the code checked them against the catalog, the library, the listening signals and a set of taste limits, and rejected what failed. The model could not see most of that data, so it guessed, and every turn started from an empty conversation. On 51 recorded sessions (26 portions) the code rejected 239 candidates for reasons other than a full portion, about 9 per portion: 113 known tracks, 52 not found in Apple Music, 52 for window limits, 22 for past skips. The same candidates came back turn after turn.

## Decision

- **Code checks facts, the model judges taste.** The code keeps exactly two checks on a candidate: the track exists in Apple Music, and it has not played in this Stream yet (unless the listener asked to hear it again). Every rule about taste leaves the code: known tracks, the per-artist, referent and known-artist limits of the Window, Favorites mode, `not_it`, Exclusions and the rejections from past skips.
- **The model sees what the code knows.** A batch `search_tracks` tool returns real catalog hits with marks: in the library, in Favorites, rated ▲/▼, played N times and when, how the last plays ended. `submit_candidates` takes the ids it returns.
- **One conversation per Stream.** The agent keeps its whole history between turns. What happens between turns (outcomes, ratings, replies) is collected and sent as one message at the start of the next turn. A cancelled turn rolls back to the last complete one. Compaction exists only as a guard near a configurable context limit (200k tokens by default).
- **Listening history includes manual plays.** A track the listener plays to the end outside the agent counts as a neutral play. Skips outside the agent are not recorded.
- **Repetition is the model's call.** Hearing the same tracks across sessions is fine; fatigue shows up as skips on tracks that used to play through, and the marks make that visible.

## Considered options

- **Keep the rules and feed rejections back into the next turn.** Cheapest, but the model keeps guessing blind and the rules keep fighting requests such as "something new", where loved tracks and a new genre can both belong.
- **A pool built by the code, the model only picks.** Removes rejections entirely, but loses deep cuts and connections that only the model knows ("rappers who sample Tame Impala"). Search complements memory instead of replacing it.
- **A cooldown on repeated tracks in code.** Rejected by the owner: repeating loved tracks is wanted, skipping is the signal.

## Consequences

- Picks depend more on the model following the prompt. Weak models may break an Exclusion; the pilot measures it per provider.
- Token use per session grows with its length. Every step logs input, cached, output and reasoning tokens, and optimisation waits for that data.
- The Window and Known track entries in `GLOSSARY.md` lose their limits.
