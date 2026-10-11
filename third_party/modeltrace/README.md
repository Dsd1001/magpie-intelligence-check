# ModelTrace Attribution

Source: https://github.com/xqy2006/ModelTrace

Pinned commit: `6cd4a8ff0f3f4e49d6d6969dd2fbadfa1569fde2`

License: MIT, Copyright (c) 2026 xqy2006. See LICENSE in this directory.

Vendored unmodified:

- `static/fingerprint-core.js` -> `internal/check/web/modeltrace/fingerprint-core.js`
  SHA-256: `83fa5bd611e18f8339122582335123c8ea168ed242298bb31f4e363abeeb6e4a`
- `static/data/unified_bank.json` -> `internal/check/web/modeltrace/unified_bank.json`
  SHA-256: `f267d0513aa06981d024ec5394746fceddf44e4e8a75fa6bf8304255f4b32d6d`

`internal/check/modeltrace.go` ports `fingerprint.py:generate_challenges`.
The bundled bank contains 18 candidates; probabilities are closed-set rankings,
not authenticated model identities. The scorer runs locally in the browser.
No Codex hooks, fork monitoring, Python server, or bank enrollment is included.
