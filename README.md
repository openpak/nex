# nex

The shared NEX host every OpenPak NEX title server imports. Thin: Pretendo's AGPL \`nex-go\`/\`nex-protocols-go\` do PRUDP and the standard protocols; this adds OpenPak account and session integration, the per-title port block convention, and the deploy shape. **Not a from-scratch NEX implementation.**

**Status: empty.** Nothing is implemented yet. This repository exists so the work has a
home, a port block and a deploy path; treat every claim below as a plan, not a result.

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](../../docs/clean-room-policy.md) before
  writing a line. Public protocol documentation, our own captures, and compatibly licensed
  code are the only inputs.
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](../../ports.md). Claim your block in the PR that first
  needs it.

This project is not affiliated with or endorsed by Nintendo, NEX's publisher, or any
original service provider. It contains no proprietary game code, assets, SDK material,
binaries, certificates, keys, firmware, extracted files, credentials, or raw packet captures.
