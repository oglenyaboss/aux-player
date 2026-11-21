# ChatGPT subscription as a picker provider

The picker can run on a ChatGPT plan instead of a paid API key. It signs in with the public OAuth client of the Codex CLI (browser, PKCE, a localhost callback on port 1455) and calls the Codex Responses endpoint `chatgpt.com/backend-api/codex/responses`, so requests count against the plan's 5-hour and weekly limits.

## Decision

- The sign-in is aux's own. Tokens live in `~/.config/aux/chatgpt-auth.json` (0600), apart from `config.json` and apart from `~/.codex/auth.json`, whose refresh token Codex rotates and which aux must not touch.
- A second LLM client speaks the Responses API over SSE with `store: false` and replays the returned output items, encrypted reasoning included. It sits behind the same `Chat` seam as the chat-completions client, so the agent does not know which one it talks to.
- The model list comes from the endpoint's `/models`, filtered by a pinned Codex client version.
- Settings marks the provider as unofficial: "via the Codex sign-in; OpenAI may restrict the account".

## Considered options

- **Reuse `~/.codex/auth.json`.** Rejected: refreshing would rotate Codex's refresh token and sign the user out of Codex, and the user would need Codex installed.
- **OpenAI API key.** Still offered as its own preset; it bills per token and ignores the subscription.
- **Device-code login.** Deferred (`ponytail:`). It helps on machines without a browser, which aux does not target today.

## Consequences

- The endpoint is undocumented and can change or be closed. The pinned client version and the `originator` header may need updates when Codex moves on.
- OpenAI may restrict accounts that use the Codex client outside Codex. That risk is the user's, and the warning line says so.
