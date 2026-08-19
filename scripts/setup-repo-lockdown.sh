#!/usr/bin/env bash
# One-shot lockdown for jacu-dev/jacu-harness (run locally, gh authenticated
# as the owner — it is your admin permission the rulesets protect).
#
# Replaces the stale scripts/setup-esteira.sh, which pointed at the old repo
# name (jacu-dev/jacu) and required six check contexts that no longer exist:
# ci.yml now emits a single required context, `verify / verify`.
#
# Idempotency: creating a duplicate ruleset fails; inspect first with --check
# and delete in Settings → Rules → Rulesets before recreating.
set -euo pipefail

REPO="jacu-dev/jacu-harness"

if [[ "${1:-}" == "--check" ]]; then
  echo "==> Repo flags"
  gh api "repos/$REPO" --jq '{has_wiki, has_projects, has_discussions, allow_auto_merge, delete_branch_on_merge, web_commit_signoff_required, security_and_analysis}'
  echo "==> Rulesets"
  gh api "repos/$REPO/rulesets" --jq '.[] | {id, name, target, enforcement}'
  echo "==> Active rules on main"
  gh api "repos/$REPO/rules/branches/main"
  echo "==> Collaborators (should be only you as admin)"
  gh api "repos/$REPO/collaborators" --jq '.[] | {login, role_name}'
  echo "==> Deploy keys / webhooks (should be empty or known)"
  gh api "repos/$REPO/keys" --jq 'length'
  gh api "repos/$REPO/hooks" --jq 'length'
  exit 0
fi

echo "==> Repo flags: auto-merge, delete branch on merge, no wiki/projects"
gh api -X PATCH "repos/$REPO" \
  -F allow_auto_merge=true \
  -F delete_branch_on_merge=true \
  -F has_wiki=false \
  -F has_projects=false \
  -F web_commit_signoff_required=false >/dev/null

echo "==> Security: secret scanning + push protection + private vuln reporting + dependabot"
gh api -X PATCH "repos/$REPO" --input - <<'JSON' >/dev/null
{
  "security_and_analysis": {
    "secret_scanning": { "status": "enabled" },
    "secret_scanning_push_protection": { "status": "enabled" }
  }
}
JSON
gh api -X PUT "repos/$REPO/private-vulnerability-reporting" >/dev/null
gh api -X PUT "repos/$REPO/vulnerability-alerts" >/dev/null
gh api -X PUT "repos/$REPO/automated-security-fixes" >/dev/null

echo "==> Actions: restrict to actions in this repo + verified creators, default GITHUB_TOKEN read-only"
gh api -X PUT "repos/$REPO/actions/permissions" \
  -F enabled=true -F allowed_actions=selected >/dev/null
gh api -X PUT "repos/$REPO/actions/permissions/selected-actions" \
  -F github_owned_allowed=true \
  -F verified_allowed=true >/dev/null 2>&1 \
  || echo "    (selected actions ja gerenciadas no nivel da org; seguindo)"
gh api -X PUT "repos/$REPO/actions/permissions/workflow" \
  -F default_workflow_permissions=read \
  -F can_approve_pull_request_reviews=false >/dev/null

echo "==> Ruleset: main (PR required, 0 approvals, verify as the gate)"
gh api -X POST "repos/$REPO/rulesets" --input - <<'JSON' >/dev/null
{
  "name": "main-esteira-adr-007",
  "target": "branch",
  "enforcement": "active",
  "conditions": { "ref_name": { "include": ["~DEFAULT_BRANCH"], "exclude": [] } },
  "rules": [
    { "type": "deletion" },
    { "type": "non_fast_forward" },
    {
      "type": "pull_request",
      "parameters": {
        "required_approving_review_count": 0,
        "dismiss_stale_reviews_on_push": false,
        "require_code_owner_review": false,
        "require_last_push_approval": false,
        "required_review_thread_resolution": false,
        "allowed_merge_methods": ["merge", "squash"]
      }
    },
    {
      "type": "required_status_checks",
      "parameters": {
        "strict_required_status_checks_policy": true,
        "required_status_checks": [
          { "context": "verify / verify" },
          { "context": "all-checks-passed" }
        ]
      }
    }
  ],
  "bypass_actors": []
}
JSON

echo "==> Ruleset: tags v* (create/move/delete + signature — owner only)"
gh api -X POST "repos/$REPO/rulesets" --input - <<'JSON' >/dev/null
{
  "name": "prod-tags-adr-007",
  "target": "tag",
  "enforcement": "active",
  "conditions": { "ref_name": { "include": ["refs/tags/v*"], "exclude": [] } },
  "rules": [
    { "type": "creation" },
    { "type": "update" },
    { "type": "deletion" },
    { "type": "required_signatures" }
  ],
  "bypass_actors": [
    { "actor_id": 5, "actor_type": "RepositoryRole", "bypass_mode": "always" }
  ]
}
JSON

echo "==> Done. Verify with: $0 --check"
echo "    Manual leftovers:"
echo "    - Settings → General: confirm forks of this repo cannot run privileged workflows (public repo default is safe)."
echo "    - Settings → Moderation: interaction limits if spam appears."
echo "    - Confirm your GPG/SSH signing key so v* tags satisfy required_signatures."
