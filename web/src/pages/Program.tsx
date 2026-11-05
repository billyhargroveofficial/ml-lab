import { useMemo, useState } from 'react';
import { useApi, type Me, type Program } from '../api';
import { Link, useSession } from '../App';

export function ProgramPage() {
  const [program, setProgram] = useState<'ml' | 'math'>(() => new URLSearchParams(location.search).get('program') === 'math' ? 'math' : 'ml');
  const [q, setQ] = useState('');
  const [opened, setOpened] = useState<Set<string>>(() => new Set());
  const { me: session } = useSession();
  const ml = useApi<Program>('/program/ml'), mathP = useApi<Program>('/program/math');
  const outline = program === 'ml' ? ml : mathP;
  const me = useApi<Me>(session ? `/me?track=${program === 'math' ? 'math' : 'all'}` : null);
  const mem = useMemo(() => new Map(me.data?.ideas.map(i => [i.code, i]) ?? []), [me.data]);
  const now = Date.now();
  const status = (code: string) => { const x = mem.get(code); return x?.due_at && new Date(x.due_at).getTime() <= now ? 'due' : x?.status; };
  const needle = q.trim().toLowerCase();
  const o = outline.data;
  return <div className="wrap page">
    <div className="page-head">
      <div><h1>Программа</h1><div className="stack" style={{ marginTop: 6 }}>
        <p className="muted" style={{ margin: 0 }} data-on={program === 'ml' || undefined}>{ml.data ? Object.keys(ml.data.ideas).length : 427} идей с собеседований, по разделам и темам. Число — на скольких собесах спрашивали.</p>
        <p className="muted" style={{ margin: 0 }} data-on={program === 'math' || undefined}>{mathP.data ? Object.keys(mathP.data.ideas).length : 1642} тем математики, от школьной базы до ML.</p>
      </div></div>
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        <div className="seg big" role="group" aria-label="Программа">{(['ml', 'math'] as const).map(p => <button key={p} aria-pressed={program === p} onClick={() => { setProgram(p); history.replaceState(null, '', `?program=${p}`); }}>{p === 'ml' ? 'ML теория' : 'ML математика'}</button>)}</div>
        {o && <button className="btn" style={{ minWidth: 150 }} onClick={() => setOpened(opened.size ? new Set() : new Set(o.sections.map(x => x.id)))}>{opened.size ? 'свернуть всё' : 'развернуть всё'}</button>}
        <input type="search" placeholder="поиск…" value={q} onChange={e => setQ(e.target.value)} style={{ width: 200 }} aria-label="Поиск по программе" />
      </div>
    </div>
    {session && <p className="legend" style={{ marginTop: -6, marginBottom: 14 }}><span><i style={{ background: 'var(--good)' }} />выучено</span><span><i style={{ background: 'var(--warn)' }} />пора повторить / начато</span></p>}
    {!o ? <p className="skel">загрузка программы</p> : o.sections.map(s => {
      const topics = s.topics.map(t => ({ ...t, ideas: t.ideas.filter(c => !needle || o.ideas[c].name.toLowerCase().includes(needle) || t.name.toLowerCase().includes(needle)) })).filter(t => t.ideas.length);
      if (!topics.length) return null;
      const all = topics.flatMap(t => t.ideas);
      const learned = all.filter(c => status(c) === 'learned').length;
      const open = !!needle || opened.has(s.id);
      return <details className="sec panel" key={s.id} open={open} onToggle={e => { const isOpen = (e.currentTarget as HTMLDetailsElement).open; if (!needle && isOpen !== opened.has(s.id)) setOpened(o => { const n = new Set(o); if (isOpen) n.add(s.id); else n.delete(s.id); return n; }); }}>
        <summary className="sec-head">
          <span className="chev" aria-hidden="true">›</span>
          <span className="sec-title"><h3>{s.name}</h3>{s.description && <p>{s.description}</p>}</span>
          <span className="num">{session ? `${learned}/` : ''}{all.length} · {topics.length} тем</span>
        </summary>
        {topics.map(t => <div className="tp" key={t.id}><b>{t.name}</b><div className="chips">{t.ideas.map(c => <Link key={c} to={`/idea/${c}`} className="chip" data-s={status(c)}>
          <span>{o.ideas[c].name}</span>{program === 'ml' && o.ideas[c].interviews > 0 && <small>{o.ideas[c].interviews}</small>}
        </Link>)}</div></div>)}
      </details>;
    })}
  </div>;
}
