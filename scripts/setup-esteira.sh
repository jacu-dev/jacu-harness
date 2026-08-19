#!/usr/bin/env bash
# Configuração one-shot do modelo ADR-007 (rodar VOCÊ, Erick, com gh autenticado
# na sua conta — é a sua permissão de admin que os rulesets vão proteger).
# Idempotência: rodar de novo falha ao criar ruleset duplicado; delete antes em
# Settings → Rules → Rulesets se precisar recriar.
set -euo pipefail

REPO="jacu-dev/jacu"

echo "==> Auto-merge + delete branch on merge"
gh api -X PATCH "repos/$REPO" \
  -F allow_auto_merge=true \
  -F delete_branch_on_merge=true >/dev/null

echo "==> Ruleset: main (PR obrigatório, 0 approvals, esteira como gate)"
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
          { "context": "verify" },
          { "context": "lint" },
          { "context": "vuln" },
          { "context": "mod-hygiene" },
          { "context": "mcp-smoke" },
          { "context": "secrets" }
        ]
      }
    }
  ],
  "bypass_actors": []
}
JSON

echo "==> Ruleset: tags v* (criar/mover/deletar + assinatura — só admin da org)"
# Bypass = OrganizationAdmin (você). A máquina/executor NUNCA entra como admin.
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
    { "actor_id": 1, "actor_type": "OrganizationAdmin", "bypass_mode": "always" }
  ]
}
JSON

echo "==> Pronto. Confira em Settings → Rules → Rulesets."
echo "    Falta manual: confirmar sua chave GPG para criar tags assinadas com tag -s."
