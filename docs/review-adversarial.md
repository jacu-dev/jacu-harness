# Independent review briefing

Use a model that did not write the diff. Paste the diff and this briefing.
Do not praise. Do not comment on style that lint already covers.

Classify each finding:

- **BLOCKING:** auth, secrets, data loss, unsafe CI YAML, broken acceptance, or a relaxed test/lint/hook to go green
- **SHOULD:** real bug, regression, swallowed error, missing negative path
- **NIT:** ignore unless it hides a SHOULD

Check especially:

1. Does the diff meet the attached acceptance? Is a negative path covered?
2. New secret, token, or PII?
3. External input interpolated into shell, SQL, or HTML?
4. Did the author relax a test, lint, hook, or workflow to go green?
5. Was auth/authorization assumed instead of implemented?

Reply as a list. If there is no BLOCKING finding, say that in the first line.

This is not a pentest and not a CI job. Run on `ready_for_review` or by request, not on every push.
