// Detects source-text assertions in test files: `readFileSync` of a .ts/.tsx
// file followed by `expect(binding).toContain(...)` (or `.toMatch`/
// `.toContainEqual`) on that binding. These pass when the behavior they claim
// to cover is broken and fail when an unrelated variable is renamed — they
// are not tests of behavior. Zero-tolerance: any non-allowlisted flagged
// assertion fails the check. See
// agent_docs/plans/august_2026/unfalsifiable_test_gate.md and
// agent_docs/instructions/testability_escalation.md.
//
// Mirrors scripts/work_dot_bat/lib/check-monolith.mjs's shape: pure AST/classification
// functions over `root`, thin CLI at the bottom. Uses `ts.createSourceFile`
// (no type checker) — consistent with the structural-duplication gate's
// no-new-deps rule (the `typescript` package is already a dependency).

import { readFileSync, existsSync, readdirSync } from 'node:fs'
import { execFileSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import ts from 'typescript'

const TEST_FILE_RE = /\.test\.tsx?$/
const ASSERTION_METHODS = new Set(['toContain', 'toMatch', 'toContainEqual'])
const ALLOW_COMMENT_RE = /^\/\/\s*allow-source-assertion:?\s*(.*)$/

// ---------------------------------------------------------------------------
// AST scan. Pure function over source text, no disk I/O.
// ---------------------------------------------------------------------------

function isReadFileSyncCall(node) {
  if (!ts.isCallExpression(node)) return false
  const expr = node.expression
  if (ts.isIdentifier(expr)) return expr.text === 'readFileSync'
  if (ts.isPropertyAccessExpression(expr)) return expr.name.text === 'readFileSync'
  return false
}

// True if the given subtree directly contains a string literal ending in
// .ts/.tsx — covers `readFileSync('foo.tsx')` and
// `readFileSync(path.resolve(__dirname, 'foo.tsx'))`.
function containsSourceLiteral(node) {
  let found = false
  function visit(n) {
    if (found) return
    if (ts.isStringLiteralLike(n) && /\.tsx?$/.test(n.text)) {
      found = true
      return
    }
    ts.forEachChild(n, visit)
  }
  visit(node)
  return found
}

// name -> initializer, for every `const/let/var x = ...` in the file.
// Single-file, unscoped by design (matches the plan's "single-file scope, no
// type checker" simplicity — see the plan's Detection section).
function buildDeclMap(sourceFile) {
  const map = new Map()
  function visit(n) {
    if (ts.isVariableDeclaration(n) && ts.isIdentifier(n.name) && n.initializer) {
      map.set(n.name.text, n.initializer)
    }
    ts.forEachChild(n, visit)
  }
  visit(sourceFile)
  return map
}

// Resolves an argument expression to a source literal either directly
// (`readFileSync('foo.tsx')`) or through one level of variable indirection
// (`const filePath = path.resolve(__dirname, 'foo.tsx'); readFileSync(filePath, ...)`
// — the actual idiom used throughout Home3D.test.tsx).
function resolvesToSourceLiteral(node, declMap, seen = new Set()) {
  if (containsSourceLiteral(node)) return true
  if (ts.isIdentifier(node) && !seen.has(node.text)) {
    seen.add(node.text)
    const decl = declMap.get(node.text)
    if (decl) return resolvesToSourceLiteral(decl, declMap, seen)
  }
  return false
}

function lineOf(sourceFile, node) {
  return sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile)).line + 1
}

// A trailing `// allow-source-assertion: <reason>` on the read's statement.
// A bare `// allow-source-assertion` (no colon, or no text after it) is
// deliberately NOT allowlisted — a reason is mandatory.
function getAllowAnnotation(code, statementEnd) {
  const ranges = ts.getTrailingCommentRanges(code, statementEnd) || []
  for (const range of ranges) {
    const text = code.slice(range.pos, range.end)
    const match = text.match(ALLOW_COMMENT_RE)
    if (!match) continue
    const reason = match[1].trim()
    if (reason) return { present: true, allowlisted: true, reason }
    return { present: true, allowlisted: false, reason: null }
  }
  return { present: false, allowlisted: false, reason: null }
}

// Scans one test file's source text. Returns every flagged binding (a
// variable assigned the result of a source-reading readFileSync call) and
// every expect(...).toContain/toMatch/toContainEqual(...) assertion whose
// subject is one of those bindings.
export function scanTestFile(relPath, code) {
  const sourceFile = ts.createSourceFile(
    relPath,
    code,
    ts.ScriptTarget.Latest,
    true,
    relPath.endsWith('.tsx') ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
  )

  const bindings = new Map() // name -> { line, allowlisted, reason, bare }
  const declMap = buildDeclMap(sourceFile)

  function visitForReads(node) {
    if (isReadFileSyncCall(node) && node.arguments.length > 0 && resolvesToSourceLiteral(node.arguments[0], declMap)) {
      let n = node
      let bindingName = null
      let statement = null
      while (n) {
        if (ts.isVariableDeclaration(n) && ts.isIdentifier(n.name)) {
          bindingName = n.name.text
        }
        if (ts.isVariableStatement(n)) {
          statement = n
          break
        }
        n = n.parent
      }
      if (bindingName && statement) {
        const annotation = getAllowAnnotation(code, statement.end)
        bindings.set(bindingName, {
          line: lineOf(sourceFile, node),
          allowlisted: annotation.allowlisted,
          reason: annotation.reason,
          bare: annotation.present && !annotation.allowlisted,
        })
      }
    }
    ts.forEachChild(node, visitForReads)
  }

  const assertions = []
  function visitForAssertions(node) {
    if (ts.isCallExpression(node)) {
      const expr = node.expression
      if (ts.isPropertyAccessExpression(expr) && ASSERTION_METHODS.has(expr.name.text)) {
        // Unwrap a `.not` in between — `expect(X).not.toContain(...)` is the
        // same source-text assertion, just negated; the doctrine bans
        // asserting on source text at all, not only the positive form.
        let target = expr.expression
        if (ts.isPropertyAccessExpression(target) && target.name.text === 'not') {
          target = target.expression
        }
        if (
          ts.isCallExpression(target) &&
          ts.isIdentifier(target.expression) &&
          target.expression.text === 'expect' &&
          target.arguments.length > 0 &&
          ts.isIdentifier(target.arguments[0])
        ) {
          const name = target.arguments[0].text
          if (bindings.has(name)) {
            assertions.push({ name, method: expr.name.text, line: lineOf(sourceFile, node) })
          }
        }
      }
    }
    ts.forEachChild(node, visitForAssertions)
  }

  visitForReads(sourceFile)
  visitForAssertions(sourceFile)

  const flaggedAssertions = assertions.filter((a) => !bindings.get(a.name).allowlisted)
  const allowlistedAssertions = assertions.filter((a) => bindings.get(a.name).allowlisted)
  const bareAllowSites = [...bindings.entries()]
    .filter(([, b]) => b.bare)
    .map(([name, b]) => ({ name, line: b.line }))

  return {
    path: relPath,
    readSiteCount: bindings.size,
    flaggedCount: flaggedAssertions.length,
    allowlistedCount: allowlistedAssertions.length,
    assertions,
    flaggedAssertions,
    allowlistedAssertions,
    bareAllowSites,
  }
}

// ---------------------------------------------------------------------------
// Disk I/O: file listing, baseline read/write, classification.
// ---------------------------------------------------------------------------

export function listTestFiles(root) {
  const srcDir = path.join(root, 'src')
  const results = []

  function walk(dir) {
    if (!existsSync(dir)) return
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name)
      if (entry.isDirectory()) {
        walk(full)
      } else if (TEST_FILE_RE.test(entry.name)) {
        results.push(path.relative(root, full).split(path.sep).join('/'))
      }
    }
  }

  walk(srcDir)
  return results.sort()
}

export function scanRepo(root, files) {
  return files.map((relPath) => scanTestFile(relPath, readFileSync(path.join(root, relPath), 'utf8')))
}

// Pure classification, kept separate from disk I/O so it's directly
// testable. Zero-tolerance: any file with a flagged assertion is a
// violation, full stop — no baseline lookup, no "new" vs. "grown"
// distinction.
export function classify(scanned) {
  const violations = []
  for (const file of scanned) {
    if (file.flaggedCount > 0) {
      violations.push({ path: file.path, count: file.flaggedCount })
    }
  }
  return { violations }
}

export function check(root) {
  const scanned = scanRepo(root, listTestFiles(root))
  const { violations } = classify(scanned)

  const allowlisted = scanned
    .filter((f) => f.allowlistedCount > 0)
    .map((f) => ({
      path: f.path,
      count: f.allowlistedCount,
      reasons: f.allowlistedAssertions.map((a) => a.name),
    }))
  const bare = scanned
    .filter((f) => f.bareAllowSites.length > 0)
    .map((f) => ({ path: f.path, sites: f.bareAllowSites }))
  return { violations, allowlisted, bare, scanned }
}

// ---------------------------------------------------------------------------
// I/O + CLI. Kept thin so the logic above stays pure and unit-testable.
// ---------------------------------------------------------------------------

export function repoRoot() {
  return execFileSync('git', ['rev-parse', '--show-toplevel'], { encoding: 'utf8' }).trim()
}

export function main(argv) {
  const root = repoRoot()
  const [cmd] = argv

  try {
    switch (cmd) {
      case 'check': {
        const result = check(root)

        if (result.allowlisted.length > 0) {
          process.stdout.write(`Allowlisted source-text assertions (${result.allowlisted.reduce((n, f) => n + f.count, 0)}):\n`)
          for (const f of result.allowlisted) {
            process.stdout.write(`  - ${f.path} (${f.count})\n`)
          }
        }
        if (result.bare.length > 0) {
          process.stdout.write(`Bare "allow-source-assertion" comments with no reason (rejected):\n`)
          for (const f of result.bare) {
            for (const site of f.sites) process.stdout.write(`  ! ${f.path}:${site.line} (${site.name})\n`)
          }
        }

        if (result.violations.length === 0) {
          process.stdout.write('Source-assertion check passed.\n')
          return 0
        }
        process.stdout.write(`Source-assertion check found ${result.violations.length} file(s) with flagged assertions:\n`)
        for (const v of result.violations) {
          process.stdout.write(`  ${v.path} (${v.count} flagged assertion(s))\n`)
        }
        process.stdout.write(
          '\nEither remove the source-text assertion (write a behavioral test instead — see ' +
            'agent_docs/instructions/testability_escalation.md) or mark it legitimate with ' +
            '`// allow-source-assertion: <reason>`.\n',
        )
        return 1
      }
      default:
        process.stderr.write(`Unknown command: "${cmd}"\nUsage: check-source-assertions.sh <check>\n`)
        return 1
    }
  } catch (err) {
    process.stderr.write(`${err.message}\n`)
    return 1
  }
}

// Run when invoked directly (not when imported by tests).
if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  process.exit(main(process.argv.slice(2)))
}
