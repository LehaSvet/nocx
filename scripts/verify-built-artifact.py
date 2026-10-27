#!/usr/bin/env python3
"""Refuse publication unless the packaged executables and identity match the declaration.

Run on the target OS after packaging, before artifact upload. The archive itself
is extracted here; a neighbouring build directory cannot stand in for its bytes.
No GUI, update install, user profile, or keychain is opened. No secret is needed:
the updater reads the real stable signed manifest with its compiled public keys.
"""
import argparse
import hashlib
import json
import os
import plistlib
import re
import stat
import subprocess
import sys
import tempfile
import zipfile
from datetime import datetime
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def sha256(path):
    digest = hashlib.sha256()
    with path.open('rb') as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b''):
            digest.update(chunk)
    return digest.hexdigest()


def require(condition, field, detail):
    if not condition:
        raise ValueError(f'{field}: {detail}')


def run(command, **kwargs):
    result = subprocess.run([str(arg) for arg in command], capture_output=True,
                            text=True, timeout=90, check=False, **kwargs)
    require(result.returncode == 0, Path(command[0]).name,
            f'exit {result.returncode}: {result.stderr.strip()}')
    return result.stdout


def extract(artifact, dest):
    if artifact.name.endswith('.zip'):
        # ditto preserves Mach-O executable bits, bundle symlinks, resource
        # forks and signatures on the actual runner; zipfile handles fixtures
        # on Linux, where no Darwin executable can be run in any event.
        with zipfile.ZipFile(artifact) as archive:
            for member in archive.infolist():
                require(not Path(member.filename).is_absolute() and '..' not in Path(member.filename).parts,
                        'archive', f'unsafe member {member.filename}')
            if sys.platform == 'darwin':
                run(['/usr/bin/ditto', '-x', '-k', artifact, dest])
            else:
                archive.extractall(dest)
                for member in archive.infolist():
                    path = dest / member.filename
                    mode = member.external_attr >> 16
                    if mode and path.is_file():
                        path.chmod(stat.S_IMODE(mode))
    elif artifact.name.endswith('.AppImage'):
        require(os.access(artifact, os.X_OK), 'archive', 'AppImage is not executable')
        run([artifact, '--appimage-extract'], cwd=dest)
    else:
        raise ValueError('archive: expected macOS .zip or Linux .AppImage')
    bundles = list(dest.glob('*.app'))
    if len(bundles) == 1:
        return bundles[0], 'darwin'
    appdir = dest / 'squashfs-root'
    require(appdir.is_dir(), 'archive', 'no single .app bundle or extracted squashfs-root')
    return appdir, 'linux'


def expectations(version):
    module = (ROOT / 'go.mod').read_text()
    wails = re.search(r'github.com/wailsapp/wails/v3\s+(v\S+)', module)
    require(wails is not None, 'Wails', 'no pin in go.mod')
    template = (ROOT / 'build/darwin/Info.plist').read_text()
    bundle_id = re.search(r'<key>CFBundleIdentifier</key>\s*<string>([^<]+)</string>', template)
    require(bundle_id is not None, 'CFBundleIdentifier', 'no declaration in committed plist')
    license_line = re.search(r'Copyright \(c\) (.+)', (ROOT / 'LICENSE').read_text())
    require(license_line is not None, 'NSHumanReadableCopyright', 'no LICENSE owner')
    description = re.search(r'Description:\s*"([^"]+)"', (ROOT / 'main.go').read_text())
    require(description is not None, 'CFBundleGetInfoString', 'no main description')
    return wails.group(1), {
        'CFBundleIdentifier': bundle_id.group(1), 'CFBundleName': 'nocx', 'CFBundleExecutable': 'nocx',
        'CFBundleVersion': version, 'CFBundleShortVersionString': version,
        'CFBundleGetInfoString': description.group(1),
        'NSHumanReadableCopyright': 'Copyright © ' + license_line.group(1),
    }


def inspect(args):
    artifact = args.artifact.resolve()
    require(re.fullmatch(r'\d+\.\d+\.\d+', args.version), 'Version', 'expected MAJOR.MINOR.PATCH')
    require(re.fullmatch(r'[0-9a-f]{40}', args.commit), 'Commit', 'expected full git SHA')
    wails, declared = expectations(args.version)
    archive_hash = sha256(artifact)
    with tempfile.TemporaryDirectory(prefix='nocx-artifact-') as temp:
        package, platform = extract(artifact, Path(temp))
        identity = {}
        if platform == 'darwin':
            with (package / 'Contents/Info.plist').open('rb') as source:
                plist = plistlib.load(source)
            for field, expected in declared.items():
                actual = plist.get(field)
                require(actual == expected, field, f'packaged {actual!r}, declared {expected!r}')
                identity[field] = actual
            bindir = package / 'Contents/MacOS'
            launch = bindir / 'nocx'
        else:
            bindir = package / 'usr/bin'
            launch = package / 'AppRun'
        executables = {}
        for name in ['nocx', 'nocx-server']:
            exe = bindir / name
            require(exe.is_file(), name, 'executable absent from package')
            command = launch if name == 'nocx' else exe
            info = json.loads(run([command, '--build-info']))
            for field in ['Version', 'Commit', 'Date', 'Go', 'Wails', 'Platform']:
                value = info.get(field)
                require(isinstance(value, str) and value.strip() and value.lower() not in ['unknown', 'dev', '(devel)']
                        and 'My Company' not in value and '{{' not in value, field,
                        f'{name} has a template or missing About value: {value!r}')
            for field, expected in [('Version', args.version), ('Commit', args.commit), ('Wails', wails)]:
                require(info[field] == expected, field, f'{name}: {info[field]!r} != declared {expected!r}')
            require(re.fullmatch(platform + r'/(amd64|arm64)', info['Platform']), 'Platform', info['Platform'])
            require(re.fullmatch(r'go\d+\.\d+(?:\.\d+)?', info['Go']), 'Go', info['Go'])
            require(info.get('Development') is False, 'Development', f'{name} is unstamped')
            try:
                datetime.strptime(info['Date'], '%Y-%m-%dT%H:%M:%SZ')
            except ValueError as error:
                raise ValueError(f'Date: {name}: {error}') from error
            executables[name] = {'sha256': sha256(exe), 'about': info}
        require(executables['nocx']['about'] == executables['nocx-server']['about'],
                'About', 'desktop and coordinator build descriptors differ')
        updater = json.loads(run([launch, '--verify-release-manifest']))
        require(isinstance(updater.get('manifest'), dict) and updater['manifest'].get('version'),
                'updater.manifest', 'no verified signed manifest returned')
        require(re.fullmatch(r'[a-f0-9]{64}', updater.get('sha256', '')) and updater.get('signature'),
                'updater.signature', 'no fetched signature/manifest hash evidence')
        require(updater.get('source') == 'https://github.com/shady2k/nocx/releases/latest/download',
                'updater.source', 'not the production stable manifest endpoint')
        require(archive_hash == sha256(artifact), 'archive.sha256', 'artifact changed during inspection')
        return {'schema': 1, 'archive': {'name': artifact.name, 'sha256': archive_hash},
                'version': args.version, 'commit': args.commit, 'declaredIdentity': declared,
                'identity': identity, 'executables': executables, 'updater': updater,
                'limitations': ['No GUI smoke test; metadata uses the same version.Info descriptor as app.about.',
                                'Agent-token propagation through a live coordinator/helper/shell is not exercised.',
                                'Landing pages are separate deployment artifacts, not carried by the desktop archive.']}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--artifact', type=Path, required=True)
    parser.add_argument('--version', required=True)
    parser.add_argument('--commit', required=True)
    parser.add_argument('--report', type=Path, required=True)
    args = parser.parse_args()
    args.report.unlink(missing_ok=True)
    try:
        report = inspect(args)
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(json.dumps(report, indent=2) + '\n')
        print(f'verified {report["archive"]["name"]} ({report["archive"]["sha256"]})')
        return 0
    except (OSError, ValueError, subprocess.TimeoutExpired, zipfile.BadZipFile) as error:
        print(f'::error::built artifact: {error}', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
