import { useApi, type Edge, type Idea, type Program } from '../api';
import { Link, packTitle, useSession } from '../App';
import { useState } from 'react';
import { Collapse, motion, Panel, soft } from '../ui/motion';
import { PinButton } from '../ui/Pin';
import { md, mdInline } from '../md';

function EdgeList({ edges, names, empty }: { edges: Edge[]; names: Record<string, string>; empty: string }) {
  if (!edges.length) return <p className="muted" style={{ fontSize: 12 }}>{empty}</p>;
  return <ul className="edges">{edges.map(e => <li key={e.code + e.kind}>
    <Link to={`/idea/${e.code}`} title={names[e.code] ?? e.code}>{e.kind === 'implicit' && <span className="tag">uses</span>} {names[e.code] ?? e.code}{e.key && ' *'}</Link>
    <span className={`w${e.kind === 'implicit' ? ' math' : ''}`} title={`encompassing ${e.encompassing}`}><i style={{ width: `${e.encompassing * 100}%` }} /></span>
  </li>)}</ul>;
}

export function IdeaPage({ code }: { code: string }) {
  const idea = useApi<Idea>(`/ideas/${code}`);
  const names = useApi<Record<string, string>>('/names').data ?? {};
  const { packs } = useSession();
  const i = idea.data;
  const outline = useApi<Program>(i ? `/program/${i.program}` : null).data;
  if (idea.error) return <div className="wrap page"><h1>Нет такой идеи</h1><p className="muted"><code>{code}</code> · <Link to="/program">к программе</Link></p></div>;
  if (!i || i.code !== code) return <div className="wrap page"><h1 className="skel">загрузка идеи</h1></div>;
  const topic = outline?.sections.flatMap(s => s.topics).find(t => t.id === i.topic_id);
  const pre = i.prerequisites.filter(e => e.kind === 'prerequisite');
  const imp = i.prerequisites.filter(e => e.kind === 'implicit');
  document.title = `${i.name} · FIRe Lab`;
  return <div className="wrap page">
    <div className="page-head"><div>
      <p className="muted" style={{ margin: '0 0 6px', fontSize: 12 }}><Link to={`/program?program=${i.program}`}>{packTitle(packs, i.program)}</Link>{topic && ` / ${topic.name}`}</p>
      <h1>{i.name}</h1>
      {i.definition && <p className="muted" style={{ margin: '8px 0 0', maxWidth: '75ch' }}>{i.definition}</p>}
    </div><div style={{ display: 'grid', gap: 8, justifyItems: 'end' }}><span style={{ display: 'flex', gap: 8, flexWrap: 'wrap', justifyContent: 'flex-end' }}><PinButton code={i.code} label="↑ этот урок первым" /><PinButton code={i.topic_id} label="↑ всю тему первой" compact /></span><span className="tag">уровень {i.level} · <code>{i.code}</code></span></div></div>
    <div className="idea-grid">
      <div className="col">
        <Panel order={1}><div className="phead"><h2>Урок</h2></div><div className="prose" dangerouslySetInnerHTML={{ __html: md(i.lesson ?? '') }} /></Panel>
        <Panel order={2}><div className="phead"><h2>Вопросы ({i.questions?.length ?? 0})</h2><span>с ответами</span></div>
          <Questions items={i.questions ?? []} />
        </Panel>
      </div>
      <aside className="col">
        <Panel order={3}><div className="phead"><h2>Опирается на</h2><Link to={`/graph?idea=${encodeURIComponent(i.code)}`}>граф →</Link></div>
          <EdgeList edges={pre} names={names} empty="Стартовая идея — без пререквизитов." />
          {imp.length > 0 && <><div className="phead" style={{ marginTop: 14 }}><h2>Неявно повторяет</h2><span>uses</span></div><EdgeList edges={imp} names={names} empty="" /></>}
          <p className="muted" style={{ fontSize: 11, marginBottom: 0 }}>* ключевой пререквизит: провал урока отправляет его на повторение.</p>
        </Panel>
        <Panel order={4}><div className="phead"><h2>Закрывается задачами</h2><span>{i.encompassed_by.length}</span></div>
          <EdgeList edges={i.encompassed_by.slice(0, 30)} names={names} empty="Ни одна задача не повторяет эту идею неявно." />
        </Panel>
        <Panel order={5}><div className="phead"><h2>API</h2></div><pre style={{ margin: 0, fontSize: 11, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>GET /api/v1/ideas/{i.code}</pre></Panel>
      </aside>
    </div>
  </div>;
}

function Questions({ items }: { items: NonNullable<Idea['questions']> }) {
  const [open, setOpen] = useState<Set<number>>(() => new Set());
  const flip = (n: number) => setOpen(s => { const x = new Set(s); if (x.has(n)) x.delete(n); else x.add(n); return x; });
  return <>{items.map((q, n) => <div className="qitem" key={n}>
    <button type="button" className="qhead prose" aria-expanded={open.has(n)} onClick={() => flip(n)}>
      <motion.span className="muted" animate={{ rotate: open.has(n) ? 90 : 0 }} transition={soft} aria-hidden="true">›</motion.span>
      <span dangerouslySetInnerHTML={{ __html: mdInline(q.question) }} />
    </button>
    <Collapse open={open.has(n)}><div style={{ padding: '0 0 12px 22px' }}>
      <ol type="A" style={{ margin: '0 0 8px', paddingLeft: 22 }}>{q.options.map((o, k) => <motion.li key={k} initial={{ opacity: 0, x: -6 }} animate={{ opacity: 1, x: 0 }} transition={{ ...soft, delay: 0.03 * k }} style={{ color: k === q.correct ? 'var(--good)' : undefined, fontWeight: k === q.correct ? 600 : undefined }} dangerouslySetInnerHTML={{ __html: mdInline(o) }} />)}</ol>
      {q.explanation && <div className="explain ok prose" dangerouslySetInnerHTML={{ __html: mdInline(q.explanation) }} />}
    </div></Collapse>
  </div>)}</>;
}
