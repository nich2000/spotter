from pathlib import Path
import re
root=Path(__file__).resolve().parents[3]
errors=[]
for doc in (root/'docs').glob('*-spec.md'):
    for target in re.findall(r'\]\(([^)]+)\)',doc.read_text()):
        if '://' in target or target.startswith('#'):
            continue
        path=target.split('#',1)[0]
        if not (doc.parent/path).exists():
            errors.append(f'{doc.name}: {target}')
if errors:
    raise SystemExit('\n'.join(errors))
print('Specification file links: OK')
