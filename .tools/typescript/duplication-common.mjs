// work_dot_bat managed file — DO NOT EDIT; overwritten by `work_dot_bat sync`.
// file: scripts/work_dot_bat/typescript/duplication-common.mjs
// version: 1.0.0
// date: 2026-10-03
// sha256: a84ca3b6fd78a2cb90f1dd82caa2796b72cbc60da94f39894a2a62bb2d865456
// Shared pure helpers for duplication detection gates (check-duplication and check-structural-duplication).

import { execFileSync } from 'node:child_process'

export const TOP_PAIRS_LIMIT = 10

const EXCLUDE_TEST = /\.test\./
const EXCLUDE_GENERATED = /generated/

export function isTrackedSourceFile(relPath) {
  if (!/\.tsx?$/.test(relPath)) return false
  if (EXCLUDE_TEST.test(relPath)) return false
  if (EXCLUDE_GENERATED.test(relPath)) return false
  return true
}

export function listSourceFiles(root) {
  const output = execFileSync('git', ['ls-tree', '-r', '--name-only', 'HEAD', '--', 'src'], {
    cwd: root,
    encoding: 'utf8',
  })
  return output
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .filter(isTrackedSourceFile)
    .sort()
}

export function extractWindows(elements, windowSize, delimiter = '\n') {
  const windows = []
  for (let i = 0; i + windowSize <= elements.length; i++) {
    windows.push(elements.slice(i, i + windowSize).join(delimiter))
  }
  return windows
}

// Pure comparison, kept separate from disk/git I/O so it's directly testable.
// `fileWindows` is [{ path, windows: Set<string> }, ...].
export function findDuplicates(fileWindows) {
  const windowToFiles = new Map()
  for (const { path: relPath, windows } of fileWindows) {
    for (const window of windows) {
      if (!windowToFiles.has(window)) windowToFiles.set(window, new Set())
      windowToFiles.get(window).add(relPath)
    }
  }

  const duplicated = []
  for (const [window, files] of windowToFiles) {
    if (files.size >= 2) duplicated.push({ window, files: [...files].sort() })
  }
  return duplicated
}

export function topFilePairs(duplicated, limit = TOP_PAIRS_LIMIT) {
  const pairCounts = new Map()
  for (const { files } of duplicated) {
    for (let i = 0; i < files.length; i++) {
      for (let j = i + 1; j < files.length; j++) {
        const key = `${files[i]} <-> ${files[j]}`
        pairCounts.set(key, (pairCounts.get(key) ?? 0) + 1)
      }
    }
  }
  return [...pairCounts.entries()]
    .map(([pair, count]) => ({ pair, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, limit)
}

export function repoRoot() {
  return execFileSync('git', ['rev-parse', '--show-toplevel'], { encoding: 'utf8' }).trim()
}
