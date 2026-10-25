"""Collect licenses of all pinned modules, including the test-only SMB client."""
import json
import pathlib
import subprocess

root = pathlib.Path(__file__).resolve().parents[1]
data = subprocess.check_output(["go", "list", "-m", "-json", "all"], cwd=root, text=True)
decoder = json.JSONDecoder()
modules = []
while data.strip():
    data = data.lstrip()
    module, end = decoder.raw_decode(data)
    data = data[end:]
    if not module.get("Main"):
        modules.append(module)

dest = root / "licenses"
dest.mkdir(exist_ok=True)
notice = ["Third-party notices", "", "Pinned dependencies, including test-only dependencies. Original license text is included below this directory.", ""]
for module in modules:
    source = pathlib.Path(module.get("Dir", ""))
    files = [p for p in source.iterdir() if p.is_file() and p.name.upper().startswith(("LICENSE", "COPYING", "NOTICE"))] if module.get("Dir") else []
    if not files:
        raise RuntimeError("Missing license for " + module["Path"])
    name = module["Path"].replace("/", "_") + "@" + module["Version"]
    for item in files:
        (dest / (name + "_" + item.name + ".txt")).write_bytes(item.read_bytes())
    notice.append("- " + module["Path"] + " " + module["Version"])
(root / "THIRD_PARTY_NOTICES.md").write_text("\n".join(notice) + "\n")
print("Collected licenses for", len(modules), "modules")
