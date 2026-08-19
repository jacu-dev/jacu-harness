# Cutting a public release

Only Erick (`ecouto`) can promote a `v*` tag. CI will refuse any other
actor. This is the owner keystroke that publishes `v0.2.0`.

## Checklist

1. `main` is green: `verify / verify` and `all-checks-passed`.
2. `CHANGELOG.md` `[0.2.0]` lists what ships. Move it off "unreleased"
   when the tag is cut.
3. From a clean `main`:

   ```bash
   git tag -a v0.2.0 -m "v0.2.0"
   git push origin v0.2.0
   ```

4. Confirm `.github/workflows/release.yml` published:

   - `jacu_0.2.0_{darwin,linux}_{amd64,arm64}.tar.gz`
   - `checksums.txt`
   - `checksums.txt.sigstore.json`
   - `jacu.spdx.json`

5. On a machine that is not the dev laptop:

   ```bash
   curl -fsSL -o install.sh \
     https://raw.githubusercontent.com/jacu-dev/jacu-harness/v0.2.0/scripts/install.sh
   less install.sh
   bash install.sh --version v0.2.0
   jacu doctor
   jacu version
   ```

6. Keep that transcript as the installable-release report (SDD-017 T7).
   After it is green, G-10a (owner-only social pilot) is unblocked.

`workflow_dispatch` on the release workflow is dry-run only. It never
publishes.
