# Security

Please report vulnerabilities privately through GitHub: **Security → Report a vulnerability** on this repository. Do not open a public issue.

Include what you found, how to reproduce it, and the AUX version (`aux version`). You will get a reply within a week.

AUX stores your Apple Music session, LLM API key and Last.fm credentials in `~/.config/aux/config.json` with `0600` permissions. Never paste that file or the output of `--debug` into an issue without removing tokens first.
