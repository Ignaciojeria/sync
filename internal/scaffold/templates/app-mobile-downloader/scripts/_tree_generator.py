#!/usr/bin/env python3
"""
Genera STRUCTURE.md con el árbol real del proyecto (no una lista plana).

La lógica es portable y se configura vía `scripts/structure.config.toml`.
Esto permite reutilizar el generador en otros repos o mover el motor al CLI
sin acoplarlo a convenciones específicas de este proyecto.
"""

from __future__ import annotations

import os
import sys
import tomllib
from fnmatch import fnmatch
from pathlib import Path
from typing import Any

DEFAULT_CONFIG: dict[str, Any] = {
    "tree": {
        "exclude_dirs": [
            ".git",
            ".einar",
            "tmp",
            "node_modules",
            ".npm-cache",
            ".agents",
            "doc",
            ".githooks",
            ".vscode",
            "__pycache__",
        ],
        "exclude_files": [
            ".env",
            "go.sum",
            "mutagen.yml",
            "mutagen.yml.lock",
            "skills-lock.json",
            "workspaces.yaml",
            "wede.config.json",
            "coverage.out",
            "coverage_filtered.out",
            "coverage.html",
            "last_run.json",
        ],
        "exclude_globs": [
            "*.jpg",
            "*.jpeg",
            "*.png",
            "*.ico",
            "*.log",
            "*.lock",
            "*.csv",
            "*.json",
        ],
        "include_globs": [
            "*.go",
            "*.templ",
            "*.md",
            "*.sh",
            "*.toml",
            "*.yml",
            "*.yaml",
            "*.mod",
            "*.sql",
            "*.py",
        ],
        "skip_hidden_dirs": True,
        "files_first": True,
    },
    "document": {
        "title": "Estructura del Proyecto",
        "regenerate_command": "scripts/generate-structure.sh",
        "footer_heading": "Convenciones de estructura",
        "conventions": [
            "Cada módulo de negocio vive en `internal/<modulo>/` con sus capas: `application`, `http`, `infrastructure`, `ui`.",
            "Código compartido: `internal/shared/` (config, auth, server, infra).",
            "Punto de entrada: `cmd/api/main.go`.",
            "Plantillas: `internal/<modulo>/ui/` o `internal/ui/layout/`.",
            "Tests: junto al código (`*_test.go`).",
            "Skills: `.agents/skills/`.",
            "Scripts: `scripts/` para automatización (ej. `generate-structure.sh`).",
        ],
    },
}

TEE = "├── "
LAST = "└── "
CLONE = "│   "
BLANK = "    "


def deep_merge(base: dict[str, Any], override: dict[str, Any]) -> dict[str, Any]:
    merged = dict(base)
    for key, value in override.items():
        if isinstance(value, dict) and isinstance(merged.get(key), dict):
            merged[key] = deep_merge(merged[key], value)
        else:
            merged[key] = value
    return merged


def load_config(config_path: Path | None) -> dict[str, Any]:
    config = DEFAULT_CONFIG
    if config_path is None or not config_path.is_file():
        return config

    with config_path.open("rb") as fh:
        user_config = tomllib.load(fh)
    return deep_merge(config, user_config)


def skip_dir(name: str, config: dict[str, Any]) -> bool:
    tree_cfg = config["tree"]
    if name in set(tree_cfg["exclude_dirs"]):
        return True
    if tree_cfg.get("skip_hidden_dirs", True) and name.startswith("."):
        return True
    return False


def include_file(name: str, config: dict[str, Any]) -> bool:
    tree_cfg = config["tree"]

    if name in set(tree_cfg["exclude_files"]):
        return False

    for pattern in tree_cfg["exclude_globs"]:
        if fnmatch(name, pattern):
            return False

    for pattern in tree_cfg["include_globs"]:
        if fnmatch(name, pattern):
            return True

    return False


class Node:
    __slots__ = ("name", "is_dir", "children")

    def __init__(self, name: str, is_dir: bool) -> None:
        self.name = name
        self.is_dir = is_dir
        self.children: list[Node] = []


def build(root_path: str, config: dict[str, Any]) -> Node:
    root = Node(".", is_dir=True)
    _walk(root_path, root, config)
    return root


def _walk(path: str, parent: Node, config: dict[str, Any]) -> None:
    try:
        entries = sorted(os.listdir(path))
    except PermissionError:
        return

    dirs: list[str] = []
    files: list[str] = []

    for entry in entries:
        full = os.path.join(path, entry)

        if os.path.isdir(full):
            if not skip_dir(entry, config):
                dirs.append(entry)
        elif os.path.isfile(full):
            if include_file(entry, config):
                files.append(entry)

    if config["tree"].get("files_first", True):
        ordered = [(name, False) for name in files] + [(name, True) for name in dirs]
    else:
        ordered = [(name, True) for name in dirs] + [(name, False) for name in files]

    for name, is_dir in ordered:
        if is_dir:
            child = Node(name + "/", is_dir=True)
            parent.children.append(child)
            _walk(os.path.join(path, name), child, config)
        else:
            parent.children.append(Node(name, is_dir=False))


def render(root: Node) -> list[str]:
    lines: list[str] = [root.name]
    last_idx = len(root.children) - 1
    for i, child in enumerate(root.children):
        lines.extend(_render_node(child, i == last_idx, []))
    return lines


def _render_node(node: Node, is_last: bool, prefix: list[str]) -> list[str]:
    connector = LAST if is_last else TEE
    lines: list[str] = ["".join(prefix) + connector + node.name]

    child_prefix = prefix + [BLANK if is_last else CLONE]
    last_child = len(node.children) - 1
    for i, child in enumerate(node.children):
        lines.extend(_render_node(child, i == last_child, child_prefix))
    return lines


def build_markdown(body: str, config: dict[str, Any]) -> str:
    doc_cfg = config["document"]
    conventions = "\n".join(f"- {item}" for item in doc_cfg["conventions"])

    return (
        f"# {doc_cfg['title']}\n"
        "\n"
        "> **Archivo generado automáticamente.** "
        f"Ejecutar `{doc_cfg['regenerate_command']}` para regenerar.\n"
        "> **No editar manualmente.**\n"
        "\n"
        "```\n"
        f"{body}\n"
        "```\n"
        "\n"
        "---\n"
        "\n"
        f"## {doc_cfg['footer_heading']}\n"
        "\n"
        f"{conventions}\n"
    )


def main() -> int:
    if len(sys.argv) < 3:
        print(
            "Uso: _tree_generator.py <root_dir> <output_md> [config_toml]",
            file=sys.stderr,
        )
        return 2

    root_dir = Path(sys.argv[1]).resolve()
    output = Path(sys.argv[2]).resolve()

    if len(sys.argv) >= 4:
        config_path = Path(sys.argv[3]).resolve()
    else:
        config_path = Path(__file__).with_name("structure.config.toml")

    config = load_config(config_path)
    tree = build(str(root_dir), config)
    body = "\n".join(render(tree))
    md = build_markdown(body, config)

    output.write_text(md, encoding="utf-8")
    return 0


if __name__ == "__main__":
    sys.exit(main())
