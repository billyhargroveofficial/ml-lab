#!/bin/bash
j=$1; cd "$(dirname "$0")"
{ cat head.md; echo; echo "## ML-идеи пачки"; cat $j.ideas.txt; echo; echo "## Каталог математики"; cat math_catalog.txt; } > $j.prompt.md
codex exec -m gpt-6.1-sol -c model_reasoning_effort='"high"' -s read-only --skip-git-repo-check --ephemeral -C . --output-schema schema.json -o $j.result.json - < $j.prompt.md > $j.log 2>&1
