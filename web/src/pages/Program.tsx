import { useMemo, useState } from 'react';
import { useApi, type Me, type Program, type Section, type Topic } from '../api';
import { Link, useSession } from '../App';
import { Collapse, motion, soft } from '../ui/motion';
import { PackSelect } from '../ui/PackSelect';
import { PinButton } from '../ui/Pin';

// Categories (sections) → topics → lessons. A pack without categories shows
// its topics in one panel.
export function ProgramPage() {
  const { packs } = useSession();
  const [chosen, setChosen] = useState(() => new URLSearchParams(location.search).get('program') ?? '');
  const program = packs.some(p => p.id === chosen) ? chosen : packs[0]?.id ?? '';
  const [q, setQ] = useState('');
  const [toggled, setToggled] = useState<Set<string>>(() => new Set());
  const outline = useApi<Program>(program ? `/program/${program}` : null);
  const me = useApi<Me>(program ? `/me?track=${program}` : null);
  const mem = useMemo(() => new Map(me.data?.ideas.map(i => [i.code, i]) ?? []), [me.data]);
  const now = Date.now();
  const status = (code: string) => { const x = mem.get(code); return x?.due_at && new Date(x.due_at).getTime() <= now ? 'due' : x?.status; };
  const needle = q.trim().toLowerCase();
  const o = outline.data?.program === program ? outline.data : undefined;
  const topicsCount = o?.sections.reduce((n, s) => n + s.topics.length, 0) ?? 0;
  const choose = (p: string) => { setChosen(p); setToggled(new Set()); history.replaceState(null, '', `?program=${p}`); };
  const filter = (t: Topic) => ({ ...t, ideas: t.ideas.filter(c => !needle || o!.ideas[c].name.toLowerCase().includes(needle) || t.name.toLowerCase().includes(needle)) });
  const single = (o?.sections.length ?? 0) <= 1;
  const topicRow = (t: Topic, ti: number) => <motion.div className="tp" key={t.id} initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={{ ...soft, delay: Math.min(ti, 12) * 0.025 }}>
    <div className="tp-name"><b>{t.name}</b>{t.definition && <small className="muted">{t.definition}</small>}<PinButton code={t.id} label="↑ первой" compact learned={t.ideas.every(c => status(c) === 'learned')} /></div>
    <div className="chips">{t.ideas.map(c => <Link key={c} to={`/idea/${c}`} className="chip" data-s={status(c)}><span>{o!.ideas[c].name}</span></Link>)}</div>
  </motion.div>;
  const section = (s: Section, si: number) => {
    const topics = s.topics.map(filter).filter(t => t.ideas.length);
    if (!topics.length) return null;
    const all = topics.flatMap(t => t.ideas);
    const learned = all.filter(c => status(c) === 'learned').length;
    const open = single || !!needle || toggled.has(s.id);
    const toggle = () => { if (!needle) setToggled(x => { const n = new Set(x); if (n.has(s.id)) n.delete(s.id); else n.add(s.id); return n; }); };
    return <motion.section className="sec panel" key={`${program}:${s.id}`} data-open={open || undefined} initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} transition={{ ...soft, delay: Math.min(si, 12) * 0.03 }} layout="position">
      {!single && <button type="button" className="sec-head" aria-expanded={open} onClick={toggle}>
        <motion.span className="chev" aria-hidden="true" animate={{ rotate: open ? 90 : 0 }} transition={soft}>›</motion.span>
        <span className="sec-title"><h3>{s.name}</h3>{s.description && <p>{s.description}</p>}</span>
        <span className="num">{learned}/{all.length} · {topics.length} тем</span>
      </button>}
      <Collapse open={open}>{topics.map(topicRow)}</Collapse>
    </motion.section>;
  };
  return <div className="wrap page">
    <div className="page-head">
      <div><h1>Программа</h1><p className="muted" style={{ margin: '6px 0 0' }}>{o ? `${Object.keys(o.ideas).length} уроков · ${topicsCount} тем${single ? '' : ` · ${o.sections.length} категорий`}.` : packs.length ? '' : 'Пакетов пока нет — см. «как это работает».'}</p></div>
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        {packs.length > 0 && <PackSelect packs={packs} value={program} onChange={choose} />}
        <input type="search" placeholder="поиск…" value={q} onChange={e => setQ(e.target.value)} style={{ width: 200 }} aria-label="Поиск по программе" />
      </div>
    </div>
    <p className="legend" style={{ marginTop: -6, marginBottom: 14 }}><span><i style={{ background: 'var(--good)' }} />выучено</span><span><i style={{ background: 'var(--warn)' }} />пора повторить / начато</span></p>
    {program && !o ? <p className="skel">загрузка программы</p> : o?.sections.map(section)}
  </div>;
}
