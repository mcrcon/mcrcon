# Security policy

## Supported versions

| Version | Supported |
| ------- | --------- |
| Latest `v1.x` release | ✅ |
| Older releases | ❌ (please upgrade) |

## Reporting a vulnerability

**Do not open a public issue** for suspected vulnerabilities. Instead, use
GitHub's **Private vulnerability reporting** on the repository's Security
tab (preferred), so details stay confidential until a fix is released.

Please include:

- mcrcon version and OS/arch
- Steps to reproduce or a proof of concept
- Impact assessment, if known

You can expect an initial response within 7 days. Once fixed, a patched
release will be published and credited (unless you prefer to stay anonymous).

## Notes for operators

RCON itself is unencrypted, so treat the network path as hostile:

- Bind `rcon.port` to localhost or a trusted network and firewall it.
- Use a long random `rcon.password`, rotated periodically.
- Prefer the `MCRCON_PASSWORD` environment variable over `-p` so the
  secret never appears in process listings or shell history.
- Never commit `server.properties`, shell history, or logs containing the
  password.
