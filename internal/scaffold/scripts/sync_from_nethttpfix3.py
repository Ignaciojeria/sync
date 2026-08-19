#!/usr/bin/env python3
"""Mirror nethttpfix3 into the einarc scaffold testdata dir.

Excludes anything the scaffold test considers out-of-scope
(.git, .einar, tmp, bin, .exe, .env, *.log, *.lock) plus
.pi/tmp runtime data.
"""
import os
import shutil
import sys

SRC = r"C:\_git\einarc\nethttpfix3"
DST = r"C:\_git\einarc\internal\scaffold\testdata\app-mobile-downloader"

EXCLUDE_DIRS = {".git", ".einar", "tmp", "bin"}
EXCLUDE_DIR_PREFIXES = (".pi/tmp",)
EXCLUDE_FILES = {".env", ".air.log", ".air.pid", "mutagen.yml.lock"}
EXCLUDE_SUFFIXES = (".exe", ".exe~", ".log")


def should_skip(rel: str) -> bool:
    parts = rel.replace("\\", "/").split("/")
    for p in parts:
        if p in EXCLUDE_DIRS:
            return True
    for prefix in EXCLUDE_DIR_PREFIXES:
        if rel.replace("\\", "/").startswith(prefix + "/"):
            return True
    name = parts[-1]
    if name in EXCLUDE_FILES:
        return True
    for suf in EXCLUDE_SUFFIXES:
        if name.endswith(suf):
            return True
    return False


def main() -> int:
    if os.path.exists(DST):
        shutil.rmtree(DST)
    os.makedirs(DST, exist_ok=True)
    count = 0
    for root, dirs, files in os.walk(SRC):
        rel_root = os.path.relpath(root, SRC)
        if rel_root == ".":
            rel_root = ""
        # filter out excluded dirs in-place so os.walk skips them
        kept_dirs = []
        for d in dirs:
            rel = os.path.join(rel_root, d).replace("\\", "/")
            if should_skip(rel):
                continue
            kept_dirs.append(d)
        dirs[:] = kept_dirs
        for f in files:
            rel = os.path.join(rel_root, f).replace("\\", "/")
            if should_skip(rel):
                continue
            src_path = os.path.join(root, f)
            dst_path = os.path.join(DST, rel)
            os.makedirs(os.path.dirname(dst_path), exist_ok=True)
            shutil.copy2(src_path, dst_path)
            count += 1
    print(f"copied {count} files into {DST}")
    # rename go.mod -> go.mod.tmpl
    gomod = os.path.join(DST, "go.mod")
    if os.path.exists(gomod):
        shutil.move(gomod, os.path.join(DST, "go.mod.tmpl"))
        print("renamed go.mod -> go.mod.tmpl")
    return 0


if __name__ == "__main__":
    sys.exit(main())