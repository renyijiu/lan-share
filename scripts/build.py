"""Build portable release packages with the same version on every platform."""
import argparse
import os
from pathlib import Path
import plistlib
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
SEMVER = re.compile(
    r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
    r"(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?"
    r"(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?"
)


def validate_version(value):
    match = SEMVER.fullmatch(value)
    if not match:
        raise ValueError("Version must use SemVer, for example 0.2.0 or 0.2.0-rc.1")
    for part in (match.group(4) or "").split("."):
        if part.isdigit() and len(part) > 1 and part.startswith("0"):
            raise ValueError("Numeric prerelease identifiers cannot have leading zeroes")
    return value


def tag_version(tag):
    if not tag.startswith("v"):
        raise ValueError("Release tags must start with v, for example v0.2.0")
    return validate_version(tag[1:])


def resolve_version(explicit=None, environ=None):
    env = os.environ if environ is None else environ
    if env.get("GITHUB_REF_TYPE") == "tag":
        version = tag_version(env["GITHUB_REF_NAME"])
        if explicit and explicit != version:
            raise ValueError("Build version must match the release tag")
        return version
    version = validate_version(explicit or env.get("VERSION") or (ROOT / "VERSION").read_text().strip())
    if not explicit and not env.get("VERSION") and env.get("GITHUB_SHA"):
        sha = env["GITHUB_SHA"]
        if not re.fullmatch(r"[0-9a-f]{40,64}", sha):
            raise ValueError("Invalid commit SHA")
        base, _, metadata = version.partition("+")
        base += ("." if "-" in base else "-") + "dev." + sha[:8]
        version = base + ("+" + metadata if metadata else "")
    return validate_version(version)


def package_names(version):
    validate_version(version)
    prefix = "LAN-Share-v" + version
    return {
        "macos": prefix + "-macOS-universal.zip",
        "windows": prefix + "-Windows-amd64.zip",
        "linux": prefix + "-Linux-amd64.tar.gz",
    }


def run(*args, **kwargs):
    subprocess.run([str(arg) for arg in args], check=True, cwd=ROOT, **kwargs)


def go_build(output, goos, arch, version, helper=False):
    flags = "-s -w"
    if not helper:
        flags += (" -X main.version=" + version
                  + " -X github.com/egoist/mygo.production=1"
                  + " -X github.com/egoist/mygo.packageIdentifier=io.github.renyijiu.lanshare"
                  + " -X github.com/egoist/mygo.packageVersion=" + version)
        if goos == "windows":
            flags += " -H=windowsgui"
    env = dict(os.environ, CGO_ENABLED="0", GOOS=goos, GOARCH=arch)
    run("go", "build", "-trimpath", "-tags", "mygo_noinspector", "-ldflags", flags,
        "-o", output, "./cmd/port-helper" if helper else "./cmd/lan-share", env=env)


def copy_notices(destination):
    shutil.copytree(ROOT / "licenses", destination / "licenses")
    shutil.copytree(ROOT / "docs", destination / "docs")
    for name in ("LICENSE", "README.md", "THIRD_PARTY_NOTICES.md"):
        shutil.copy2(ROOT / name, destination / name)


def verify_version(binary, version):
    result = subprocess.run([str(binary), "--version"], check=True, capture_output=True, text=True, timeout=30)
    if result.stdout.strip() != version:
        raise ValueError("The executable version does not match the package version")


def build(platform, output, version):
    output = Path(output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    archive = output / package_names(version)[platform]
    with tempfile.TemporaryDirectory(prefix="lan-share-build-") as temp:
        temp = Path(temp)
        if platform == "macos":
            if sys.platform != "darwin":
                raise ValueError("The macOS package requires a macOS host for lipo and codesign")
            app = temp / "LAN Share.app"
            binaries = app / "Contents/MacOS"
            resources = app / "Contents/Resources"
            binaries.mkdir(parents=True)
            resources.mkdir(parents=True)
            for arch in ("arm64", "amd64"):
                go_build(temp / ("main-" + arch), "darwin", arch, version)
                go_build(temp / ("helper-" + arch), "darwin", arch, version, helper=True)
            for prefix, name in (("main", "lan-share"), ("helper", "lan-share-port-helper")):
                run("lipo", "-create", temp / (prefix + "-arm64"), temp / (prefix + "-amd64"),
                    "-output", binaries / name)
            with (ROOT / "resources/Info.plist").open("rb") as f:
                info = plistlib.load(f)
            info["CFBundleShortVersionString"] = version.split("-")[0].split("+")[0]
            info["CFBundleVersion"] = os.environ.get("GITHUB_RUN_NUMBER", "1")
            if not re.fullmatch(r"[1-9][0-9]*", info["CFBundleVersion"]):
                raise ValueError("Invalid macOS build number")
            with (app / "Contents/Info.plist").open("wb") as f:
                plistlib.dump(info, f)
            copy_notices(resources)
            shutil.copy2(ROOT / "resources/AppIcon.icns", resources / "AppIcon.icns")
            run("codesign", "--force", "--sign", "-", binaries / "lan-share-port-helper")
            run("codesign", "--force", "--sign", "-", app)
            run("codesign", "--verify", "--deep", "--strict", app)
            verify_version(binaries / "lan-share", version)
            run("ditto", "-c", "-k", "--sequesterRsrc", "--keepParent", app, archive)
            target = output / "LAN Share.app"
            if target.exists():
                shutil.rmtree(target)
            shutil.copytree(app, target)
        else:
            package = temp / "LAN-Share"
            package.mkdir()
            binary = package / ("lan-share.exe" if platform == "windows" else "lan-share")
            go_build(binary, platform, "amd64", version)
            binary.chmod(0o755)
            if (platform == "windows" and sys.platform == "win32") or (platform == "linux" and sys.platform.startswith("linux")):
                verify_version(binary, version)
            copy_notices(package)
            if platform == "windows":
                with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as z:
                    for path in sorted(package.rglob("*")):
                        if path.is_file():
                            z.write(path, path.relative_to(temp))
            else:
                with tarfile.open(archive, "w:gz") as t:
                    t.add(package, arcname="LAN-Share")
    print("Built:", archive)
    return archive


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--platform", choices=("macos", "windows", "linux"), required=True)
    parser.add_argument("--output", type=Path, default=ROOT / "dist")
    parser.add_argument("--version")
    args = parser.parse_args()
    build(args.platform, args.output, resolve_version(args.version))
