"""Record the task delta against its dirty-workspace baseline, not Git HEAD."""

import difflib
import hashlib
import json
from pathlib import Path
import tarfile


root = Path(__file__).resolve().parents[4]
output = Path(__file__).resolve().parent
archive = root / ".semlia/evidence-work/SP-T004-A002-baseline.tar"
before = {}
directories = set()
with tarfile.open(archive) as source:
    for member in source:
        if any(part.startswith("._") for part in Path(member.name).parts):
            continue
        if member.isdir():
            directories.add(member.name.rstrip("/"))
        elif member.isfile():
            before[member.name] = source.extractfile(member).read()

paths = set(before)
for directory in directories:
    paths.update(str(path.relative_to(root)) for path in (root / directory).rglob("*") if path.is_file())

changed = []
patch = []
for name in sorted(paths):
    if name.startswith("docs/evidence/"):
        continue
    path = root / name
    old = before.get(name)
    current = path.read_bytes() if path.is_file() else None
    if old == current:
        continue
    changed.append({
        "path": name,
        "status": "added" if old is None else "deleted" if current is None else "modified",
        "before_sha256": hashlib.sha256(old).hexdigest() if old is not None else None,
        "after_sha256": hashlib.sha256(current).hexdigest() if current is not None else None,
    })
    try:
        old_lines = (old or b"").decode("utf-8").splitlines(keepends=True)
        new_lines = (current or b"").decode("utf-8").splitlines(keepends=True)
    except UnicodeDecodeError:
        patch.append("Binary files differ: " + name + "\n")
        continue
    patch.extend(difflib.unified_diff(
        old_lines,
        new_lines,
        fromfile="a/" + name if old is not None else "/dev/null",
        tofile="b/" + name if current is not None else "/dev/null",
    ))

preserved = []
for name, content in sorted(before.items()):
    if name.startswith("migrations/"):
        match = (root / name).read_bytes() == content
        preserved.append({"path": name, "unchanged": match, "sha256": hashlib.sha256(content).hexdigest()})
        if not match:
            raise SystemExit("Existing migration changed: " + name)

(output / "source-delta.json").write_text(json.dumps(changed, indent=2) + "\n")
(output / "source.diff").write_text("".join(patch))
(output / "preserved-migrations.json").write_text(json.dumps(preserved, indent=2) + "\n")
print(json.dumps({"changed_files": len(changed), "preserved_migrations": len(preserved)}))
