"""Reject action configuration accidentally embedded in a shell run block."""
from pathlib import Path
import re


def check(source):
    run_indent = None
    for number, line in enumerate(source.splitlines(), 1):
        if not line.strip():
            continue
        indent = len(line) - len(line.lstrip())
        if run_indent is not None and indent <= run_indent:
            run_indent = None
        if re.match(r"\s*run:\s*[|>]", line):
            run_indent = indent
        elif run_indent is not None and re.match(
                r"\s*(compression-level|retention-days|if-no-files-found):", line):
            raise ValueError(f"action input inside a shell run block at line {number}")


# The exact class of the failed post-publication command must be rejected.
try:
    check("run: |\n  echo ready\n  compression-level: 9\n")
except ValueError:
    pass
else:
    raise AssertionError("misplaced action input was accepted")
check("run: |\n  echo ready\nwith:\n  compression-level: 9\n")
for workflow in Path(".github/workflows").glob("*.yml"):
    check(workflow.read_text(encoding="utf-8"))
print("WORKFLOW_RUN_BLOCKS_OK")
