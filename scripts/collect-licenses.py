"""Collect licenses of all pinned modules, including the test-only SMB client.

Output names omit versions, which go.mod already records, so a dependency
upgrade only changes these files when the license text itself changes.
"""
import json
import pathlib
import subprocess

root = pathlib.Path(__file__).resolve().parents[1]
# Downloading the whole module graph adds hashes that go mod tidy removes again.
go_sum = root / "go.sum"
tidy = go_sum.read_bytes()
try:
    subprocess.run(["go", "mod", "download", "all"], cwd=root, check=True)
    data = subprocess.check_output(["go", "list", "-m", "-json", "all"], cwd=root, text=True)
finally:
    go_sum.write_bytes(tidy)
decoder = json.JSONDecoder()
modules = []
while data.strip():
    data = data.lstrip()
    module, end = decoder.raw_decode(data)
    data = data[end:]
    if not module.get("Main"):
        modules.append(module)

dest = root / "licenses"
# Start empty so licenses of dropped or upgraded modules do not linger.
if dest.exists():
    for stale in dest.iterdir():
        stale.unlink()
dest.mkdir(exist_ok=True)
notice = ["Third-party notices", "", "Dependencies, including test-only dependencies. Versions are pinned in go.mod; original license text is included in the licenses directory.", ""]
for module in modules:
    source = pathlib.Path(module.get("Dir", ""))
    files = [p for p in source.iterdir() if p.is_file() and p.name.upper().startswith(("LICENSE", "COPYING", "NOTICE"))] if module.get("Dir") else []
    if not files:
        raise RuntimeError("Missing license for " + module["Path"])
    name = module["Path"].replace("/", "_")
    for item in files:
        (dest / (name + "_" + item.name + ".txt")).write_bytes(item.read_bytes())
    notice.append("- " + module["Path"])
(root / "THIRD_PARTY_NOTICES.md").write_text("\n".join(notice) + "\n")
print("Collected licenses for", len(modules), "modules")
