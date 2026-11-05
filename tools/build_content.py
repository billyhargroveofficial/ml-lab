#!/usr/bin/env python3
"""Builds the public read-only content database data/content.db.

Inputs (data/source):
  ml.json      — ML ideas: taxonomy, weight, lessons, quizzes, ML→ML prerequisites.
  math.json    — math program from school basics to ML level: lessons, quizzes,
                 math→math prerequisites.
  weights.json — FIRe encompassing weights set by Codex (gpt-6.1-sol):
                 ML idea → math topics and refined ML → ML weights.

Usage: python3 tools/build_content.py [--out data/content.db]
"""
import json, sqlite3, sys, hashlib, os
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SRC = ROOT / 'data' / 'source'
out = Path(sys.argv[sys.argv.index('--out') + 1]) if '--out' in sys.argv else ROOT / 'data' / 'content.db'

ml = json.load(open(SRC / 'ml.json'))
math = json.load(open(SRC / 'math.json'))
weights = json.load(open(SRC / 'weights.json')) if (SRC / 'weights.json').exists() else {'ideas': []}

codes = {i['code'] for i in ml['ideas']} | {i['code'] for i in math['ideas']}
ml_codes = {i['code'] for i in ml['ideas']}

import re
UTM = re.compile(r'[?&]utm_[a-z]+=[^)\s&]*')
def clean(text):
    return UTM.sub('', text) if isinstance(text, str) else text

def clamp(w):
    return round(max(0.05, min(1.0, float(w))), 2)

# Codex weights: refined ML→ML encompassing values and new ML→math edges.
refined, cross = {}, {}
for row in weights['ideas']:
    if row['code'] not in ml_codes:
        continue
    for e in row.get('ml', []):
        refined[(row['code'], e['code'])] = clamp(e['w'])
    for e in row.get('math', []):
        if e['code'] in codes and e['code'] not in ml_codes:
            cross[(row['code'], e['code'])] = max(cross.get((row['code'], e['code']), 0), clamp(e['w']))

if out.exists():
    out.unlink()
db = sqlite3.connect(out)
db.executescript('''
CREATE TABLE meta(key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE sections(id TEXT PRIMARY KEY, program TEXT NOT NULL, ord INTEGER NOT NULL, name TEXT NOT NULL, description TEXT);
CREATE TABLE topics(id TEXT PRIMARY KEY, program TEXT NOT NULL, section_id TEXT NOT NULL REFERENCES sections(id), ord INTEGER NOT NULL, code TEXT, name TEXT NOT NULL, definition TEXT);
CREATE TABLE ideas(code TEXT PRIMARY KEY, id TEXT NOT NULL UNIQUE, program TEXT NOT NULL, topic_id TEXT NOT NULL REFERENCES topics(id), ord INTEGER NOT NULL,
  name TEXT NOT NULL, definition TEXT, level INTEGER NOT NULL, weight REAL NOT NULL, lesson TEXT NOT NULL);
CREATE TABLE questions(code TEXT NOT NULL REFERENCES ideas(code), ord INTEGER NOT NULL, question TEXT NOT NULL, options TEXT NOT NULL, correct INTEGER NOT NULL,
  explanation TEXT, difficulty INTEGER, PRIMARY KEY(code, ord));
CREATE TABLE edges(code TEXT NOT NULL REFERENCES ideas(code), prereq TEXT NOT NULL REFERENCES ideas(code), key INTEGER NOT NULL, encompassing REAL NOT NULL,
  kind TEXT NOT NULL CHECK(kind IN ('prerequisite','implicit')), source TEXT NOT NULL, PRIMARY KEY(code, prereq));
CREATE INDEX edges_prereq ON edges(prereq);
CREATE INDEX ideas_program ON ideas(program, ord);
''')

def add_program(bundle, program):
    for so, s in enumerate(bundle['sections']):
        db.execute('INSERT INTO sections VALUES(?,?,?,?,?)', (s['id'], program, so, s['name'], s.get('description')))
    for to, t in enumerate(bundle['topics']):
        db.execute('INSERT INTO topics VALUES(?,?,?,?,?,?,?)', (t['id'], program, t['section_id'], to, t.get('code'), t['name'], t.get('definition')))
    for io, i in enumerate(bundle['ideas']):
        db.execute('INSERT INTO ideas VALUES(?,?,?,?,?,?,?,?,?,?)', (i['code'], i['id'], program, i['topic_id'], io, i['name'], i.get('definition'),
                   int(i.get('level') or 2), float(i.get('weight') or 0), clean(i.get('lesson') or '')))
        for qo, q in enumerate(i.get('quiz') or []):
            db.execute('INSERT INTO questions VALUES(?,?,?,?,?,?,?)', (i['code'], qo, q['question'], json.dumps(q['options'], ensure_ascii=False), q['correct'], clean(q.get('explanation')), q.get('difficulty')))
        for p in i.get('prerequisites') or []:
            w = refined.get((i['code'], p['code']))
            db.execute('INSERT INTO edges VALUES(?,?,?,?,?,?)', (i['code'], p['code'], int(bool(p.get('key'))), w if w is not None else p['encompassing'], 'prerequisite',
                       'codex-gpt-6.1-sol' if w is not None else 'bundle'))

add_program(ml, 'ml')
add_program(math, 'math')
for (code, prereq), w in sorted(cross.items()):
    db.execute('INSERT OR IGNORE INTO edges VALUES(?,?,?,?,?,?)', (code, prereq, 0, w, 'implicit', 'codex-gpt-6.1-sol'))

meta = {
    'schema': 'ml-lab-content-v1',
    'license': 'CC BY 4.0',
    'sources': {name: hashlib.sha256((SRC / name).read_bytes()).hexdigest() for name in ['ml.json', 'math.json', 'weights.json'] if (SRC / name).exists()},
}
db.execute('INSERT INTO meta VALUES(?,?)', ('info', json.dumps(meta, ensure_ascii=False)))
db.commit()
n = {t: db.execute(f'SELECT count(*) FROM {t}').fetchone()[0] for t in ['sections', 'topics', 'ideas', 'questions', 'edges']}
n['implicit_edges'] = db.execute("SELECT count(*) FROM edges WHERE kind='implicit'").fetchone()[0]
n['refined_edges'] = db.execute("SELECT count(*) FROM edges WHERE kind='prerequisite' AND source!='bundle'").fetchone()[0]
db.execute('VACUUM')
db.close()
print(json.dumps(n))
