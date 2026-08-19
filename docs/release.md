# Cutting a public release

Only Erick (`ecouto`) can promote a `v*` tag. CI will refuse any other
actor. This is the owner keystroke that publishes `v0.2.0`.

## Checklist

1. `main` is green: `verify / verify` and `all-checks-passed`.
2. The tag MUST point at that `main` tip — not an older import commit.
   A tag on the wrong SHA ships an installer that cannot install itself.
3. `CHANGELOG.md` `[0.2.0]` lists what ships. Move it off "unreleased"
   when the tag is cut.
4. From a clean `main`:

   ```bash
   git tag -a v0.2.0 -m "v0.2.0"
   git push origin v0.2.0
   ```

5. Confirm `.github/workflows/release.yml` published a **non-draft**
   release (otherwise `/releases/latest` is 404 and curl/brew cannot
   resolve it):

   - `jacu_0.2.0_{darwin,linux}_{amd64,arm64}.tar.gz`
   - `checksums.txt`
   - `checksums.txt.sigstore.json`
   - `jacu.spdx.json`
   - `install.sh`

   Then refresh `Formula/jacu.rb` `url`/`sha256` for the four tarballs
   so `brew install` tracks the new tag.

6. On a machine that is not the dev laptop:

   ```bash
   curl -fsSL -o install.sh \
     https://raw.githubusercontent.com/jacu-dev/jacu-harness/v0.2.0/scripts/install.sh
   less install.sh
   bash install.sh --version v0.2.0
   jacu doctor
   jacu version
   ```

7. Keep that transcript as the installable-release report (SDD-017 T7).
   After it is green, G-10a (owner-only social pilot) is unblocked.

If `v0.2.0` was pushed on the wrong commit and no one has consumed the
assets yet, delete the tag and recut it on `main`:

```bash
git tag -d v0.2.0
git push origin :refs/tags/v0.2.0
git switch main
git pull --ff-only
git tag -a v0.2.0 -m "v0.2.0"
git push origin v0.2.0
```

`workflow_dispatch` on the release workflow is dry-run only. It never
publishes.
