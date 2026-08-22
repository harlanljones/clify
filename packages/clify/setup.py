import os

from setuptools import setup
from setuptools.command.build_py import build_py

MANIFESTS = [
    "agent_manifest.json",
    "agent_manifest.cliamp.json",
    "agent_manifest.cliamp_playback.json",
    "agent_manifest.cliamp_library.json",
]


class build_py_with_manifests(build_py):
    """Copy runtime-loaded scope manifests next to the installed modules."""

    def run(self):
        super().run()
        for name in MANIFESTS:
            self.copy_file(name, os.path.join(self.build_lib, name))


setup(cmdclass={"build_py": build_py_with_manifests})
