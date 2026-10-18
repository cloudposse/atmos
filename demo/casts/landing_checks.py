"""Normalize and validate committed homepage casts using the shared checks."""
import json
import sys
from pathlib import Path
from cast_checks import (assert_body_color, assert_no_error_output, assert_no_experimental,
                         assert_no_local_paths, load_text, sanitize_paths, strip_ansi)


def check(action, name):
    expectations = json.loads(Path('landing-expectations.json').read_text())
    if name not in expectations:
        raise SystemExit(f'Unknown landing demo: {name}')
    path = Path(f'../../website/static/casts/demo/landing/{name}.cast')
    if action == 'sanitize':
        repo = Path('../..').resolve()
        text = path.read_text()
        if repo.is_relative_to(Path.home()):
            abbreviated = '~/' + str(repo.relative_to(Path.home()))
            text = text.replace(abbreviated, '/absolute/path/to/repo')
        path.write_text(sanitize_paths(text, repo))
        return
    if action != 'validate':
        raise SystemExit(f'Unknown action: {action}')
    lines = path.read_text().splitlines()
    header = json.loads(lines[0])
    if name == 'hero' and header.get('term', {}).get('cols', 0) != 64:
        raise SystemExit('hero: use 64 columns to keep homepage text readable')
    if header.get('version') != 3:
        raise SystemExit(f'{name}: expected asciicast v3')
    events = [json.loads(line) for line in lines[1:]]
    if not events or not any(e[1] in ('o', 'e') and e[2].strip() for e in events):
        raise SystemExit(f'{name}: recording is empty')
    text = load_text(path)
    plain = strip_ansi(text)
    assert_body_color(text)
    assert_no_error_output(text)
    assert_no_experimental(text)
    assert_no_local_paths(text)
    if name == 'hero' and '\x1b[999A' in text:
        raise SystemExit('hero: preserve terminal history between commands')
    if name == 'local-ci' and '--ui' in plain:
        raise SystemExit('local-ci: interactive UI does not belong in CI')
    for needle in expectations[name]:
        if needle.lower() not in plain.lower():
            raise SystemExit(f'{name}: missing expected output {needle!r}')
    for unwanted in ('perl ', 'atmos helmfile', 'unknown flag', 'panic:', 'dd-demo-key', 'demo-db-password', 'rotated-demo-password'):
        if unwanted in plain:
            raise SystemExit(f'{name}: unexpected output {unwanted!r}')
    print(f'Validated landing cast: {name}')


if __name__ == '__main__':
    check(*sys.argv[1:])
