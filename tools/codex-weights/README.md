# FIRe weights by Codex

`one.sh <batch>` sends one batch of ML ideas plus the full math catalog to Codex
(`gpt-6.1-sol`, effort high, read-only, JSON schema `schema.json`, prompt head
`head.md`). The 427 ML ideas were split into 14 batches of 31 and run in parallel.
Math → math weights: `math/` (same model, 14 batches over 1233 math topics, 2508 edges; after the math program was cut to the 634 topics ML stands on, the edges among them remain), merged into `data/source/math_weights.json`.

Results were validated (unknown codes dropped, every ML idea present) and merged
into `data/source/weights.json`, which `tools/build_content.py` turns into
`edges` rows with `source = codex-gpt-6.1-sol`.
