import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from build import package_names, resolve_version, tag_version
from publish import prepare_assets, publish


class ReleaseTests(unittest.TestCase):
    def test_tag_version_and_development_builds(self):
        env = {"GITHUB_REF_TYPE": "tag", "GITHUB_REF_NAME": "v0.2.0-rc.1"}
        self.assertEqual(resolve_version(environ=env), "0.2.0-rc.1")
        with self.assertRaises(ValueError):
            resolve_version("0.1.0", env)
        self.assertIn("-dev.aaaaaaaa", resolve_version(environ={"GITHUB_SHA": "a" * 40}))
        for tag in ("0.2.0", "v01.2.3", "v0.2.0-01", "v1.2.3;echo", "v1.2.3/other", "v1.2.3\n"):
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                tag_version(tag)

    def make_packages(self, directory, version="0.2.0"):
        for name in package_names(version).values():
            (Path(directory) / name).write_bytes(b"test package bytes")

    def test_missing_platform_never_calls_github(self):
        with tempfile.TemporaryDirectory() as directory, patch("publish.subprocess.run") as run:
            with self.assertRaises(ValueError):
                publish(directory, "v0.2.0", "example/repo")
            run.assert_not_called()
            self.assertFalse((Path(directory) / "SHA256SUMS.txt").exists())

    def test_checksums_match_every_package(self):
        with tempfile.TemporaryDirectory() as directory:
            self.make_packages(directory)
            assets = prepare_assets(directory, "v0.2.0")
            lines = assets[-1].read_text().splitlines()
            self.assertEqual(len(lines), 3)
            for line in lines:
                digest, name = line.split("  ", 1)
                self.assertEqual(digest, hashlib.sha256((Path(directory) / name).read_bytes()).hexdigest())

    def test_upload_failure_leaves_draft_unpublished(self):
        with tempfile.TemporaryDirectory() as directory, patch("publish.subprocess.run") as run:
            self.make_packages(directory)
            run.side_effect = [subprocess.CompletedProcess([], 1),
                               subprocess.CompletedProcess([], 0),
                               subprocess.CalledProcessError(1, ["gh", "release", "upload"])]
            with self.assertRaises(subprocess.CalledProcessError):
                publish(directory, "v0.2.0", "example/repo")
            self.assertNotIn("edit", [call.args[0][4] for call in run.call_args_list])

    def test_retry_preserves_published_assets(self):
        with tempfile.TemporaryDirectory() as directory, patch("publish.subprocess.run") as run:
            self.make_packages(directory)
            run.return_value = subprocess.CompletedProcess([], 0, json.dumps({"isDraft": False}))
            publish(directory, "v0.2.0", "example/repo")
            self.assertEqual(run.call_count, 1)

    def test_prerelease_uploads_before_publication(self):
        with tempfile.TemporaryDirectory() as directory, patch("publish.subprocess.run") as run:
            self.make_packages(directory, "0.2.0-rc.1")
            run.side_effect = [subprocess.CompletedProcess([], 1)] + [subprocess.CompletedProcess([], 0)] * 3
            publish(directory, "v0.2.0-rc.1", "example/repo")
            commands = [call.args[0] for call in run.call_args_list]
            self.assertEqual([command[4] for command in commands], ["view", "create", "upload", "edit"])
            self.assertIn("--draft", commands[1])
            self.assertIn("--prerelease=true", commands[-1])
            self.assertIn("--latest=false", commands[-1])


if __name__ == "__main__":
    unittest.main()
