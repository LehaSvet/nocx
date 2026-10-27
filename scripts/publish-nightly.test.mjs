import { test } from 'node:test'
import { execFileSync, spawnSync } from 'node:child_process'
import assert from 'node:assert/strict'
import { mkdtempSync, writeFileSync, readFileSync, rmSync, cpSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { generateKeyPairSync, sign, createHash } from 'node:crypto'
import { decide, publish, prepare, attest, authorize } from './publish-nightly.mjs'

function fixture(t) {
  const dir = mkdtempSync(join(tmpdir(), 'nightly-'))
  t.after(() => rmSync(dir, { recursive: true, force: true }))
  const { publicKey, privateKey } = generateKeyPairSync('ed25519')
  const files = ['nocx-0.8.0-darwin-universal.zip', 'nocx-0.8.0-linux-amd64.AppImage']
  const artifacts = files.map((name, i) => {
    const data = Buffer.from(`built artifact ${i}`)
    writeFileSync(join(dir, name), data)
    return {
      os: i ? 'linux' : 'darwin',
      arch: i ? 'amd64' : 'universal',
      format: i ? 'AppImage' : 'zip',
      url: `https://github.com/shady2k/nocx/releases/download/nightly-20261027-42/${name}`,
      sha256: createHash('sha256').update(data).digest('hex'),
      size: data.length,
    }
  })
  const manifest = Buffer.from(JSON.stringify({ version: '0.8.0', artifacts, helpers: [] }))
  writeFileSync(join(dir, 'manifest.json'), manifest)
  writeFileSync(join(dir, 'manifest.json.sig'), sign(null, manifest, privateKey).toString('base64'))
  const input = {
    moved: true,
    sha: 'a'.repeat(40),
    version: '0.8.0',
    tag: 'nightly-20261027-42',
    runId: '42',
    runAttempt: '1',
    dir,
    keyring: [publicKey.export({ format: 'der', type: 'spki' }).subarray(-32).toString('base64')],
  }
  for (const [i, platform] of ['darwin', 'linux'].entries()) {
    const about = { Version: input.version, Commit: input.sha, Wails: 'v3.0.0-beta.9' }
    writeFileSync(
      join(dir, `verified-${platform}.json`),
      JSON.stringify({
        version: input.version,
        commit: input.sha,
        archive: { name: files[i], sha256: artifacts[i].sha256 },
        identity: { CFBundleIdentifier: 'dev.nocx' },
        declaredIdentity: { CFBundleIdentifier: 'dev.nocx' },
        executables: { nocx: { about }, 'nocx-server': { about } },
        updater: {
          signature: 'fixture-signature',
          sha256: 'fixture-hash',
          manifest: { version: '0.7.2' },
        },
      }),
    )
  }
  for (const name of [
    'nocx-0.8.0-darwin-universal.dmg',
    ...['linux-amd64', 'linux-arm64', 'darwin-amd64', 'darwin-arm64'].map(
      (p) => `nocx-helper-0.8.0-${p}`,
    ),
  ])
    writeFileSync(join(dir, name), 'fixture payload')
  const provenance = Buffer.from(JSON.stringify(attest(input)))
  writeFileSync(join(dir, 'nightly.json'), provenance)
  writeFileSync(
    join(dir, 'nightly.json.sig'),
    sign(null, provenance, privateKey).toString('base64'),
  )
  return { dir, input, privateKey }
}

test('unchanged main publishes nothing, without artifacts or signing keys', async () => {
  const calls = []
  assert.deepEqual(await publish({ moved: false }, (...args) => calls.push(args)), {
    published: false,
  })
  assert.deepEqual(calls, [])
})
test('moved main publishes signed dated and moving prereleases, never a cask', async (t) => {
  const { input } = fixture(t)
  const calls = []
  await publish(input, (args) => {
    calls.push(args)
    return '[]'
  })
  assert.ok(calls.some((c) => c.includes(input.tag) && c.includes('--prerelease')))
  assert.ok(calls.some((c) => c.includes('nightly') && c.includes('--prerelease')))
  assert.ok(!JSON.stringify(calls).includes('homebrew'))
})
test('missing or corrupt signature refuses before any publication', async (t) => {
  const { input, dir } = fixture(t)
  const calls = []
  writeFileSync(join(dir, 'manifest.json.sig'), 'broken')
  await assert.rejects(
    publish(input, (args) => calls.push(args)),
    /signature/,
  )
  assert.deepEqual(calls, [])
})
test('changed artifact refuses naming its filename before publication', (t) => {
  const { input, dir } = fixture(t)
  writeFileSync(join(dir, 'nocx-0.8.0-darwin-universal.zip'), 'corrupt')
  assert.throws(() => decide(input), /nocx-0.8.0-darwin-universal.zip/)
})
test('wrong stamped version refuses', (t) => {
  const { input } = fixture(t)
  assert.throws(() => decide({ ...input, version: '0.9.0' }), /version/)
})
test('prepare distinguishes moved, unchanged, and supporting-only commits', () => {
  const base = {
    sha: 'a'.repeat(40),
    previous: 'b'.repeat(40),
    version: '0.8.0',
    date: '20261027',
    runId: '42',
  }
  assert.equal(prepare({ ...base, files: ['main.go'] }).moved, true)
  assert.equal(prepare({ ...base, previous: base.sha, files: [] }).moved, false)
  assert.equal(prepare({ ...base, files: ['docs/a.md', '.beads/issues.jsonl'] }).moved, false)
  assert.equal(prepare({ ...base, previous: null, files: [] }).moved, true)
  assert.throws(() => prepare({ ...base, files: [] }), /empty diff/)
})
test('publication failure does not move nightly tag', async (t) => {
  const { input } = fixture(t)
  const calls = []
  await assert.rejects(
    publish(input, (args) => {
      calls.push(args)
      throw Error('upload failed')
    }),
    /upload failed/,
  )
  assert.equal(calls.length, 1)
})

// Static wiring checks supplement, never replace, the executed decisions above.
test('nightly workflow calls the shared build without CI or Homebrew and gates unchanged main', () => {
  const workflow = readFileSync(
    new URL('../.github/workflows/nightly.yml', import.meta.url),
    'utf8',
  )
  assert.match(workflow, /cron:/)
  assert.match(workflow, /if: needs.prepare.outputs.moved == 'true'/)
  assert.match(workflow, /uses: \.\/\.github\/workflows\/release-build.yml/)
  assert.doesNotMatch(workflow, /uses:.*ci.yml|HOMEBREW_TAP_TOKEN|Casks\//i)
  const build = readFileSync(
    new URL('../.github/workflows/release-build.yml', import.meta.url),
    'utf8',
  )
  assert.doesNotMatch(build, /uses:.*ci.yml/)
  assert.match(build, /scripts\/appimage\/verify-appimage.sh/)
  assert.match(build, /go run \.\/cmd\/manifest-sign -verify/)
})

test('deliberately broken About.Wails refuses naming field before any gh call', async (t) => {
  const { input, dir } = fixture(t)
  const calls = []
  const file = join(dir, 'verified-darwin.json')
  const report = JSON.parse(readFileSync(file, 'utf8'))
  report.executables.nocx.about.Wails = 'unknown'
  writeFileSync(file, JSON.stringify(report))
  await assert.rejects(
    publish(input, (args) => calls.push(args)),
    /About.Wails/,
  )
  assert.deepEqual(calls, [])
})
test('retention keeps seven dated copies and never removes stable releases', async (t) => {
  const { input } = fixture(t)
  const calls = []
  const releases = Array.from({ length: 9 }, (_, i) => ({
    tag_name: `nightly-202610${String(26 - i).padStart(2, '0')}-${i}`,
    prerelease: true,
    published_at: `2026-10-${String(26 - i).padStart(2, '0')}`,
  }))
  releases.push({ tag_name: 'v0.7.2', prerelease: false, published_at: '2026-10-26' })
  await publish(input, (args) => {
    calls.push(args)
    return args[0] === 'api' ? JSON.stringify([releases]) : ''
  })
  assert.equal(calls.filter((c) => c[1] === 'delete').length, 3)
  assert.ok(!calls.some((c) => c[1] === 'delete' && c.includes('v0.7.2')))
})

test('undeclared nightly.json-prefixed file is refused', (t) => {
  const { input, dir } = fixture(t)
  writeFileSync(join(dir, 'nightly.json.exe'), 'undeclared')
  assert.throws(() => decide(input), /nightly.artifacts/)
})
test('failed alias replacement restores the previous signed nightly', async (t) => {
  const { input, dir } = fixture(t)
  const calls = []
  const prior = { tag_name: 'nightly', name: 'Old Nightly', body: 'previous', draft: false }
  let creates = 0
  await assert.rejects(
    publish(input, (args) => {
      calls.push(args)
      if (args[0] === 'api') return JSON.stringify([[prior]])
      if (args[1] === 'download') {
        cpSync(dir, args[args.indexOf('--dir') + 1], { recursive: true })
        return ''
      }
      if (args[1] === 'create' && args[2] === 'nightly' && ++creates === 1)
        throw Error('GitHub upload failed')
      return ''
    }),
    /previous signed nightly restored/,
  )
  assert.equal(creates, 2)
  assert.equal(calls.at(-1)[2], 'nightly')
  assert.ok(calls.at(-1).includes('Old Nightly'))
})

test('landing page stamps stable tags only, not the newer nightly aliases', () => {
  const result = execFileSync('bash', ['scripts/latest-stable-tag.sh'], {
    input: 'nightly\nnightly-20261027-42\nv0.7.2\nv0.8.0-rc.1\nv0.6.1\nv0.10.0\n',
    encoding: 'utf8',
  })
  assert.equal(result.trim(), 'v0.10.0')
})
test('lost landing-page release and CTA markers refuse publication', (t) => {
  const dir = mkdtempSync(join(tmpdir(), 'nocx-site-'))
  t.after(() => rmSync(dir, { recursive: true, force: true }))
  cpSync('scripts', join(dir, 'scripts'), { recursive: true })
  cpSync('site', join(dir, 'site'), { recursive: true })
  const run = () => spawnSync('bash', [join(dir, 'scripts/check-site.sh')], { encoding: 'utf8' })
  assert.equal(run().status, 0)
  const page = join(dir, 'site/index.html')
  writeFileSync(
    page,
    readFileSync(page, 'utf8')
      .replaceAll('data-released', 'lost-released')
      .replaceAll('<span data-version>', '<span lost-version>'),
  )
  const result = run()
  assert.equal(result.status, 1)
  assert.match(result.stdout, /no <span data-released>/)
  assert.match(result.stdout, /no <span data-version>/)
})

test('partial alias delete failure still restores the old tag and signed release', async (t) => {
  const { input, dir } = fixture(t)
  const calls = []
  let removed = false
  const prior = { tag_name: 'nightly', name: 'Previous', draft: false }
  await assert.rejects(
    publish(input, (args) => {
      calls.push(args)
      if (args[0] === 'api' && args[1] === '--paginate')
        return JSON.stringify([[...(removed ? [] : [prior])]])
      if (args[0] === 'api') return '[]'
      if (args[1] === 'download') {
        cpSync(dir, args.at(-1), { recursive: true })
        return ''
      }
      if (args[1] === 'delete') {
        removed = true
        throw Error('response lost after deletion')
      }
      return ''
    }),
    /previous signed nightly restored/,
  )
  assert.ok(
    calls.some(
      (args) => args.includes('ref=refs/tags/nightly') && args.includes(`sha=${input.sha}`),
    ),
  )
  assert.ok(calls.at(-1).includes('Previous'))
})

test('new workflows admit branch dry runs and Linux-first iteration, never branch publication', () => {
  for (const name of ['nightly', 'release-build']) {
    const yaml = readFileSync(new URL(`../.github/workflows/${name}.yml`, import.meta.url), 'utf8')
    assert.match(yaml, /workflow_dispatch:/)
    assert.match(yaml, /dry-run:/)
    assert.match(yaml, /linux-only:/)
  }
  const yaml = readFileSync(new URL('../.github/workflows/nightly.yml', import.meta.url), 'utf8')
  assert.match(yaml, /authorize\(/)
})

test('branch publication is refused while branch Linux dry run is allowed', () => {
  assert.throws(
    () => authorize({ ref: 'refs/heads/feature', dryRun: false, linuxOnly: false }),
    /requires main/,
  )
  assert.throws(
    () => authorize({ ref: 'refs/heads/main', dryRun: false, linuxOnly: true }),
    /requires both platforms/,
  )
  assert.deepEqual(authorize({ ref: 'refs/heads/feature', dryRun: true, linuxOnly: true }), {
    publish: false,
  })
  assert.deepEqual(authorize({ ref: 'refs/heads/main', dryRun: false, linuxOnly: false }), {
    publish: true,
  })
})
