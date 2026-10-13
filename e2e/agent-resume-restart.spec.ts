import { createHash } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { delimiter, join } from 'node:path'
import { expect } from '@playwright/test'
import {
  standalone as test,
  VaultBackend,
  bindEndpoint,
  documentDir,
  openControlPlane,
  promptReady,
  type DisposableRoot,
} from './harness'
import { readStand } from './stand'

const adapterNames = ['claude', 'codex', 'omp', 'prime-agent'] as const
const tabs = '.nocx-tab'
const titles = '.nocx-tab-title'
const serverBin = () => readStand().server

test('restart visibly restores each agent and refuses a lazy ID', async ({ page }) => {
  test.setTimeout(180_000)
  const root = mkdtempSync(join(tmpdir(), 'nocx-agent-restart-'))
  const repo = join(root, 'shared-checkout')
  const fakeBin = join(root, 'bin')
  const invocations = join(root, 'invocations')
  mkdirSync(fakeBin, { recursive: true })
  mkdirSync(invocations, { recursive: true })
  mkdirSync(repo)
  execFileSync('git', ['init', '-q', '-b', 'main'], { cwd: repo })
  execFileSync(
    'git',
    [
      '-c',
      'user.name=Nocx E2E',
      '-c',
      'user.email=e2e@invalid',
      'commit',
      '--allow-empty',
      '-qm',
      'fixture',
    ],
    { cwd: repo },
  )

  const worktrees = adapterNames.map((name) => join(root, `worktree-${name}`))
  adapterNames.forEach((name, index) => {
    execFileSync(
      'git',
      ['worktree', 'add', '-q', '-b', `resume-${name}`, worktrees[index]!, 'HEAD'],
      { cwd: repo },
    )
  })

  const scripts: Record<(typeof adapterNames)[number], string> = {
    claude:
      '#!/bin/sh\nprintf "RESUMED claude cwd=%s args=%s\\n" "$PWD" "$*"\nprintf "%s|%s|%s\\n" "$PWD" "$0" "$*" >> "$NOCX_RESTART_INVOCATIONS/claude.log"\nexec cat >/dev/null\n',
    codex:
      '#!/bin/sh\nprintf "RESUMED codex cwd=%s args=%s\\n" "$PWD" "$*"\nprintf "%s|%s|%s\\n" "$PWD" "$0" "$*" >> "$NOCX_RESTART_INVOCATIONS/codex.log"\nexec cat >/dev/null\n',
    omp: '#!/bin/sh\nprintf "RESUMED omp cwd=%s args=%s\\n" "$PWD" "$*"\nprintf "%s|%s|%s\\n" "$PWD" "$0" "$*" >> "$NOCX_RESTART_INVOCATIONS/omp.log"\nexec cat >/dev/null\n',
    'prime-agent':
      '#!/bin/sh\nprintf "RESUMED prime-agent cwd=%s args=%s\\n" "$PWD" "$*"\nprintf "%s|%s|%s\\n" "$PWD" "$0" "$*" >> "$NOCX_RESTART_INVOCATIONS/prime-agent.log"\nexec cat >/dev/null\n',
  }
  for (const name of adapterNames) {
    const binary = join(fakeBin, name)
    writeFileSync(binary, scripts[name], { mode: 0o755 })
  }

  const oldPath = process.env.PATH
  const oldLogDir = process.env.NOCX_RESTART_INVOCATIONS
  process.env.PATH = `${fakeBin}${delimiter}${oldPath ?? ''}`
  process.env.NOCX_RESTART_INVOCATIONS = invocations
  const home: DisposableRoot = { root }
  const backend = new VaultBackend(serverBin(), home)
  const entryIds: Array<{
    name: string
    title: string
    cwd: string
    mode: string
    id: string
    worktree: object
    tabId: string
    paneId: string
  }> = []
  try {
    // Agent settings are read during backend startup, so install the fake
    // adapter executables before starting the first app process.
    const config = documentDir(join(root, 'home'))
    const agentDir = join(config, 'agents')
    mkdirSync(agentDir, { recursive: true })
    const adapterConfig = {
      claude: {
        version: 1,
        command: join(fakeBin, 'claude'),
        resume: { resumeCwdArgs: ['--continue'], resumeIdArgs: ['--resume', '{UUID}'] },
      },
      codex: {
        version: 1,
        command: join(fakeBin, 'codex'),
        resume: { resumeCwdArgs: ['resume', '--last'], resumeIdArgs: ['resume', '{UUID}'] },
      },
      omp: {
        version: 1,
        command: join(fakeBin, 'omp'),
        resume: { resumeCwdArgs: ['--continue'], resumeIdArgs: ['--resume={UUID}'] },
      },
      'prime-agent': {
        version: 1,
        command: join(fakeBin, 'prime-agent'),
        resume: { resumeCwdArgs: ['-c'], resumeIdArgs: ['-r', '{UUID}'] },
      },
    }
    for (const name of adapterNames)
      writeFileSync(join(agentDir, `${name}.json`), JSON.stringify(adapterConfig[name], null, 2))

    const ep1 = await backend.start()
    await bindEndpoint(page, ep1)
    await page.goto('/')
    await promptReady(page)
    const wire = await openControlPlane(ep1.port, ep1.token)
    try {
      const layout = (await wire.call('layout.read', {})) as { defaultWorkspaceId: string }
      const workspaceId = layout.defaultWorkspaceId
      const entries = [
        ...adapterNames.map((name, index) => ({
          name,
          title: `restart-${name}-cwd`,
          cwd: worktrees[index]!,
          mode: 'by-cwd',
          id: '',
          worktree: { Path: worktrees[index]!, Branch: `resume-${name}` },
        })),
        ...adapterNames.map((name, index) => ({
          name,
          title: `restart-${name}-id`,
          cwd: repo,
          mode: 'by-id',
          id: `${name}-stable-id-${index + 1}`,
          worktree: {},
        })),
        {
          name: 'claude',
          title: 'restart-claude-lazy-id',
          cwd: repo,
          mode: 'none',
          id: '',
          worktree: {},
        },
      ]
      for (const [index, entry] of entries.entries()) {
        const tabId = uuidv7()
        const paneId = uuidv7()
        await wire.call('tabs.create', {
          id: tabId,
          workspaceId,
          name: entry.title,
          position: index + 1,
          layout: 'row',
          firstPane: { id: paneId, cwd: entry.cwd, kind: 'local', sizeShare: 1 },
        })
        entryIds.push({ ...entry, tabId, paneId })
      }

      const approvals = adapterNames.map((name) => {
        const executable = join(fakeBin, name)
        return {
          agent: name,
          executable: {
            path: executable,
            sha256: createHash('sha256').update(readFileSync(executable)).digest('hex'),
          },
          domain: { kind: 'local' },
          scope: `tool-endpoint:${workspaceId}`,
          answer: 'granted',
        }
      })
      writeFileSync(
        join(config, 'agent-approvals.json'),
        JSON.stringify({ version: 2, approvals }, null, 2),
      )
      const records = entryIds.map((entry) => ({
        Participant: `worker-${entry.name}-${entry.title}`,
        Group: 'restart-acceptance',
        CoordinatorSession: 'previous-coordinator',
        PaneID: entry.paneId,
        TabID: entry.tabId,
        Agent: entry.name,
        Command: join(fakeBin, entry.name),
        Cwd: entry.cwd,
        Resume: entry.mode === 'none' ? { Mode: entry.mode } : { Mode: entry.mode, ID: entry.id },
        Worktree: entry.worktree,
      }))
      writeFileSync(
        join(config, 'worker-restarts.json'),
        JSON.stringify({ version: 1, records }, null, 2),
      )
    } finally {
      wire.close()
    }

    const ep2 = await backend.restart()
    await bindEndpoint(page, ep2)
    await page.reload()
    await promptReady(page)

    for (const entry of entryIds) {
      const tab = page.locator(tabs).filter({ has: page.locator(titles, { hasText: entry.title }) })
      await expect(tab, `${entry.title} restored as a visible tab`).toHaveCount(1, {
        timeout: 90_000,
      })
      await tab.click()
      if (entry.mode === 'none') {
        const output = page.locator('.pane.active .scrollback-inner')
        await expect(output).toContainText('nocx could not resume claude: resume-unavailable', {
          timeout: 30_000,
        })
        await expect(output).toContainText('no conversation', { timeout: 30_000 })
      } else {
        const invocationLog = join(invocations, `${entry.name}.log`)
        const expectedArgv =
          entry.mode === 'by-cwd'
            ? cwdArgs(entry.name).join(' ')
            : idArgs(entry.name, entry.id).join(' ')
        await expect
          .poll(() => (existsSync(invocationLog) ? readFileSync(invocationLog, 'utf8') : ''), {
            timeout: 15_000,
            message: `${entry.name} adapter invocation log`,
          })
          .toContain(`${entry.cwd}|${join(fakeBin, entry.name)}|${expectedArgv}`)
        await expect(tab, `${entry.title} selected in the visible tab strip`).toHaveAttribute(
          'aria-selected',
          'true',
        )
      }
    }

    for (const name of adapterNames) {
      const lines = readFileSync(join(invocations, `${name}.log`), 'utf8')
        .trim()
        .split('\n')
      expect(lines, `${name} CWD and ID restarts ran`).toHaveLength(2)
      const cwdEntry = entryIds.find((entry) => entry.name === name && entry.mode === 'by-cwd')!
      const idEntry = entryIds.find((entry) => entry.name === name && entry.mode === 'by-id')!
      expect(lines).toContain(`${cwdEntry.cwd}|${join(fakeBin, name)}|${cwdArgs(name).join(' ')}`)
      expect(lines).toContain(
        `${idEntry.cwd}|${join(fakeBin, name)}|${idArgs(name, idEntry.id).join(' ')}`,
      )
    }
  } finally {
    await backend.stop()
    if (oldPath === undefined) delete process.env.PATH
    else process.env.PATH = oldPath
    if (oldLogDir === undefined) delete process.env.NOCX_RESTART_INVOCATIONS
    else process.env.NOCX_RESTART_INVOCATIONS = oldLogDir
  }
})

function cwdArgs(name: string): string[] {
  return {
    claude: ['--continue'],
    codex: ['resume', '--last'],
    omp: ['--continue'],
    'prime-agent': ['-c'],
  }[name]!
}

function idArgs(name: string, id: string): string[] {
  return {
    claude: ['--resume', id],
    codex: ['resume', id],
    omp: [`--resume=${id}`],
    'prime-agent': ['-r', id],
  }[name]!
}

function uuidv7(): string {
  const bytes = new Uint8Array(16)
  globalThis.crypto.getRandomValues(bytes)
  bytes[6] = (bytes[6]! & 0x0f) | 0x70
  bytes[8] = (bytes[8]! & 0x3f) | 0x80
  const hex = [...bytes].map((b) => b.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}
