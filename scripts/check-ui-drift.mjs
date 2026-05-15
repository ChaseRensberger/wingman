#!/usr/bin/env bun

import { existsSync, readdirSync, readFileSync } from "node:fs"
import { relative, resolve } from "node:path"
import { spawnSync } from "node:child_process"

const root = resolve(import.meta.dirname, "..")

const args = process.argv.slice(2)
const showDiff = args.includes("--diff")
const appFilter = readOption("--app")

const apps = [
  {
    name: "web",
    exactDirs: [{ source: "ui/src/components/core", target: "web/src/components/core" }],
    exactFiles: [
      ["ui/src/components/theme-provider.tsx", "web/src/components/theme-provider.tsx"],
      ["ui/src/components/theme-toggle.tsx", "web/src/components/theme-toggle.tsx"],
    ],
    tokenFiles: [["ui/src/globals.css", "web/src/globals.css"]],
  },
  {
    name: "hero",
    exactFiles: [
      ["ui/src/components/core/accordion.tsx", "hero/src/components/core/accordion.tsx"],
      ["ui/src/components/core/button.tsx", "hero/src/components/core/button.tsx"],
      ["ui/src/components/theme-provider.tsx", "hero/src/components/theme-provider.tsx"],
    ],
    tokenFiles: [["ui/src/globals.css", "hero/src/styles/globals.css"]],
  },
  {
    name: "docs",
    tokenFiles: [["ui/src/globals.css", "docs/src/styles/custom.css"]],
  },
]

const selectedApps = appFilter ? apps.filter((app) => app.name === appFilter) : apps

if (appFilter && selectedApps.length === 0) {
  console.error(`Unknown app: ${appFilter}`)
  console.error(`Known apps: ${apps.map((app) => app.name).join(", ")}`)
  process.exit(2)
}

const results = []

for (const app of selectedApps) {
  for (const dir of app.exactDirs ?? []) {
    compareExactDir(app.name, dir.source, dir.target)
  }

  for (const [source, target] of app.exactFiles ?? []) {
    compareExactFile(app.name, source, target)
  }

  for (const [source, target] of app.tokenFiles ?? []) {
    compareTokens(app.name, source, target)
  }
}

if (results.length === 0) {
  console.log("No UI drift found.")
  process.exit(0)
}

console.log("UI drift found:\n")

for (const result of results) {
  console.log(`${result.app}: ${result.kind} ${result.target}`)
  console.log(`  source: ${result.source}`)
  if (result.detail) {
    console.log(`  ${result.detail}`)
  }

  if (showDiff) {
    printDiff(result)
  }
}

process.exit(1)

function readOption(name) {
  const index = args.indexOf(name)
  if (index === -1) return undefined
  return args[index + 1]
}

function compareExactDir(app, sourceDir, targetDir) {
  const sourcePath = resolve(root, sourceDir)
  const targetPath = resolve(root, targetDir)

  if (!existsSync(sourcePath)) {
    results.push({ app, kind: "missing-source", source: sourceDir, target: targetDir })
    return
  }

  for (const entry of readdirSync(sourcePath, { withFileTypes: true })) {
    if (!entry.isFile()) continue
    compareExactFile(app, `${sourceDir}/${entry.name}`, `${targetDir}/${entry.name}`)
  }
}

function compareExactFile(app, source, target) {
  const sourcePath = resolve(root, source)
  const targetPath = resolve(root, target)

  if (!existsSync(sourcePath)) {
    results.push({ app, kind: "missing-source", source, target })
    return
  }

  if (!existsSync(targetPath)) {
    results.push({ app, kind: "missing", source, target })
    return
  }

  if (readFileSync(sourcePath, "utf8") !== readFileSync(targetPath, "utf8")) {
    results.push({ app, kind: "modified", source, target })
  }
}

function compareTokens(app, source, target) {
  const sourcePath = resolve(root, source)
  const targetPath = resolve(root, target)

  if (!existsSync(sourcePath)) {
    results.push({ app, kind: "missing-source", source, target })
    return
  }

  if (!existsSync(targetPath)) {
    results.push({ app, kind: "missing", source, target })
    return
  }

  const sourceTokens = extractCssTokens(readFileSync(sourcePath, "utf8"), source)
  const targetTokens = extractCssTokens(readFileSync(targetPath, "utf8"), target)
  const changes = []

  for (const [token, sourceValue] of sourceTokens) {
    if (!targetTokens.has(token)) {
      changes.push(`${token} missing`)
      continue
    }

    const targetValue = targetTokens.get(token)
    if (sourceValue !== targetValue) {
      changes.push(`${token} differs`)
    }
  }

  if (changes.length > 0) {
    const detail = `${changes.length} shared token change${changes.length === 1 ? "" : "s"}`
    results.push({ app, kind: "tokens-modified", source, target, detail })
  }
}

function extractCssTokens(css, file) {
  const tokens = new Map()
  const blocks = css.matchAll(/([^{}]+)\{([^{}]*)\}/g)
  const isDocs = file.includes("docs/src/styles/custom.css")

  for (const block of blocks) {
    const selector = block[1].trim()
    const body = block[2]
    const modes = tokenModes(selector, isDocs)

    if (modes.length === 0) continue

    const declarations = body.matchAll(/--[a-zA-Z0-9-]+\s*:\s*[^;]+;/g)

    for (const declaration of declarations) {
      const [name, value] = declaration[0].slice(0, -1).split(/:\s*/, 2)
      const token = name.trim()

      if (!isSharedToken(token)) continue

      for (const mode of modes) {
        tokens.set(`${mode}:${token}`, value.trim().replace(/\s+/g, " "))
      }
    }
  }

  return tokens
}

function tokenModes(selector, isDocs) {
  if (isDocs) {
    if (selector === ":root") return ["light", "dark"]
    if (selector === ":root[data-theme=\"light\"]") return ["light"]
    return []
  }

  if (selector === ":root") return ["light"]
  if (selector === ".dark") return ["dark"]
  return []
}

function isSharedToken(name) {
  return (
    name === "--radius" ||
    name === "--font-mono" ||
    [
      "--background",
      "--foreground",
      "--card",
      "--card-foreground",
      "--popover",
      "--popover-foreground",
      "--primary",
      "--primary-foreground",
      "--secondary",
      "--secondary-foreground",
      "--muted",
      "--muted-foreground",
      "--accent",
      "--accent-foreground",
      "--destructive",
      "--border",
      "--input",
      "--ring",
      "--overlay",
      "--subtle-foreground",
    ].includes(name)
  )
}

function printDiff(result) {
  const source = resolve(root, result.source)
  const target = resolve(root, result.target)

  if (!existsSync(source) || !existsSync(target)) return

  const diff = spawnSync("git", ["diff", "--no-index", "--", source, target], {
    cwd: root,
    encoding: "utf8",
  })

  const output = `${diff.stdout}${diff.stderr}`.trimEnd()
  if (!output) return

  console.log(indentDiff(output))
}

function indentDiff(output) {
  return output
    .split("\n")
    .map((line) => `  ${line.replaceAll(root + "/", "")}`)
    .join("\n")
}
