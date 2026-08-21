# Net-cost protocol

This protocol is the prerequisite for any claim that JACU reduced cost.
It does not contain measured n, arms, or p-values. Filling those is
blocked on G-T (one week without JACU). Citing `stats` as a gain before
that fill is refused.

## n

n is the number of paired tasks in the corpus. It is not filled here.
Declare n before analysis. Do not peek at `stats` to choose n. A later
power note may raise n; it may not shrink a declared n after seeing
outcomes.

## Arms

Two arms, paired on the same tasks:

1. **without JACU** — G-T baseline week.
2. **with JACU** — same corpus after the baseline, using the shipped
   CLI/MCP surfaces.

No third arm. No counterfactual "would have" column. Telemetry fields
that invent savings without a named baseline stay forbidden.

## Corpus

The corpus is a frozen list of tasks with:

- an objective
- a write-scope
- verify commands
- a binary quality criterion (pass or fail)

The list is owner-authored. This document does not invent it.

## Quality criterion

A task counts as quality-pass only when all of the following hold:

- mission verify verdict is `pass`
- write-scope holds (no out-of-scope path)
- provenance scan of the change is clean
- the owned quality.json audit is schema-valid

Human preference is not a substitute for that tuple.

## Statistical test

Lock the test before seeing outcomes:

- Primary: Wilcoxon signed-rank on paired human-time (minutes from
  first keystroke to merge-ready), two-sided, alpha 0.05.
- Secondary (descriptive only): first-pass verify rate and remediation
  iterations from local telemetry. These are not a gain claim without
  G-T and without n.

If the signed-rank assumptions fail, stop. Do not shop for a test that
produces a star.

## Refusal

`jacu stats` output, MCP catalogue headroom, and file-size ratchets are
engineering gates. They are not net-cost evidence. A gain sentence that
lacks n, arms, corpus, criterion, test, and G-T is refused.
