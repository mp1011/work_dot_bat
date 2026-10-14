---
status: Done
---

# Make the mechanical gate support configurable scan roots and extensions (e.g. C# projects)

## Goal
`check-monolith` and `check-duplication` support configurable scan roots, file extensions, and directory exclusions per project so non-TypeScript projects (such as .NET/C# projects like BackStage/ExploreGame) are measured properly by the mechanical gate.

## Context
- `check-monolith` (`src/internal/checks/monolith.go`) previously hardcoded walking `src/` and only matched `.ts`/`.tsx`/`.css`.
- `check-duplication` (`src/internal/checks/duplication.go`) previously hardcoded `git ls-tree ... -- src` and only matched `.ts`/`.tsx`.
- Projects without a `src/` folder (such as BackStage, which uses `ExploringGame/`, `ExploringGame.UnitTests/`, `ExploringGame.IntegrationTests/`) reported passing on an empty file set and could not enforce monolith or duplication gates on `.cs` or `.fx` source.

## Requirements
1. **Configurable Scan Roots and Extensions**:
   - `scanDirs` (or `scanRoots`), `scanExtensions`, and `excludeDirs` are configurable per project in `projects.json` and `work_dot_bat.config.json`.
   - Defaults preserve today's behavior for unconfigured repositories (`src/**/*.ts` with default excludes like `generated`, `bin`, `obj`).
2. **Monolith Check Support**:
   - `monolith.go` walks the configured roots, respects excluded directory names (`bin`, `obj`, `generated`), matches configured extensions, and generates clean breakup plan paths for any file extension.
3. **Duplication Check Support**:
   - `duplication.go` checks git-tracked files in configured roots, filters by configured extensions, and excludes paths under excluded directory segments.
4. **Configuration Integration**:
   - `Project` in `projects.go` and `Config` in `projectconfig.go` and `runchecks.go` validate, serialize, and resolve the new fields.
5. **BackStage / C# Compatibility**:
   - BackStage (`ExploreGame`) configured in `projects.json` with scan roots (`ExploringGame`, `ExploringGame.UnitTests`, `ExploringGame.IntegrationTests`), extensions (`.cs`, `.fx`), and excluded dirs (`bin`, `obj`).
   - Baselines in BackStage updated to record real source metrics.

## Implementation Plan
1. Add `ScanConfig` and `ReadScanConfig` in `src/internal/checks/scanconfig.go` resolving `scanDirs`/`scanRoots`, `scanExtensions`, and `excludeDirs` via `projectconfig.ForProject`.
2. Update `monolith.go` and `duplication.go` to use `ScanConfig`.
3. Update `projects.go`, `projectconfig.go`, and `runchecks.go` with schema fields and validation.
4. Update `BackStage` configuration in `projects.json`.

## Testing Plan
- Unit tests in `src/internal/checks/checks_test.go`:
  - `TestListMonolithFilesConfiguredScan`
  - `TestListTrackedSourceFilesConfiguredScan`
  - `TestBreakupPlanPathCSharp`
  - Existing unconfigured tests ensure `src/**/*.ts` backwards compatibility.
- Unit tests in `src/internal/projectconfig/projectconfig_test.go`:
  - Validation tests for `scanDirs`, `scanRoots`, `scanExtensions`, `excludeDirs`.
- Verification against BackStage (`ExploreGame`):
  - `check-monolith check` flags oversized files.
  - Temporary 501-line `.cs` file fails the gate.
  - Baselines updated with accepted reasons.
  - `check-monolith check` and `check-duplication check` pass.

## Verification
- `go test ./...` and `go vet ./...` pass.
- Verified in BackStage that `check-monolith` and `check-duplication` scan C# and shader files properly and pass with updated baselines.
