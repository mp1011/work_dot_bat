---
status: Done
---

# GOAL

Determine whether or not we can use the structural duplication check, or if its too heavily tied to TypeScript to be generally useful.

It currently does not work correctly. If it is not currently generic enough to use for other languages, then show me how to configure projects to not use it.

# FINDINGS

**Verdict: TypeScript-only; not generic. Disable it for other languages.**

Evidence:
- `.tools/typescript/duplication-common.mjs` `isTrackedSourceFile` accepts only `.ts`/`.tsx`, and `listSourceFiles` only looks under `src/` (`git ls-tree ... HEAD -- src`).
- `.tools/typescript/check-structural-duplication.mjs` tokenizes with `ts.createScanner` (TypeScript's scanner: JSX, template literals), so the normalization is TS grammar, not a language-neutral one.
- It runs under Node and `import ts from 'typescript'` resolves from the *target repo's* `node_modules`. Without that package it dies with `ERR_MODULE_NOT_FOUND` (reproduced here in work_dot_bat itself). Likely cause of "does not work correctly" in non-TS projects. `bootstrap.PrerequisiteNotes` already warns about this.
- When `checks` is absent from `work_dot_bat.config.json`, **all** tool checks are enabled (`runchecks.go` BuildCheckList uses `projectconfig.ToolCheckIDs`), so non-TS projects get it by default.
- The native `duplication` check is also `.ts/.tsx`-only (`trackedTSRE`); on a non-TS repo it sees zero files and passes vacuously (harmless, but meaningless). `source-assertions` is likewise Node/TypeScript-based.

How to disable (per project `work_dot_bat.config.json`) — list only the checks you want:

```json
{ "checks": ["plan-drift", "monolith", "skills"] }
```

(Valid ids: plan-drift, monolith, duplication, structural-duplication, source-assertions, skills, audit. `"checks": []` disables all, as this repo does.) The same `checks` key can be set per entry in `projects.json`. For a non-TS repo, also omit `duplication` and `source-assertions`. Optionally delete `agent_docs/structural_duplication_baseline.json`; bootstrap only creates baselines for enabled checks.

Possible follow-up (not done): make `checks` default language-aware, or make the file set/tokenizer configurable.
