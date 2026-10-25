"""Verify all three packages, then upload a draft and publish a tagged release."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess

from build import package_names, tag_version


def prepare_assets(directory, tag):
    version = tag_version(tag)
    directory = Path(directory).resolve()
    packages = [directory / name for name in package_names(version).values()]
    # Validate every platform before writing checksums or touching a Release.
    for package in packages:
        if not package.is_file() or package.stat().st_size == 0:
            raise ValueError("Missing or empty release package: " + package.name)
    checksums = directory / "SHA256SUMS.txt"
    lines = []
    for package in packages:
        with package.open("rb") as f:
            digest = hashlib.file_digest(f, "sha256").hexdigest()
        lines.append(digest + "  " + package.name + "\n")
    checksums.write_text("".join(lines), encoding="utf-8", newline="\n")
    return packages + [checksums]


def publish(directory, tag, repo, dry_run=False):
    assets = prepare_assets(directory, tag)
    if dry_run:
        print("Verified release assets:", ", ".join(p.name for p in assets))
        return
    if not repo:
        raise ValueError("GITHUB_REPOSITORY or --repo is required")
    command = ["gh", "--repo", repo, "release"]
    view = subprocess.run(command + ["view", tag, "--json", "isDraft"], capture_output=True, text=True)
    if view.returncode == 0:
        if not json.loads(view.stdout)["isDraft"]:
            print("Release already published; keeping its existing assets:", tag)
            return
    else:
        args = command + ["create", tag, "--verify-tag", "--draft", "--generate-notes", "--title", "LAN Share " + tag]
        if "-" in tag_version(tag).split("+")[0]:
            args.append("--prerelease")
        # Errors other than a missing release also fail here, without publishing.
        subprocess.run(args, check=True)
    subprocess.run(command + ["upload", tag, *map(str, assets), "--clobber"], check=True)
    prerelease = "-" in tag_version(tag).split("+")[0]
    args = command + ["edit", tag, "--verify-tag", "--draft=false",
                      "--prerelease=" + str(prerelease).lower()]
    if prerelease:
        args.append("--latest=false")
    subprocess.run(args, check=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--directory", type=Path, default=Path("dist"))
    parser.add_argument("--tag", default=os.environ.get("GITHUB_REF_NAME", ""))
    parser.add_argument("--repo", default=os.environ.get("GITHUB_REPOSITORY"))
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()
    publish(args.directory, args.tag, args.repo, args.dry_run)
