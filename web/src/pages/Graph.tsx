import { useMemo, useState, type CSSProperties } from 'react';
import { useApi, type Edge, type PackInfo } from '../api';
import { go, Link, packTitle, useSession } from '../App';
import { fmt } from '../ui/charts';
import { AnimatePresence, motion, Num, Panel, soft } from '../ui/motion';
import { PackSelect } from '../ui/PackSelect';
import { PinButton } from '../ui/Pin';
import { GraphCanvas } from '../ui/GraphCanvas';

type Row = Edge & { from: string };
interface Node { code: string; name: string; program: string; section: string; sectionName: string; alt: boolean }
interface Graph { nodes: Map<string, Node>; out: Map<string, Row[]>; inc: Map<string, Row[]>; sections: { id: string; name: string; alt: boolean }[] }
interface GraphNode { code: string; name: string; pack: string; topic: string; topic_name: string }
const CUTOFF = 0.1;

// view: "all" or one pack. Nodes of every other pack are drawn in the second
// colour; the heatmap groups by topic, or by pack when topics are too many.
function build(list: GraphNode[], edges: Row[], view: string, packs: PackInfo[]): Graph {
  const nodes = new Map<string, Node>();
  const order = packs.map(p => p.id);
  const altOf = (pack: string) => view === 'all' ? order.indexOf(pack) % 2 === 1 : pack !== view;
  // A pack's view also shows what its ideas link to in other packs.
  const linked = new Set<string>();
  if (view !== 'all') for (const e of edges) if (e.from.startsWith(view + '.')) linked.add(e.code);
  const shown = list.filter(n => view === 'all' || n.pack === view || linked.has(n.code));
  const topics = new Set(shown.map(n => n.topic));
  const byPack = topics.size > 30;
  const sections = new Map<string, Graph['sections'][number]>();
  for (const n of shown) {
    const id = byPack ? n.pack : n.topic;
    const name = byPack ? packTitle(packs, n.pack) : n.topic_name;
    if (!sections.has(id)) sections.set(id, { id, name, alt: altOf(n.pack) });
    nodes.set(n.code, { code: n.code, name: n.name, program: n.pack, section: id, sectionName: name, alt: altOf(n.pack) });
  }
  const out = new Map<string, Row[]>(), inc = new Map<string, Row[]>();
  for (const e of edges) {
    if (!nodes.has(e.from) || !nodes.has(e.code)) continue;
    (out.get(e.from) ?? out.set(e.from, []).get(e.from)!).push(e);
    (inc.get(e.code) ?? inc.set(e.code, []).get(e.code)!).push(e);
  }
  return { nodes, out, inc, sections: [...sections.values()] };
}

// The same walk as the server's FIREPropagate: every node gets the weight of
// its best path (product of encompassing weights), paths below 0.1 stop.
function propagate(g: Graph, start: string, dir: 'down' | 'up') {
  const w = new Map<string, number>([[start, 1]]);
  const depth = new Map<string, number>([[start, 0]]);
  const via = new Map<string, string>();
  const queue = [start];
  while (queue.length) {
    const cur = queue.shift()!;
    const edges = dir === 'down' ? g.out.get(cur) ?? [] : g.inc.get(cur) ?? [];
    for (const e of edges) {
      const next = dir === 'down' ? e.code : e.from;
      const x = w.get(cur)! * e.encompassing;
      if (x < CUTOFF || x <= (w.get(next) ?? 0)) continue;
      w.set(next, x); depth.set(next, depth.get(cur)! + 1); via.set(next, cur);
      queue.push(next);
    }
  }
  w.delete(start);
  return { w, depth, via };
}

export function GraphPage() {
  const { packs } = useSession();
  const list = useApi<GraphNode[]>('/graph');
  const edges = useApi<Row[]>('/edges');
  const params = new URLSearchParams(location.search);
  const [chosen, setView] = useState(() => params.get('view') ?? 'all');
  const view = chosen === 'all' || packs.some(p => p.id === chosen) ? chosen : 'all';
  const g = useMemo(() => list.data && edges.data ? build(list.data, edges.data, view, packs) : undefined, [list.data, edges.data, view, packs]);
  // Default idea: the one with the most links in the current view.
  const busiest = useMemo(() => g ? [...g.nodes.values()].filter(n => view === 'all' || n.program === view).sort((a, b) => ((g.out.get(b.code)?.length ?? 0) + (g.inc.get(b.code)?.length ?? 0)) - ((g.out.get(a.code)?.length ?? 0) + (g.inc.get(a.code)?.length ?? 0)))[0]?.code : undefined, [g, view]);
  const [picked, setCode] = useState(() => params.get('idea') ?? '');
  const code = g && g.nodes.has(picked) ? picked : busiest ?? picked;
  const url = (c: string, v: string) => history.replaceState(null, '', `?view=${v}&idea=${encodeURIComponent(c)}`);
  const [focus, setFocus] = useState(0);
  const select = (c: string) => { setCode(c); url(c, view); };
  const jump = (c: string) => { select(c); setFocus(f => f + 1); };
  const down = useMemo(() => g ? propagate(g, code, 'down') : { w: new Map(), via: new Map(), depth: new Map() }, [g, code]);
  const up = useMemo(() => g ? propagate(g, code, 'up') : { w: new Map(), via: new Map(), depth: new Map() }, [g, code]);
  const changeView = (v: string) => { setView(v); url(code, v); };
  const [memory, setMemory] = useState(0.3);

  const stats = useMemo(() => {
    if (!g) return undefined;
    const all = [...g.out.values()].flat();
    const by = (k: string) => all.filter(e => e.kind === k);
    const hist = (rows: Row[]) => Array.from({ length: 10 }, (_, i) => rows.filter(e => Math.min(9, Math.floor(e.encompassing * 10 - 1e-9)) === i).length);
    return { pre: by('prerequisite').length, imp: by('implicit').length, hPre: hist(by('prerequisite')), hImp: hist(by('implicit')) };
  }, [g]);

  return <div className="wrap page">
    <div className="page-head"><div>
      <h1>Граф знаний</h1>
      <p className="muted" style={{ margin: '6px 0 0', maxWidth: '78ch' }}>Каждое ребро «сложная → простая» несёт вес <code>encompassing</code>: какую долю повторения простой темы засчитывает успешное решение сложной. Сплошные рёбра — <code>requires</code> (блокируют урок), пунктир — <code>uses</code> (только раздают повторение). Веса ставит автор пакета — обычно агент — по рубрике из <code>docs/pack-format.md</code>.</p>
    </div>{packs.length > 0 && <PackSelect all packs={packs} value={view} onChange={changeView} />}</div>
    {!packs.length && list.data ? <p className="muted">Пакетов пока нет — граф появится, когда вы добавите первый.</p> : !g ? <p className="skel">загрузка графа</p> : <>
      <Picker g={g} code={code} onPick={jump} />
      {g.nodes.has(code) ? <>
        <Panel order={1} style={{ marginBottom: 16 }}>
          <div className="phead"><h2>{g.nodes.get(code)!.name}</h2><span style={{ display: 'inline-flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}><PinButton code={code} /><Link to={`/idea/${code}`}>карточка идеи →</Link></span></div>
          <GraphCanvas g={g} code={code} down={down} up={up} focus={focus} onPick={select} />
          <div className="legend"><span><i style={{ background: 'var(--accent)' }} />на что опирается (получает зачёт)</span><span><i style={{ background: 'var(--ds)' }} />что на ней построено</span><span><i style={{ background: 'var(--ml)' }} />uses</span><span>число на ребре — его вес, «путь» — произведение весов до выбранной; колесо — масштаб, перетаскивание — сдвиг, клик — выбрать</span></div>
        </Panel>
        <div className="stats-grid" style={{ marginBottom: 16 }}>
          <Credit g={g} code={code} memory={memory} setMemory={setMemory} onPick={select} />
          <Covers g={g} code={code} onPick={select} />
        </div>
      </> : <p className="alert">Нет идеи с кодом <code>{code}</code>.</p>}
      <Heatmap g={g} />
      {stats && <Panel order={5} style={{ marginTop: 16 }}>
        <div className="phead"><h2>Распределение весов</h2><span>{fmt(stats.pre, 0)} requires · {fmt(stats.imp, 0)} uses</span></div>
        <WeightHist a={stats.hPre} b={stats.hImp} />
      </Panel>}
    </>}
  </div>;
}

function Picker({ g, code, onPick }: { g: Graph; code: string; onPick: (c: string) => void }) {
  const [q, setQ] = useState('');
  const hits = useMemo(() => {
    const n = q.trim().toLowerCase();
    if (n.length < 2) return [];
    return [...g.nodes.values()].filter(x => x.name.toLowerCase().includes(n) || x.code.includes(n)).slice(0, 12);
  }, [q, g]);
  const rich = useMemo(() => [...g.nodes.values()].map(n => ({ n, k: (g.out.get(n.code)?.length ?? 0) + (g.inc.get(n.code)?.length ?? 0) })).sort((a, b) => b.k - a.k).slice(0, 8), [g]);
  return <div style={{ display: 'grid', gap: 8, marginBottom: 16, position: 'relative' }}>
    <input type="search" placeholder="найти идею или тему…" value={q} onChange={e => setQ(e.target.value)} aria-label="Поиск идеи" />
    {hits.length > 0 && <div className="panel" style={{ position: 'absolute', top: 38, left: 0, right: 0, zIndex: 5, padding: 6, animation: 'none' }}>
      {hits.map(h => <button key={h.code} className="btn ghost" style={{ width: '100%', justifyContent: 'space-between', height: 30 }} onClick={() => { onPick(h.code); setQ(''); }}>
        <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{h.name}</span><small>{h.sectionName}</small>
      </button>)}
    </div>}
    <div className="chips">{rich.map(({ n }) => <button key={n.code} className="chip" data-s={n.code === code ? 'learned' : undefined} onClick={() => onPick(n.code)} style={{ cursor: 'pointer' }}><span>{n.name}</span></button>)}</div>
  </div>;
}


// Columns by depth: prerequisites to the left (up to 2 steps), dependents to
// the right (up to 2). Each column keeps its strongest nodes.
function Credit({ g, code, memory, setMemory, onPick }: { g: Graph; code: string; memory: number; setMemory: (m: number) => void; onPick: (c: string) => void }) {
  const rows = useMemo(() => [...propagate(g, code, 'down').w].sort((a, b) => b[1] - a[1]), [g, code]);
  const total = rows.reduce((s, [, w]) => s + w * (1 - memory), 0);
  return <Panel order={2}>
    <div className="phead"><h2>Неявное повторение</h2><span>{rows.length} тем</span></div>
    <p className="muted" style={{ marginTop: 0, fontSize: 12 }}>Если решить эту задачу, каждая тема ниже получит долю повторения <b style={{ color: 'var(--fg)' }}>вес пути × (1 − память)</b>. Сумма — <b style={{ color: 'var(--fg)' }}><Num value={total} format={v => fmt(v, 1)} /></b> «повторений даром».</p>
    <label className="pace" style={{ marginTop: 0, marginBottom: 10 }}><span>Память простых тем — <b>{Math.round(memory * 100)}%</b></span>
      <input type="range" min={0} max={0.9} step={0.1} value={memory} style={{ '--fill': `${memory / 0.9 * 100}%` } as CSSProperties} onChange={e => setMemory(Number(e.target.value))} aria-label="Память простых тем" /></label>
    {rows.length ? <ul className="edges" style={{ maxHeight: 320, overflow: 'auto' }}>{rows.slice(0, 60).map(([c, w]) => <li key={c} style={{ gridTemplateColumns: 'minmax(0,1fr) 70px 40px' }}>
      <a href={`?idea=${encodeURIComponent(c)}`} onClick={e => { e.preventDefault(); onPick(c); }}>{g.nodes.get(c)!.alt && <span className="tag">{g.nodes.get(c)!.program}</span>} {g.nodes.get(c)!.name}</a>
      <span className={`w${g.nodes.get(c)!.alt ? ' math' : ''}`}><motion.i animate={{ width: `${w * (1 - memory) * 100}%` }} initial={{ width: 0 }} transition={soft} /></span>
      <span className="num">+{(w * (1 - memory)).toFixed(2)}</span>
    </li>)}</ul> : <p className="empty">Эта тема ни на что не опирается — она лист графа.</p>}
  </Panel>;
}

function Covers({ g, code, onPick }: { g: Graph; code: string; onPick: (c: string) => void }) {
  const rows = useMemo(() => [...propagate(g, code, 'up').w].sort((a, b) => b[1] - a[1]), [g, code]);
  const full = rows.filter(([, w]) => w >= 0.5).length;
  return <Panel order={3}>
    <div className="phead"><h2>Кто её закрывает</h2><span>{rows.length} задач</span></div>
    <p className="muted" style={{ marginTop: 0, fontSize: 12 }}>Решая любую из этих задач, ученик неявно повторяет выбранную тему. С весом ≥ 0,5 — <b style={{ color: 'var(--fg)' }}>{full}</b>: такой урок в ленте заменяет просроченное повторение этой темы.</p>
    {rows.length ? <ul className="edges" style={{ maxHeight: 360, overflow: 'auto' }}>{rows.slice(0, 60).map(([c, w]) => <li key={c} style={{ gridTemplateColumns: 'minmax(0,1fr) 70px 40px' }}>
      <a href={`?idea=${encodeURIComponent(c)}`} onClick={e => { e.preventDefault(); onPick(c); }}>{g.nodes.get(c)!.alt && <span className="tag">{g.nodes.get(c)!.program}</span>} {g.nodes.get(c)!.name}</a>
      <span className="w"><motion.i initial={{ width: 0 }} animate={{ width: `${w * 100}%` }} transition={soft} /></span><span className="num">{w.toFixed(2)}</span>
    </li>)}</ul> : <p className="empty">Ни одна задача на неё не опирается — это вершина графа.</p>}
  </Panel>;
}

// Section × section: summed encompassing weight of every edge between them.
function Heatmap({ g }: { g: Graph }) {
  const [cell, setCell] = useState<{ a: string; b: string } | null>(null);
  const { m, max, rowsIdx, colsIdx } = useMemo(() => {
    const m = new Map<string, { sum: number; n: number; top: Row[] }>();
    for (const list of g.out.values()) for (const e of list) {
      const k = `${g.nodes.get(e.from)!.section}|${g.nodes.get(e.code)!.section}`;
      const v = m.get(k) ?? { sum: 0, n: 0, top: [] };
      v.sum += e.encompassing; v.n++; v.top.push(e); m.set(k, v);
    }
    let max = 0; for (const v of m.values()) max = Math.max(max, v.sum);
    const used = (id: string, side: 0 | 1) => [...m.keys()].some(k => k.split('|')[side] === id);
    return { m, max, rowsIdx: g.sections.filter(s => used(s.id, 0)), colsIdx: g.sections.filter(s => used(s.id, 1)) };
  }, [g]);
  const sel = cell ? m.get(`${cell.a}|${cell.b}`) : undefined;
  const short = (s: string) => s.length > 22 ? s.slice(0, 21) + '…' : s;
  return <Panel order={4}>
    <div className="phead"><h2>Темы → темы</h2><span>сумма весов рёбер; строка опирается на столбец</span></div>
    <div style={{ overflowX: 'auto', margin: '0 -16px', padding: '0 16px' }}>
      <table style={{ borderCollapse: 'separate', borderSpacing: 2, fontSize: 10 }}>
        <thead><tr><th />{colsIdx.map(c => <th key={c.id} title={c.name} style={{ writingMode: 'vertical-rl', transform: 'rotate(180deg)', fontWeight: 400, color: c.alt ? 'var(--ds)' : 'var(--muted)', height: 150, textAlign: 'left', whiteSpace: 'nowrap' }}>{short(c.name)}</th>)}</tr></thead>
        <tbody>{rowsIdx.map(r => <tr key={r.id}><th title={r.name} style={{ textAlign: 'right', fontWeight: 400, whiteSpace: 'nowrap', paddingRight: 6, color: r.alt ? 'var(--ds)' : 'var(--fg)' }}>{short(r.name)}</th>
          {colsIdx.map(c => { const v = m.get(`${r.id}|${c.id}`); const t = v ? Math.sqrt(v.sum / max) : 0; const on = cell?.a === r.id && cell?.b === c.id; return <motion.td key={c.id} initial={{ opacity: 0, scale: 0.6 }} whileInView={{ opacity: 1, scale: 1 }} viewport={{ once: true }} transition={{ ...soft, delay: (rowsIdx.indexOf(r) + colsIdx.indexOf(c)) * 0.012 }} whileHover={v ? { scale: 1.2 } : undefined} onClick={() => v && setCell({ a: r.id, b: c.id })} title={v ? `${r.name} → ${c.name}: ${v.n} рёбер, Σ ${fmt(v.sum)}` : ''}
            style={{ width: 22, minWidth: 22, height: 22, borderRadius: 3, cursor: v ? 'pointer' : 'default', background: v ? `color-mix(in srgb, ${c.alt !== r.alt ? 'var(--ml)' : 'var(--accent)'} ${Math.round(12 + t * 88)}%, var(--bg))` : 'var(--sunk)', outline: on ? '2px solid var(--fg)' : undefined }} />; })}
        </tr>)}</tbody>
      </table>
    </div>
    {sel && cell && <div style={{ marginTop: 12 }}>
      <p style={{ margin: '0 0 6px' }}><b>{g.sections.find(s => s.id === cell.a)!.name}</b> → <b>{g.sections.find(s => s.id === cell.b)!.name}</b>: {sel.n} рёбер, средний вес {fmt(sel.sum / sel.n, 2)}</p>
      <ul className="edges">{sel.top.sort((a, b) => b.encompassing - a.encompassing).slice(0, 12).map((e, i) => <li key={i} style={{ gridTemplateColumns: 'minmax(0,1fr) 44px' }}>
        <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}><a href={`/graph?idea=${encodeURIComponent(e.from)}`} onClick={ev => { ev.preventDefault(); go(`/graph?idea=${encodeURIComponent(e.from)}`); }}>{g.nodes.get(e.from)!.name}</a> → {g.nodes.get(e.code)!.name}</span>
        <span className="num">{e.encompassing.toFixed(2)}</span>
      </li>)}</ul>
    </div>}
  </Panel>;
}

function WeightHist({ a, b }: { a: number[]; b: number[] }) {
  const max = Math.sqrt(Math.max(1, ...a, ...b));
  const h = (v: number) => `${Math.sqrt(v) / max * 100}%`;
  return <div>
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(10, 1fr)', gap: 6, alignItems: 'end', height: 110 }}>
      {a.map((v, i) => <div key={i} style={{ display: 'flex', gap: 2, alignItems: 'end', height: '100%' }} title={`${(i / 10).toFixed(1)}–${((i + 1) / 10).toFixed(1)}: пререквизитов ${v}, неявных ${b[i]}`}>
        <motion.i initial={{ height: 0 }} animate={{ height: h(v) }} transition={{ ...soft, delay: i * 0.03 }} style={{ flex: 1, background: 'var(--accent)', borderRadius: '2px 2px 0 0', minHeight: v ? 2 : 0 }} />
        <motion.i initial={{ height: 0 }} animate={{ height: h(b[i]) }} transition={{ ...soft, delay: i * 0.03 + 0.05 }} style={{ flex: 1, background: 'var(--ml)', borderRadius: '2px 2px 0 0', minHeight: b[i] ? 2 : 0 }} />
      </div>)}
    </div>
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(10, 1fr)', gap: 6, fontSize: 10, color: 'var(--faint)', marginTop: 4 }}>{a.map((v, i) => <span key={i}>{(i / 10).toFixed(1)}–{((i + 1) / 10).toFixed(1)}<br /><b style={{ color: 'var(--accent)', fontWeight: 500 }}>{v}</b> · <b style={{ color: 'var(--ml)', fontWeight: 500 }}>{b[i]}</b></span>)}</div>
    <div className="legend"><span><i style={{ background: 'var(--accent)' }} />пререквизиты</span><span><i style={{ background: 'var(--ml)' }} />неявные ML → мат</span><span>ось x — вес encompassing, высота — корень из числа рёбер; наведите на столбик</span></div>
  </div>;
}
