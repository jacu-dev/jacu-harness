# Security

## Reporting

Report vulnerabilities privately via
[GitHub private vulnerability reporting](https://github.com/jacu-dev/jacu-harness/security/advisories/new).
Do not open a public issue for a credential or an exploitable defect.

Expect an acknowledgment within 72 hours. Coordinated disclosure: the report
stays private until a fix ships or 90 days pass, whichever comes first.

## Supported versions

Only the latest release receives fixes. There are no backports.

## Boundary

JACU does not speak to the network. Installers download release assets; the
binary does not. The full threat model, including what is guaranteed versus
cooperative, lives in [docs/threat-model.md](docs/threat-model.md).

## Release integrity

Release assets are verified against a checksum manifest signed with Sigstore
(GitHub OIDC, `release.yml@refs/tags/v*` identity) before they are written.
A mismatched tarball must be refused. `scripts/install.sh` performs this
verification; if you download by hand, verify `checksums.txt` against
`checksums.txt.sigstore.json` with cosign before trusting an asset.
