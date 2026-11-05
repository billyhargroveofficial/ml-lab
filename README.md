# FIRe Lab

A local spaced-repetition app that schedules lessons and reviews over a **knowledge graph**, with content written by your own coding agent.

- **FSRS-6** models memory: each learned idea has a stability and a difficulty, and comes back for review when recall drops below 90%.
- **FIRe** (fractional implicit repetition, from Justin Skycak's *The Math Academy Way*) counts a solved lesson as partial review of the simpler ideas it contains, weighted by `encompassing` links. You repeat less and keep the same recall. Implicit credit is capped, so ideas that live only on it still get explicit checks.
- **A placement test** marks what you already know with a few dozen questions. It works like a binary search over the graph.
- **No curriculum is included.** Content is *packs*: folders in `packs/` with lessons in Markdown + KaTeX and multiple-choice questions with explanations. You ask an agent for a topic, and it writes, links and validates the pack. The app picks it up within seconds.
- **No accounts, no server.** Progress is stored in `data/progress.db` on your machine. You can export and import it.

## Run

You need Go 1.25+ and Node.js 20+.

```sh
git clone https://github.com/billyhargroveofficial/ml-lab.git && cd ml-lab
./fire dev            # Windows: fire dev
```

Open http://127.0.0.1:5190. The first run installs the web dependencies.

## Add a topic

Open the folder in Claude Code, Codex, Cursor or any agent that reads `AGENTS.md`, and ask in your own words, for example:

> I struggle to read quantifiers. Make me 6 lessons with 5–7 problems each, from trivial to brutal.

The agent follows [`.agents/skills/add-topic`](.agents/skills/add-topic/SKILL.md):

1. It finds existing ideas to link to with `./fire search`.
2. It writes `packs/<id>/pack.json` in the format described in [`docs/pack-format.md`](docs/pack-format.md).
3. It sets the weights by the rubric.
4. It runs `./fire validate` until the pack has no errors.

`packs/` is git-ignored, so your topics stay yours.

No terminal? Give a chat assistant `docs/pack-format.md`, then save its `pack.json` into `packs/<id>/`. The **пакеты** page shows any errors.

To see the format first, copy the example:

```sh
cp -r examples/packs/softmax packs/
```

## Tools

```
./fire dev | serve           run the app (dev: hot reload on :5190; serve: built app on :17490)
./fire search <words>        find ideas to link to
./fire show <code>           one idea as JSON
./fire new <id>              scaffold a pack
./fire validate [id…]        check packs (exit 1 on errors)
./fire docs [-check]         regenerate docs/ from the code
./fire progress export|import|reset
```

The HTTP API (localhost only) is described in [`docs/api.md`](docs/api.md).

## Layout

| Path | |
|---|---|
| `server/` | Go: pack loader and validator, graph, FSRS-6 + FIRe, placement test, API, CLI |
| `web/` | React 19 + Vite UI: dashboard, program, knowledge graph, packs, how-it-works |
| `examples/packs/` | A small complete pack |
| `docs/` | Generated reference: API, pack format, JSON Schema |
| `AGENTS.md`, `.agents/skills/` | Instructions for agents (`CLAUDE.md` imports `AGENTS.md`; `.claude/skills` is a generated mirror) |

## Credits

The FSRS-6 parameters and the reference implementation come from [open-spaced-repetition/py-fsrs](https://github.com/open-spaced-repetition/py-fsrs) (MIT). The Go port is checked against py-fsrs in tests. The idea of fractional implicit repetition comes from *The Math Academy Way*; no text or material from Math Academy is used. The code is MIT-licensed.
