---
name: jacu-inspect
description: 'Use when the user asks "what does this project do?" or requests project explanation or analysis without changing anything.'
---

# Inspect a project

For project explanation or analysis without changes, call
`jacu_project_inspect`. Answer from its structured result plus any files you
read. Never compile a mission or open a workspace for it.

When defining a requested change, use the inspected `project_id`, manifests,
languages, and test commands to define the change accurately, then continue
with `jacu-mission`.
