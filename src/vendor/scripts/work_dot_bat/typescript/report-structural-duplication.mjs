// Generates an actionable structural duplication report detailing Type-2 clones across src/
// Focuses detailed analysis strictly on "Logic / Algorithmic" duplication while reporting
// overall counts for all categories.
// See agent_docs/plans/september_2026/improve_duplication_detection.md.

import { readFileSync, writeFileSync, mkdirSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import ts from 'typescript'
import {
  listSourceFiles,
  repoRoot,
} from './duplication-common.mjs'

export const DEFAULT_WINDOW_SIZE = 50
export const DEFAULT_REPORT_PATH = 'agent_docs/reports/structural_duplication_report.md'

const CONTROL_KEYWORDS = new Set([
  'if',
  'else',
  'for',
  'while',
  'do',
  'switch',
  'case',
  'default',
  'try',
  'catch',
  'throw',
])

export function getLineNumber(code, offset) {
  let line = 1
  for (let i = 0; i < offset && i < code.length; i++) {
    if (code.charCodeAt(i) === 10) line++
  }
  return line
}

export function tokenizeDetailed(code, isJsx = false) {
  const variant = isJsx ? ts.LanguageVariant.JSX : ts.LanguageVariant.Standard
  const scanner = ts.createScanner(ts.ScriptTarget.Latest, true, variant, code)
  let token = scanner.scan()
  const tokens = []
  const templateStack = []

  while (token !== ts.SyntaxKind.EndOfFileToken) {
    const start = scanner.getTokenPos()
    const end = scanner.getTextPos()
    const text = scanner.getTokenText()

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

    let norm = text
    let isControl = false
    let isJsxToken = false

    if (token === ts.SyntaxKind.Identifier || token === ts.SyntaxKind.PrivateIdentifier) {
      norm = 'ID'
    } else if (
      ts.isLiteralKind(token) ||
      token === ts.SyntaxKind.TemplateHead ||
      token === ts.SyntaxKind.TemplateMiddle ||
      token === ts.SyntaxKind.TemplateTail
    ) {
      norm = 'LIT'
    } else {
      if (CONTROL_KEYWORDS.has(text)) {
        isControl = true
      }
      if (
        token === ts.SyntaxKind.JsxElement ||
        token === ts.SyntaxKind.JsxSelfClosingElement ||
        token === ts.SyntaxKind.JsxOpeningElement ||
        token === ts.SyntaxKind.JsxClosingElement ||
        token === ts.SyntaxKind.JsxText ||
        token === ts.SyntaxKind.LessThanSlashToken ||
        token === ts.SyntaxKind.SlashGreaterThanToken
      ) {
        isJsxToken = true
      }
    }

    tokens.push({ norm, start, end, isControl, isJsxToken, text })
    token = scanner.scan()
  }
  return tokens
}

export function classifyClone(matchedTokens, isJsxFile) {
  const hasControl = matchedTokens.some((t) => t.isControl)
  const hasJsx = isJsxFile && matchedTokens.some((t) => t.isJsxToken || t.text === '<' || t.text === '>')

  if (hasControl) return 'Logic / Algorithmic'
  if (hasJsx) return 'UI / JSX Component Shell'
  return 'Declarative Data / Configuration'
}

export function findCoalescedClones(fileData, windowSize = DEFAULT_WINDOW_SIZE) {
  const windowMap = new Map()

  for (const file of fileData) {
    const t = file.tokens
    for (let i = 0; i + windowSize <= t.length; i++) {
      const w = t.slice(i, i + windowSize).map((x) => x.norm).join(' ')
      let list = windowMap.get(w)
      if (!list) {
        list = []
        windowMap.set(w, list)
      }
      list.push({ fileIdx: file.idx, tokenIdx: i })
    }
  }

  const pairMatches = new Map()
  for (const [, occurrences] of windowMap) {
    if (occurrences.length < 2) continue
    for (let i = 0; i < occurrences.length; i++) {
      for (let j = i + 1; j < occurrences.length; j++) {
        const o1 = occurrences[i]
        const o2 = occurrences[j]
        if (o1.fileIdx === o2.fileIdx) continue
        const [fA, fB, tA, tB] =
          o1.fileIdx < o2.fileIdx
            ? [o1.fileIdx, o2.fileIdx, o1.tokenIdx, o2.tokenIdx]
            : [o2.fileIdx, o1.fileIdx, o2.tokenIdx, o1.tokenIdx]

        const pairKey = `${fA}:${fB}`
        let diagMap = pairMatches.get(pairKey)
        if (!diagMap) {
          diagMap = new Map()
          pairMatches.set(pairKey, diagMap)
        }
        const diag = tB - tA
        let starts = diagMap.get(diag)
        if (!starts) {
          starts = []
          diagMap.set(diag, starts)
        }
        starts.push(tA)
      }
    }
  }

  const clones = []
  for (const [pairKey, diagMap] of pairMatches) {
    const [fA, fB] = pairKey.split(':').map(Number)
    const fileA = fileData[fA]
    const fileB = fileData[fB]

    for (const [diag, starts] of diagMap) {
      starts.sort((a, b) => a - b)
      let curStart = starts[0]
      let curEnd = starts[0]

      for (let i = 1; i < starts.length; i++) {
        if (starts[i] <= curEnd + 1) {
          curEnd = starts[i]
        } else {
          clones.push(buildCloneRecord(fileA, fileB, curStart, curEnd, diag, windowSize))
          curStart = starts[i]
          curEnd = starts[i]
        }
      }
      clones.push(buildCloneRecord(fileA, fileB, curStart, curEnd, diag, windowSize))
    }
  }

  return clones.sort((a, b) => b.tokenLength - a.tokenLength)
}

function buildCloneRecord(fileA, fileB, curStart, curEnd, diag, windowSize) {
  const tokStartA = curStart
  const tokEndA = curEnd + windowSize - 1
  const tokStartB = curStart + diag
  const tokEndB = curEnd + diag + windowSize - 1
  const tokenLength = (curEnd - curStart) + windowSize

  const charStartA = fileA.tokens[tokStartA].start
  const charEndA = fileA.tokens[tokEndA].end
  const charStartB = fileB.tokens[tokStartB].start
  const charEndB = fileB.tokens[tokEndB].end

  const lineStartA = getLineNumber(fileA.code, charStartA)
  const lineEndA = getLineNumber(fileA.code, charEndA)
  const lineStartB = getLineNumber(fileB.code, charStartB)
  const lineEndB = getLineNumber(fileB.code, charEndB)

  const matchedTokensA = fileA.tokens.slice(tokStartA, tokEndA + 1)
  const category = classifyClone(matchedTokensA, fileA.path.endsWith('.tsx') || fileB.path.endsWith('.tsx'))
  const controlKeywords = [...new Set(matchedTokensA.filter((t) => t.isControl).map((t) => t.text))]

  return {
    fileA: fileA.path,
    fileB: fileB.path,
    tokenLength,
    rawWindowsCount: curEnd - curStart + 1,
    lineStartA,
    lineEndA,
    lineCountA: lineEndA - lineStartA + 1,
    lineStartB,
    lineEndB,
    lineCountB: lineEndB - lineStartB + 1,
    category,
    controlKeywords,
    snippetA: fileA.code.slice(charStartA, charEndA).trim(),
    snippetB: fileB.code.slice(charStartB, charEndB).trim(),
  }
}

export function generateReportMarkdown(allClones, { fileCount, totalRawWindows, limit = 50, root = '' } = {}) {
  const categoryCounts = {
    'Logic / Algorithmic': 0,
    'UI / JSX Component Shell': 0,
    'Declarative Data / Configuration': 0,
  }

  for (const c of allClones) {
    categoryCounts[c.category] = (categoryCounts[c.category] ?? 0) + 1
  }

  // Focus detailed analysis strictly on Logic / Algorithmic clones
  const logicClones = allClones.filter((c) => c.category === 'Logic / Algorithmic')

  const pairCounts = new Map()
  for (const c of logicClones) {
    const pair = `${c.fileA} <-> ${c.fileB}`
    pairCounts.set(pair, (pairCounts.get(pair) ?? 0) + 1)
  }
  const topLogicPairs = [...pairCounts.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 15)

  // Sub-pattern classification among logic clones
  const logicPatterns = {
    'Branching (if / else)': 0,
    'Error Handling (try / catch / throw)': 0,
    'Loops (for / while / do)': 0,
    'Switch / Case Dispatch': 0,
  }
  for (const c of logicClones) {
    const kw = new Set(c.controlKeywords ?? [])
    if (kw.has('if') || kw.has('else')) logicPatterns['Branching (if / else)']++
    if (kw.has('try') || kw.has('catch') || kw.has('throw')) logicPatterns['Error Handling (try / catch / throw)']++
    if (kw.has('for') || kw.has('while') || kw.has('do')) logicPatterns['Loops (for / while / do)']++
    if (kw.has('switch') || kw.has('case') || kw.has('default')) logicPatterns['Switch / Case Dispatch']++
  }

  const lines = []
  lines.push('# Structural Duplication Report: Logic & Algorithmic Analysis')
  lines.push('')
  lines.push(`Generated on **${new Date().toISOString()}** across **${fileCount}** tracked source files.`)
  lines.push('')
  lines.push('> **Report Scope**: Duplication counts across all categories are tallied in the Executive Summary, while in-depth clone inspection, pattern breakdowns, and pair rankings focus exclusively on **Logic / Algorithmic** duplication.')
  lines.push('')
  lines.push('## Executive Summary (All Categories)')
  lines.push('')
  lines.push('| Metric | Count | Percentage | Description |')
  lines.push('| :--- | :--- | :--- | :--- |')
  lines.push(`| **Raw 50-Token Sliding Windows** | **${totalRawWindows}** | — | Total pairwise overlapping micro-windows across codebase. |`)
  lines.push(`| **Total Coalesced Clone Regions** | **${allClones.length}** | 100% | All contiguous duplicate code regions merged across file pairs. |`)
  lines.push(`| ├─ **Logic / Algorithmic** | **${categoryCounts['Logic / Algorithmic']}** | ${(allClones.length ? ((categoryCounts['Logic / Algorithmic'] / allClones.length) * 100).toFixed(1) : 0)}% | **Analyzed in this report.** Clones containing control flow (\`if\`, \`for\`, \`switch\`, \`throw\`, etc.). |`)
  lines.push(`| ├─ **UI / JSX Component Shell** | **${categoryCounts['UI / JSX Component Shell']}** | ${(allClones.length ? ((categoryCounts['UI / JSX Component Shell'] / allClones.length) * 100).toFixed(1) : 0)}% | Clones sharing JSX markup shells, wrappers, or component trees. |`)
  lines.push(`| └─ **Declarative Data / Config** | **${categoryCounts['Declarative Data / Configuration']}** | ${(allClones.length ? ((categoryCounts['Declarative Data / Configuration'] / allClones.length) * 100).toFixed(1) : 0)}% | Static numeric tables, instrument presets, and route lists. |`)
  lines.push('')
  lines.push('---')
  lines.push('')
  lines.push('## Logic Duplication Breakdown')
  lines.push('')
  lines.push('| Control Structure Pattern | Matching Clone Regions | Key Characteristics |')
  lines.push('| :--- | :--- | :--- |')
  lines.push(`| **Branching (\`if\` / \`else\`)** | **${logicPatterns['Branching (if / else)']}** | Conditional checks, validation logic, and branch selection. |`)
  lines.push(`| **Error Handling (\`throw\` / \`try\` / \`catch\`)** | **${logicPatterns['Error Handling (try / catch / throw)']}** | Guard clauses throwing descriptive errors on unexpected input. |`)
  lines.push(`| **Loops (\`for\` / \`while\` / \`do\`)** | **${logicPatterns['Loops (for / while / do)']}** | Iterative traversal, array mutations, and time-series scanning. |`)
  lines.push(`| **Switch / Case Dispatch** | **${logicPatterns['Switch / Case Dispatch']}** | Pattern matching across node/wave types or event codes. |`)
  lines.push('')
  lines.push('---')
  lines.push('')
  lines.push('## Top File Pairs by Logic / Algorithmic Duplication')
  lines.push('')
  lines.push('| Rank | Logic Clones | File Pair |')
  lines.push('| :--- | :--- | :--- |')
  topLogicPairs.forEach(([pair, count], idx) => {
    lines.push(`| ${idx + 1} | ${count} | \`${pair}\` |`)
  })
  lines.push('')
  lines.push('---')
  lines.push('')
  lines.push(`## Detailed Logic Clone Inspection (Top ${Math.min(limit, logicClones.length)} of ${logicClones.length})`)
  lines.push('')

  const displayed = logicClones.slice(0, limit)
  displayed.forEach((c, idx) => {
    const urlA = root ? `file:///${path.join(root, c.fileA).replace(/\\/g, '/')}` : c.fileA
    const urlB = root ? `file:///${path.join(root, c.fileB).replace(/\\/g, '/')}` : c.fileB
    const kwText = c.controlKeywords && c.controlKeywords.length > 0 ? c.controlKeywords.join(', ') : 'none'

    lines.push(`### #${idx + 1}: ${c.fileA} <-> ${c.fileB}`)
    lines.push('')
    lines.push(`- **Size**: **${c.tokenLength} tokens** (${c.rawWindowsCount} overlapping sliding windows)`)
    lines.push(`- **Control Keywords**: \`${kwText}\``)
    lines.push(`- **Location A**: [${c.fileA}#L${c.lineStartA}-L${c.lineEndA}](${urlA}#L${c.lineStartA}-L${c.lineEndA}) (${c.lineCountA} lines)`)
    lines.push(`- **Location B**: [${c.fileB}#L${c.lineStartB}-L${c.lineEndB}](${urlB}#L${c.lineStartB}-L${c.lineEndB}) (${c.lineCountB} lines)`)
    lines.push('')
    lines.push('<details><summary><b>View Code Snippets</b></summary>')
    lines.push('')
    lines.push(`#### \`${c.fileA}\` (Lines ${c.lineStartA}–${c.lineEndA}):`)
    lines.push('```typescript')
    lines.push(c.snippetA)
    lines.push('```')
    lines.push('')
    lines.push(`#### \`${c.fileB}\` (Lines ${c.lineStartB}–${c.lineEndB}):`)
    lines.push('```typescript')
    lines.push(c.snippetB)
    lines.push('```')
    lines.push('</details>')
    lines.push('')
    lines.push('---')
    lines.push('')
  })

  return lines.join('\n')
}

export function runReport(root, options = {}) {
  const windowSize = options.windowSize ?? DEFAULT_WINDOW_SIZE
  const limit = options.limit ?? 50
  const outPath = options.out ? path.resolve(root, options.out) : path.join(root, DEFAULT_REPORT_PATH)

  const files = listSourceFiles(root)
  const fileData = files.map((f, idx) => {
    const code = readFileSync(path.join(root, f), 'utf8')
    const tokens = tokenizeDetailed(code, f.endsWith('.tsx'))
    return { idx, path: f, code, tokens }
  })

  const allClones = findCoalescedClones(fileData, windowSize)
  const totalRawWindows = allClones.reduce((acc, c) => acc + c.rawWindowsCount, 0)
  const logicClones = allClones.filter((c) => c.category === 'Logic / Algorithmic')

  const markdown = generateReportMarkdown(allClones, {
    fileCount: files.length,
    totalRawWindows,
    limit,
    root,
  })

  mkdirSync(path.dirname(outPath), { recursive: true })
  writeFileSync(outPath, markdown, 'utf8')

  return {
    fileCount: files.length,
    totalRawWindows,
    allClonesCount: allClones.length,
    logicClonesCount: logicClones.length,
    outPath,
    markdown,
    logicClones,
    allClones,
  }
}

export function main(argv) {
  const root = repoRoot()
  let out = DEFAULT_REPORT_PATH
  let limit = 50
  let stdout = false

  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === '--out' && argv[i + 1]) {
      out = argv[++i]
    } else if (argv[i] === '--limit' && argv[i + 1]) {
      limit = parseInt(argv[++i], 10)
    } else if (argv[i] === '--stdout') {
      stdout = true
    }
  }

  const result = runReport(root, { out, limit })

  const categoryCounts = {
    'Logic / Algorithmic': 0,
    'UI / JSX Component Shell': 0,
    'Declarative Data / Configuration': 0,
  }
  for (const c of result.allClones) {
    categoryCounts[c.category] = (categoryCounts[c.category] ?? 0) + 1
  }

  process.stdout.write(`\n--- Structural Duplication: Logic & Algorithmic Analysis ---\n`)
  process.stdout.write(`Tracked Files Scanned: ${result.fileCount}\n`)
  process.stdout.write(`Total Raw 50-Token Sliding Windows: ${result.totalRawWindows}\n`)
  process.stdout.write(`Total Coalesced Clone Regions (All Categories): ${result.allClonesCount}\n`)
  process.stdout.write(`  ├─ Logic / Algorithmic: ${result.logicClonesCount} (Analyzed in detail)\n`)
  process.stdout.write(`  ├─ UI / JSX Component Shell: ${categoryCounts['UI / JSX Component Shell']}\n`)
  process.stdout.write(`  └─ Declarative Data / Config: ${categoryCounts['Declarative Data / Configuration']}\n`)
  process.stdout.write(`Detailed Report written to: ${result.outPath}\n\n`)

  if (stdout) {
    process.stdout.write(result.markdown + '\n')
  }

  return 0
}

if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  process.exit(main(process.argv.slice(2)))
}
