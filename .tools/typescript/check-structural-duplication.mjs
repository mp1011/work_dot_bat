// work_dot_bat managed file — DO NOT EDIT; overwritten by `work_dot_bat sync`.
// file: scripts/work_dot_bat/typescript/check-structural-duplication.mjs
// version: 1.0.0
// date: 2026-10-03
// sha256: b358ac9d92c06be2ab1205a110497f7bccbd73520984596ba03a8c370972d4d2
// Detects token-normalized structural duplication across tracked TypeScript/TSX code
// and blocks push if the count exceeds a checked-in baseline.
// See agent_docs/plans/august_2026/structural_duplication_detection.md.

import { readFileSync, existsSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import ts from 'typescript'
import {
  isTrackedSourceFile,
  listSourceFiles,
  extractWindows as extractWindowsCommon,
  findDuplicates,
  topFilePairs,
  repoRoot,
  TOP_PAIRS_LIMIT,
} from './duplication-common.mjs'
import {
  tokenizeDetailed,
  findCoalescedClones,
  classifyClone,
  main as runReportMain,
} from './report-structural-duplication.mjs'

export {
  isTrackedSourceFile,
  listSourceFiles,
  findDuplicates,
  topFilePairs,
  repoRoot,
  TOP_PAIRS_LIMIT,
}

export function topClonePairs(clones, limit = TOP_PAIRS_LIMIT) {
  const pairCounts = new Map()
  for (const c of clones) {
    const pair = `${c.fileA} <-> ${c.fileB}`
    pairCounts.set(pair, (pairCounts.get(pair) ?? 0) + 1)
  }
  return [...pairCounts.entries()]
    .map(([pair, count]) => ({ pair, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, limit)
}

export const WINDOW_SIZE = 50

export function tokenizeAndNormalize(code, isJsx = false) {
  const variant = isJsx ? ts.LanguageVariant.JSX : ts.LanguageVariant.Standard
  const scanner = ts.createScanner(ts.ScriptTarget.Latest, true, variant, code)
  let token = scanner.scan()
  const tokens = []
  const templateStack = []

  while (token !== ts.SyntaxKind.EndOfFileToken) {
    if (token === ts.SyntaxKind.TemplateHead || token === ts.SyntaxKind.TemplateMiddle) {
      templateStack.push(0)
    } else if (templateStack.length > 0) {
      if (token === ts.SyntaxKind.OpenBraceToken) {
        templateStack[templateStack.length - 1]++
      } else if (token === ts.SyntaxKind.CloseBraceToken) {
        if (templateStack[templateStack.length - 1] === 0) {
          token = scanner.reScanTemplateToken()
          if (token === ts.SyntaxKind.TemplateTail) {
            templateStack.pop()
          }
        } else {
          templateStack[templateStack.length - 1]--
        }
      }
    }

    if (token === ts.SyntaxKind.Identifier || token === ts.SyntaxKind.PrivateIdentifier) {
      tokens.push('ID')
    } else if (
      ts.isLiteralKind(token) ||
      token === ts.SyntaxKind.TemplateHead ||
      token === ts.SyntaxKind.TemplateMiddle ||
      token === ts.SyntaxKind.TemplateTail
    ) {
      tokens.push('LIT')
    } else {
      tokens.push(scanner.getTokenText())
    }
    token = scanner.scan()
  }
  return tokens
}

export function extractTokenWindows(tokens, windowSize = WINDOW_SIZE) {
  return extractWindowsCommon(tokens, windowSize, ' ')
}

export function computeFileTokenWindows(root, files, windowSize = WINDOW_SIZE) {
  return files
    .filter((relPath) => existsSync(path.join(root, relPath)))
    .map((relPath) => {
      const code = readFileSync(path.join(root, relPath), 'utf8')
      const isJsx = relPath.endsWith('.tsx')
      const tokens = tokenizeAndNormalize(code, isJsx)
      return {
        path: relPath,
        windows: new Set(extractTokenWindows(tokens, windowSize)),
      }
    })
}

export function countStructuralDuplication(root, windowSize = WINDOW_SIZE) {
  const files = listSourceFiles(root)
  const fileData = files
    .filter((relPath) => existsSync(path.join(root, relPath)))
    .map((relPath, idx) => {
      const code = readFileSync(path.join(root, relPath), 'utf8')
      const isJsx = relPath.endsWith('.tsx')
      const tokens = tokenizeDetailed(code, isJsx)
      return { idx, path: relPath, code, tokens }
    })
  const allClones = findCoalescedClones(fileData, windowSize)
  const logicClones = allClones.filter((c) => c.category === 'Logic / Algorithmic')
  return {
    fileCount: files.length,
    totalDuplicatedBlocks: logicClones.length,
    clones: logicClones,
    topPairs: topClonePairs(logicClones),
  }
}

function baselineFile(root) {
  return path.join(root, 'agent_docs', 'structural_duplication_baseline.json')
}

export function readBaseline(root) {
  const file = baselineFile(root)
  if (!existsSync(file)) return { totalDuplicatedBlocks: 0 }
  return JSON.parse(readFileSync(file, 'utf8'))
}

export function writeBaseline(root, baseline) {
  writeFileSync(baselineFile(root), JSON.stringify(baseline, null, 2) + '\n')
}

export function check(root, windowSize = WINDOW_SIZE, { noTighten = false } = {}) {
  const baseline = readBaseline(root)
  const result = countStructuralDuplication(root, windowSize)
  const passed = result.totalDuplicatedBlocks <= baseline.totalDuplicatedBlocks
  let tightened = false
  const oldBaselineCount = baseline.totalDuplicatedBlocks

  if (passed && result.totalDuplicatedBlocks < baseline.totalDuplicatedBlocks) {
    if (!noTighten) {
      updateBaseline(root, { windowSize })
    }
    tightened = true
  }

  return {
    ...result,
    baselineCount: baseline.totalDuplicatedBlocks,
    oldBaselineCount,
    passed,
    tightened,
  }
}

export function updateBaseline(root, { allowRegression = false, windowSize = WINDOW_SIZE } = {}) {
  const baseline = readBaseline(root)
  const result = countStructuralDuplication(root, windowSize)

  if (!allowRegression && result.totalDuplicatedBlocks > baseline.totalDuplicatedBlocks) {
    throw new Error(
      `Refusing to raise the structural duplication baseline from ${baseline.totalDuplicatedBlocks} to ` +
        `${result.totalDuplicatedBlocks} without --allow-regression.`,
    )
  }

  const newBaseline = {
    totalDuplicatedBlocks: result.totalDuplicatedBlocks,
    windowSize,
    note: 'See scripts/work_dot_bat/lib/check-structural-duplication.mjs for exact token-normalization method. Lower is better; this file is a ratchet.',
  }
  writeBaseline(root, newBaseline)
  return newBaseline
}

export async function main(argv) {
  const root = repoRoot()
  const [cmd, ...rest] = argv

  try {
    switch (cmd) {
      case 'check': {
        const noTighten = rest.includes('--no-tighten')
        const result = check(root, WINDOW_SIZE, { noTighten })
        if (result.passed) {
          if (result.tightened) {
            process.stdout.write(
              noTighten
                ? `Structural duplication check passed (${result.totalDuplicatedBlocks} <= baseline ${result.baselineCount}).\n`
                : `Structural duplication check passed and baseline tightened (${result.oldBaselineCount} -> ${result.totalDuplicatedBlocks}).\n`,
            )
          } else {
            process.stdout.write(
              `Structural duplication check passed (${result.totalDuplicatedBlocks} <= baseline ${result.baselineCount}).\n`,
            )
          }
          return 0
        }

        process.stdout.write(
          `Structural duplication check failed: ${result.totalDuplicatedBlocks} duplicated ${WINDOW_SIZE}-token logic clone region(s) ` +
            `(baseline ${result.baselineCount}).\n`,
        )
        process.stdout.write('Top offending file pairs:\n')
        for (const { pair, count } of result.topPairs) {
          process.stdout.write(`  ${count}x  ${pair}\n`)
        }
        process.stdout.write(
          '\nEither remove the structural duplication or accept it deliberately: ' +
            '`scripts/work_dot_bat/check-structural-duplication.sh update [--allow-regression]`.\n',
        )
        return 1
      }
      case 'update': {
        const allowRegression = rest.includes('--allow-regression')
        const baseline = updateBaseline(root, { allowRegression })
        process.stdout.write(`Baseline updated: ${baseline.totalDuplicatedBlocks} duplicated block(s) recorded.\n`)
        return 0
      }
      case 'report': {
        return runReportMain(rest)
      }
      default:
        process.stderr.write(
          `Unknown command: "${cmd}"\nUsage: check-structural-duplication.sh <check|update|report> [--allow-regression|--category <logic|ui|data|all>|--limit <n>|--out <file>]\n`,
        )
        return 1
    }
  } catch (err) {
    process.stderr.write(`${err.message}\n`)
    return 1
  }
}

if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  process.exit(await main(process.argv.slice(2)))
}
