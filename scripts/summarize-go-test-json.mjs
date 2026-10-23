#!/usr/bin/env node
import readline from 'node:readline'

export function createGoTestSummary(limit = 20) {
  const byPackage = new Map()
  return {
    add(event) {
      if (
        !['pass', 'fail'].includes(event.Action) ||
        typeof event.Test !== 'string' ||
        !Number.isFinite(event.Elapsed)
      )
        return
      const tests = byPackage.get(event.Package) ?? []
      tests.push({ name: event.Test, seconds: event.Elapsed })
      byPackage.set(event.Package, tests)
    },
    result() {
      return [...byPackage.entries()]
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([pkg, tests]) => ({
          package: pkg,
          tests: tests
            .sort((a, b) => b.seconds - a.seconds || a.name.localeCompare(b.name))
            .slice(0, limit),
        }))
    },
  }
}

export function summarizeGoTestEvents(events, limit = 20) {
  const summary = createGoTestSummary(limit)
  for (const event of events) summary.add(event)
  return summary.result()
}

async function main() {
  const summary = createGoTestSummary()
  const lines = readline.createInterface({ input: process.stdin, crlfDelay: Infinity })
  for await (const line of lines) {
    try {
      const event = JSON.parse(line)
      summary.add(event)
      if (event.Action === 'output' && typeof event.Output === 'string') {
        process.stdout.write(event.Output)
      }
    } catch {
      // Keep malformed or non-JSON diagnostics visible; they cannot contribute a test.
      process.stdout.write(`${line}\n`)
    }
  }
  for (const { package: pkg, tests } of summary.result()) {
    console.error(`\nSlowest tests in ${pkg} (seconds):`)
    for (const { name, seconds } of tests) console.error(`  ${seconds.toFixed(3)}  ${name}`)
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main().catch((error) => {
    console.error(error)
    process.exitCode = 1
  })
}
