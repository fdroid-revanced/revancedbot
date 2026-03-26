import subprocess
import tempfile
from pathlib import Path
from github import Github

from .models import PatchJob

class Patcher():
    def __init__(self, tool_location: Path =None):
        if tool_location is None:
            tool_location = Path(tempfile.mkdtemp()).parent / "revancedbot"
        self.tool_location = tool_location
        self._started = None

    @property
    def patch_file(self):
        return self.tool_location / "patches.rvp"

    @property
    def patcher_file(self):
        return self.tool_location / "patcher.jar"

    def _startup(self):
        if self._started is not None:
            return
        g = Github()
        self.tool_location.mkdir(parents=True, exist_ok=True)
        if not self.patch_file.exists():
            latest_patch_release = g.get_repo("ReVanced/revanced-patches").get_latest_release()
            patch_asset = [p for p in latest_patch_release.assets if p.name.endswith(".rvp")][0]
            patch_asset.download_asset(self.patch_file)

        if not self.patcher_file.exists():
            latest_patcher_release = g.get_repo("ReVanced/revanced-cli").get_latest_release()
            patcher_asset = [p for p in latest_patcher_release.assets if p.name.endswith(".jar")][0]
            patcher_asset.download_asset(self.patcher_file)
        self._started = True

    def __call__(self, *args, stdin=None, stdout=None, stderr=None):
        self._startup()
        return subprocess.run(
            ["java", "-jar", self.patcher_file, *args],
            stdin=stdin,
            stdout=stdout,
            stderr=stderr
        )

    @property
    def jobs(self):
        data = self("list-versions", self.patch_file, stdout=subprocess.PIPE).stdout.decode()
        for package in data.split("Package name: "):
            package_parts = package.split("Most common compatible versions:")
            if len(package_parts) != 2:
                continue
            package_id = package_parts[0].strip()
            rest = package_parts[1]
            for version in rest.split('\n'):
                version = version.strip().split(' ')[0]
                if version == '':
                    continue
                yield PatchJob(package_id=package_id, package_version=None if version == 'Any' else version)
