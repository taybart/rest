#!/usr/bin/env python3
"""Regenerate gomod2nix.toml without nix.

Equivalent to `gomod2nix generate` for this repo: lists modules with
`go mod download -json` and hashes each module's source directory the way nix
does (sha256 of its NAR serialization, in SRI form).

Usage (from the repo root): python3 scripts/gomod2nix.py
"""
import base64
import hashlib
import json
import os
import stat
import struct
import subprocess
import sys


def _str(h, b):
    if isinstance(b, str):
        b = b.encode()
    h.update(struct.pack("<Q", len(b)))
    h.update(b)
    h.update(b"\0" * ((8 - len(b) % 8) % 8))


def _dump(h, path):
    st = os.lstat(path)
    _str(h, "(")
    if stat.S_ISLNK(st.st_mode):
        for s in ("type", "symlink", "target", os.readlink(path)):
            _str(h, s)
    elif stat.S_ISDIR(st.st_mode):
        _str(h, "type")
        _str(h, "directory")
        for name in sorted(os.listdir(path), key=os.fsencode):
            for s in ("entry", "(", "name", name, "node"):
                _str(h, s)
            _dump(h, os.path.join(path, name))
            _str(h, ")")
    else:
        _str(h, "type")
        _str(h, "regular")
        if st.st_mode & stat.S_IXUSR:
            _str(h, "executable")
            _str(h, "")
        _str(h, "contents")
        with open(path, "rb") as f:
            _str(h, f.read())
    _str(h, ")")


def nar_hash(path):
    h = hashlib.sha256()
    _str(h, "nix-archive-1")
    _dump(h, path)
    return "sha256-" + base64.b64encode(h.digest()).decode()


def downloaded_modules():
    out = subprocess.run(
        ["go", "mod", "download", "-json"], capture_output=True, text=True, check=True
    ).stdout
    dec = json.JSONDecoder()
    i = 0
    while True:
        while i < len(out) and out[i].isspace():
            i += 1
        if i >= len(out):
            return
        mod, i = dec.raw_decode(out, i)
        if mod.get("Main"):
            continue
        if mod.get("Error"):
            sys.exit(f"go mod download {mod['Path']}: {mod['Error']}")
        if mod.get("Replace"):
            # gomod2nix records replacements differently; don't guess
            sys.exit(f"{mod['Path']} is replaced; use `nix run .#gomod2nix -- generate`")
        yield mod


def main():
    mods = sorted(downloaded_modules(), key=lambda m: m["Path"].encode())
    lines = ["schema = 3", "", "[mod]"]
    for i, m in enumerate(mods):
        if i:
            lines.append("")
        lines += [
            f"  [mod.'{m['Path']}']",
            f"    version = '{m['Version']}'",
            f"    hash = '{nar_hash(m['Dir'])}'",
        ]
    with open("gomod2nix.toml", "w") as f:
        f.write("\n".join(lines) + "\n")
    print(f"wrote gomod2nix.toml ({len(mods)} modules)")


if __name__ == "__main__":
    main()
