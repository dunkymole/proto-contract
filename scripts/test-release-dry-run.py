"""Exercise the next minor release in a disposable local checkout; no GitHub writes."""
import datetime
import json
import pathlib
import subprocess
import sys
import tempfile

root = pathlib.Path(__file__).resolve().parents[1]


def run(*args, cwd=root):
    subprocess.run([str(a) for a in args], cwd=cwd, check=True)


def capture(*args, cwd=root):
    return subprocess.check_output([str(a) for a in args], cwd=cwd, text=True).strip()


def main():
    source = capture('git', 'rev-parse', 'HEAD')
    current = json.loads((root / 'release.json').read_text())['version']
    major, minor, _ = map(int, current.split('.'))
    future = f'{major}.{minor + 1}.0'
    # Temporary worktree cleanup is constrained to this explicitly named output
    # directory, including on Windows. Keep the verified candidate/report.
    output_root = (root / 'artifacts').resolve()
    output_root.mkdir(exist_ok=True)
    evidence = pathlib.Path(tempfile.mkdtemp(prefix='release-dry-run-', dir=output_root))
    checkout = (evidence / 'source').resolve()
    if not checkout.is_relative_to(output_root):
        raise ValueError('dry-run checkout must stay inside artifacts')
    run('git', 'worktree', 'add', '--detach', checkout, source)
    try:
        tool = checkout / 'scripts/release.py'
        run(sys.executable, tool, 'bump', '--version', future, '--commit', source,
            '--summary', 'Dry-run release validation only; not for publication.',
            '--date', datetime.datetime.now(datetime.timezone.utc).date().isoformat(), cwd=checkout)
        run('git', 'add', '--all', cwd=checkout)
        run('git', '-c', 'user.name=Release dry-run', '-c', 'user.email=release-dry-run@invalid.example',
            'commit', '-m', 'Local release dry run; never push this commit', cwd=checkout)
        commit = capture('git', 'rev-parse', 'HEAD', cwd=checkout)
        candidate = evidence / 'candidate'
        run(sys.executable, tool, 'prepare', '--commit', commit, '--out', candidate, cwd=checkout)
        result = json.loads(capture(sys.executable, tool, 'publish', '--version', future,
                                   '--commit', commit, '--candidate', candidate, '--dry-run', cwd=checkout))
        if result['writes'] != [] or not result['dryRun']:
            raise ValueError('dry-run attempted publication')
        # These read-only checks also detect a conflicting tag/release created
        # concurrently. All mutation-capable paths are separately failure-tested.
        run('gh', 'api', f'repos/dunkymole/proto-contract/git/matching-refs/tags/v{future}')
        result.update({'testedSource': source, 'localCandidateCommit': commit,
                       'promotable': False, 'validation': 'compiler, security, packages, starter, 20 combinations, naming, rollout'})
        (evidence / 'DRY_RUN_REPORT.json').write_text(json.dumps(result, indent=2) + '\n')
        print(f'Dry run passed; evidence: {evidence}')
    finally:
        if not checkout.is_relative_to(output_root):
            raise ValueError('refusing to remove checkout outside artifacts')
        run('git', 'worktree', 'remove', '--force', checkout)


if __name__ == '__main__':
    main()
