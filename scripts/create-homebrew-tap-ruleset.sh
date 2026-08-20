#!/usr/bin/env bash
# Owner-only: create the homebrew-jacu main ruleset. The cloud token cannot
# write rulesets (403). Run this from an owner `gh auth` session.
set -euo pipefail

repo="${1:-jacu-dev/homebrew-jacu}"

existing="$(gh api "repos/${repo}/rulesets" --jq '.[] | select(.name=="main-tap-protect") | .id' || true)"
if [ -n "$existing" ]; then
  echo "create-homebrew-tap-ruleset: main-tap-protect already exists ($existing)"
  exit 0
fi

gh api --method POST "repos/${repo}/rulesets" --input - <<'EOF'
{
  "name": "main-tap-protect",
  "target": "branch",
  "enforcement": "active",
  "bypass_actors": [],
  "conditions": {
    "ref_name": {
      "include": ["~DEFAULT_BRANCH"],
      "exclude": []
    }
  },
  "rules": [
    {"type": "deletion"},
    {"type": "non_fast_forward"},
    {
      "type": "pull_request",
      "parameters": {
        "required_approving_review_count": 1,
        "dismiss_stale_reviews_on_push": true,
        "required_reviewers": [],
        "require_code_owner_review": true,
        "dismissal_restriction": {
          "enabled": false,
          "allowed_actors": []
        },
        "require_last_push_approval": true,
        "required_review_thread_resolution": false,
        "allowed_merge_methods": ["squash"]
      }
    },
    {
      "type": "required_status_checks",
      "parameters": {
        "strict_required_status_checks_policy": true,
        "do_not_enforce_on_create": false,
        "required_status_checks": [
          {"context": "verify"}
        ]
      }
    }
  ]
}
EOF

echo "create-homebrew-tap-ruleset: OK"
