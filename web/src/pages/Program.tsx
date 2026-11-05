import { useMemo, useState } from 'react';
import { useApi, type Me, type Program } from '../api';
import { Link, useSession } from '../App';
import { Collapse, motion, soft } from '../ui/motion';
import { PackSelect } from '../ui/PackSelect';
import { PinButton } from '../ui/Pin';

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
  const topics = o?.sections.flatMap(s => s.topics) ?? [];
  const openByDefault = topics.length <= 6;
  const choose = (p: string) => { setChosen(p); setToggled(new Set()); history.replaceState(null, '', `?program=${p}`); };
  return <div className="wrap page">
    <div className="page-head">
      <div><h1>Программа</h1><p className="muted" style={{ margin: '6px 0 0' }}>{o ? `${Object.keys(o.ideas).length} уроков в ${topics.length} ${topics.length === 1 ? 'теме' : 'темах'}.` : packs.length ? '' : 'Пакетов пока нет — см. «как это работает».'}</p></div>
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        {packs.length > 0 && <PackSelect packs={packs} value={program} onChange={choose} />}
        {o && topics.length > 1 && <button className="btn" style={{ minWidth: 150 }} onClick={() => setToggled(toggled.size ? new Set() : new Set(topics.map(t => t.id)))}>{toggled.size ? 'как было' : openByDefault ? 'свернуть всё' : 'развернуть всё'}</button>}
        <input type="search" placeholder="поиск…" value={q} onChange={e => setQ(e.target.value)} style={{ width: 200 }} aria-label="Поиск по программе" />
      </div>
    </div>
    <p className="legend" style={{ marginTop: -6, marginBottom: 14 }}><span><i style={{ background: 'var(--good)' }} />выучено</span><span><i style={{ background: 'var(--warn)' }} />пора повторить / начато</span></p>
    {program && !o ? <p className="skel">загрузка программы</p> : topics.map((t, ti) => {
      const ideas = t.ideas.filter(c => !needle || o!.ideas[c].name.toLowerCase().includes(needle) || t.name.toLowerCase().includes(needle));
      if (!ideas.length) return null;
      const learned = ideas.filter(c => status(c) === 'learned').length;
      const open = !!needle || (openByDefault !== toggled.has(t.id));
      const toggle = () => { if (!needle) setToggled(s => { const n = new Set(s); if (n.has(t.id)) n.delete(t.id); else n.add(t.id); return n; }); };
      return <motion.section className="sec panel" key={`${program}:${t.id}`} data-open={open || undefined} initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} transition={{ ...soft, delay: Math.min(ti, 12) * 0.03 }} layout="position">
        <div className="sec-row">
          <button type="button" className="sec-head" aria-expanded={open} onClick={toggle}>
            <motion.span className="chev" aria-hidden="true" animate={{ rotate: open ? 90 : 0 }} transition={soft}>›</motion.span>
            <span className="sec-title"><h3>{t.name}</h3>{t.definition && <p>{t.definition}</p>}</span>
            <span className="num">{learned}/{ideas.length}</span>
          </button>
          <PinButton code={t.id} label="↑ первой" compact learned={t.ideas.every(c => status(c) === 'learned')} />
        </div>
        <Collapse open={open}>
          <div className="chips" style={{ padding: '4px 0 2px' }}>{ideas.map(c => <Link key={c} to={`/idea/${c}`} className="chip" data-s={status(c)}><span>{o!.ideas[c].name}</span></Link>)}</div>
        </Collapse>
      </motion.section>;
    })}
  </div>;
}
