# ML Lab

Open interview-prep trainer for DS / ML / AI: **427 ML ideas** taken from 167 real
interviews and **1642 math topics** from school arithmetic to SVD, convex
optimisation and statistics. Every idea has a lesson and 3–6 multiple-choice
questions. Scheduling is FIRe (Fractional Implicit Repetition, from *The Math
Academy Way*): solving a composite task counts as a partial review of the simpler
topics it contains, weighted by `encompassing` edges.

Live: https://billyhargrove.ru/ml · API docs: https://billyhargrove.ru/ml/docs

## What is open

- `data/source/*.json` — the program bundles, the market index and the Codex weights;
- `data/content.db` — the same as one read-only SQLite file (built by `tools/build_content.py`,
  downloadable at `/ml/data/content.db`);
- a keyless JSON API at `/ml/api/v1` (program, ideas with answers, graph, market,
  leaderboard, anonymized stats).

Code is MIT, content is CC BY 4.0 (`data/LICENSE.md`).

## Layout

| path | what |
|---|---|
| `server/` | Go: content loader, FIRe (`fire.go`), attempts and progress (`lab.go`), leaderboard and stats (`social.go`), HTTP (`http.go`) |
| `web/` | React + Vite, monospace UI, light and dark themes |
| `tools/build_content.py` | `data/source` → `data/content.db` |
| `tools/codex-weights/` | how the ML → math FIRe weights were produced |

## Run locally

```sh
python3 tools/build_content.py
(cd server && go build -o ../bin/ml-lab .)
./bin/ml-lab -db data/users.db &          # API on 127.0.0.1:17490/ml
(cd web && npm ci && npm run dev)         # http://127.0.0.1:5190/ml/ (proxies /ml/api)
```

Production: `(cd web && npm run build)` and run the binary with `-web web/dist -secure`
behind a reverse proxy that forwards `/ml*` to it.

