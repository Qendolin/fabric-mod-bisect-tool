#!/usr/bin/env python3
"""Build and package mod-bisect-gui for a target platform.

Usage:
    python3 scripts/build_gui.py --goos linux --goarch amd64 --tag v1.2.0
"""

from __future__ import annotations

import argparse
import os
import re
import shutil
import stat
import subprocess
import sys
import tarfile
import urllib.request
from pathlib import Path

# ── Helpers ───────────────────────────────────────────────────────────────────


def run(*cmd: str | Path, extra_env: dict[str, str] | None = None) -> None:
    """Print and execute a command, merging extra_env into the current environment."""
    env = {**os.environ, **(extra_env or {})}
    print("+", " ".join(str(c) for c in cmd), flush=True)
    subprocess.run([str(c) for c in cmd], check=True, env=env)


def find_one(directory: Path, pattern: str) -> Path:
    """Return the first glob match for pattern under directory, or exit with an error."""
    hits = sorted(directory.glob(pattern))
    if not hits:
        sys.exit(f"ERROR: no '{pattern}' found in {directory}")
    return hits[0]


def patch_toml_version(toml: Path, version: str) -> None:
    text = toml.read_text()
    text = re.sub(r'Version\s*=\s*"[^"]*"', f'Version = "{version}"', text)
    toml.write_text(text)


def fyne_package(
    target: str, git_rev: str, extra_env: dict[str, str] | None = None
) -> None:
    run(
        "fyne",
        "package",
        "--target",
        target,
        "--release",
        "--metadata",
        f"gitRevision={git_rev}",
        extra_env=extra_env,
    )


# ── Linux ─────────────────────────────────────────────────────────────────────


def build_linux(
    goarch: str, git_tag: str, git_rev: str, project_dir: Path, dist: Path
) -> None:
    fyne_package("linux", git_rev)

    tarball = find_one(project_dir, "*.tar.xz")

    # fyne wraps the FHS layout in an extra named subdirectory inside the archive,
    # e.g. "Mod Bisect Tool.tar/mod-bisect-gui/usr/local/...".
    extract_dir = project_dir / "_extract"
    extract_dir.mkdir()
    with tarfile.open(tarball, "r:xz") as tf:
        tf.extractall(extract_dir)
    inner = next(p for p in extract_dir.iterdir() if p.is_dir())

    # Reconstruct a conformant AppDir from the extracted FHS layout.
    appdir = project_dir / "AppDir"
    shutil.copytree(inner / "usr", appdir / "usr")

    # AppImage spec: .desktop and icon must also live at the AppDir root.
    for pattern in (
        "usr/local/share/applications/*.desktop",
        "usr/local/share/pixmaps/*.png",
    ):
        shutil.copy2(find_one(appdir, pattern), appdir)

    # appimagetool hard-errors if Categories= is absent. fyne doesn\'t write it,
    # so patch it into the copy at the AppDir root.
    desktop = find_one(appdir, "*.desktop")
    text = desktop.read_text()
    if "Categories=" not in text:
        desktop.write_text(text.rstrip("\n") + "\nCategories=Utility;\n")

    # Write AppRun — resolves the binary path relative to the AppImage at runtime.
    binary_name = find_one(appdir / "usr/local/bin", "*").name
    apprun = appdir / "AppRun"
    apprun.write_text(
        "#!/bin/sh\n"
        'HERE="$(cd "$(dirname "$0")"; pwd)"\n'
        f'exec "$HERE/usr/local/bin/{binary_name}" "$@"\n'
    )
    apprun.chmod(apprun.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)

    tool_arch = "aarch64" if goarch == "arm64" else "x86_64"
    output = dist / f"mod-bisect-gui-{git_tag}-linux-{goarch}.AppImage"
    tool_search_dir = project_dir.parent.parent
    try:
        appimagetool_path = next(
            tool_search_dir.glob("appimagetool-*.AppImage")
        ).resolve()
    except StopIteration:
        raise FileNotFoundError(
            f"Could not find appimagetool-*.AppImage in {tool_search_dir}"
        )

    run(
        str(appimagetool_path),
        str(appdir),
        extra_env={"VERSION": git_rev, "ARCH": tool_arch},
    )

    # Find the generated AppImage and move it to the expected output path.
    generated = list(project_dir.glob("*.AppImage"))
    if not generated:
        raise FileNotFoundError("appimagetool did not produce an .AppImage")
    shutil.move(generated[0], output)


# ── Windows ───────────────────────────────────────────────────────────────────


def build_windows(
    goarch: str, git_tag: str, git_rev: str, project_dir: Path, dist: Path
) -> None:
    if goarch == "arm64":
        extra_env = {
            "CC": "zig cc -target aarch64-windows-gnu",
            "CXX": "zig c++ -target aarch64-windows-gnu",
        }
    else:
        extra_env = {
            "CC": "x86_64-w64-mingw32-gcc",
            "CXX": "x86_64-w64-mingw32-g++",
        }

    fyne_package("windows", git_rev, extra_env=extra_env)

    exe = find_one(project_dir, "*.exe")
    shutil.move(exe, dist / f"mod-bisect-gui-{git_tag}-windows-{goarch}.exe")


# ── macOS ─────────────────────────────────────────────────────────────────────


def build_darwin(
    goarch: str, git_tag: str, git_rev: str, project_dir: Path, dist: Path
) -> None:
    fyne_package("darwin", git_rev)

    app = find_one(project_dir, "*.app")
    output = dist / f"mod-bisect-gui-{git_tag}-darwin-{goarch}.zip"
    # ditto preserves macOS resource forks and extended attributes that zip -r drops.
    run("ditto", "-c", "-k", "--sequesterRsrc", "--keepParent", app, output)


# ── Entry point ───────────────────────────────────────────────────────────────

BUILDERS = {
    "linux": build_linux,
    "windows": build_windows,
    "darwin": build_darwin,
}


def main() -> None:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--goos", required=True, choices=BUILDERS.keys())
    p.add_argument("--goarch", required=True, choices=["amd64", "arm64"])
    p.add_argument("--tag", required=True, help="Release tag, e.g. v1.2.0")
    p.add_argument(
        "--project-dir",
        default="cmd/mod-bisect-gui",
        help="Path to the GUI module directory (default: cmd/mod-bisect-gui)",
    )
    p.add_argument(
        "--dist",
        default="dist",
        help="Output directory for built artifacts (default: dist)",
    )
    args = p.parse_args()

    project_dir = Path(args.project_dir).resolve()
    dist = Path(args.dist).resolve()
    dist.mkdir(parents=True, exist_ok=True)

    version = args.tag.lstrip("v")
    git_rev = subprocess.check_output(
        ["git", "rev-parse", "--short", "HEAD"],
        text=True,
    ).strip()

    patch_toml_version(project_dir / "FyneApp.toml", version)

    # fyne package must run from inside the project directory.
    os.chdir(project_dir)

    BUILDERS[args.goos](args.goarch, args.tag, git_rev, project_dir, dist)
    print(f"\nDone. Output in {dist}", flush=True)


if __name__ == "__main__":
    main()
