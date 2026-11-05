import { useApi, type Edge, type Idea, type Program } from '../api';
import { Link } from '../App';
import { md, mdInline } from '../md';
import { fmt, TRACK_COLOR } from '../ui/charts';

function EdgeList({ edges, names, empty }: { edges: Edge[]; names: Record<string, string>; empty: string }) {
  if (!edges.length) return <p className="muted" style={{ fontSize: 12 }}>{empty}</p>;
  return <ul className="edges">{edges.map(e => <li key={e.code + e.kind}>
    <Link to={`/idea/${e.code}`} title={names[e.code] ?? e.code}>{e.kind === 'implicit' && <span className="tag">мат</span>} {names[e.code] ?? e.code}{e.key && ' *'}</Link>
    <span className={`w${e.kind === 'implicit' ? ' math' : ''}`} title={`encompassing ${e.encompassing}`}><i style={{ width: `${e.encompassing * 100}%` }} /></span>
  </li>)}</ul>;
}

export function IdeaPage({ code }: { code: string }) {
  const idea = useApi<Idea>(`/ideas/${code}`);
  const ml = useApi<Program>('/program/ml'), math = useApi<Program>('/program/math');
  const names: Record<string, string> = Object.fromEntries([...Object.values(ml.data?.ideas ?? {}), ...Object.values(math.data?.ideas ?? {})].map(i => [i.code, i.name]));
  const i = idea.data;
  if (idea.error) return <div className="wrap page"><h1>Нет такой идеи</h1><p className="muted"><code>{code}</code> · <Link to="/program">к программе</Link></p></div>;
  if (!i || i.code !== code) return <div className="wrap page"><h1 className="skel">загрузка идеи</h1></div>;
  const outline = i.program === 'ml' ? ml.data : math.data;
  const section = outline?.sections.find(s => s.topics.some(t => t.id === i.topic_id));
  const topic = section?.topics.find(t => t.id === i.topic_id);
  const pre = i.prerequisites.filter(e => e.kind === 'prerequisite');
  const imp = i.prerequisites.filter(e => e.kind === 'implicit');
  document.title = `${i.name} · ML Lab`;
  return <div className="wrap page">
    <div className="page-head"><div>
      <p className="muted" style={{ margin: '0 0 6px', fontSize: 12 }}><Link to={`/program?program=${i.program}`}>{i.program === 'ml' ? 'ML теория' : 'ML математика'}</Link>{section && ` / ${section.name}`}{topic && ` / ${topic.name}`}</p>
      <h1>{i.name}</h1>
      {i.definition && <p className="muted" style={{ margin: '8px 0 0', maxWidth: '75ch' }}>{i.definition}</p>}
    </div><span className="tag">уровень {i.level} · <code>{i.code}</code></span></div>
    <div className="idea-grid">
      <div className="col">
        <section className="panel"><div className="phead"><h2>Урок</h2></div><div className="prose" dangerouslySetInnerHTML={{ __html: md(i.lesson ?? '') }} /></section>
        <section className="panel"><div className="phead"><h2>Вопросы ({i.questions?.length ?? 0})</h2><span>ответы открыты — это open data</span></div>
          {i.questions?.map((q, n) => <details key={n} style={{ borderTop: n ? '1px dashed var(--line)' : 0, padding: '8px 0' }}>
            <summary className="prose" style={{ cursor: 'pointer' }}><span dangerouslySetInnerHTML={{ __html: mdInline(q.question) }} /></summary>
            <ol type="A" style={{ margin: '8px 0', paddingLeft: 22 }}>{q.options.map((o, k) => <li key={k} style={{ color: k === q.correct ? 'var(--good)' : undefined, fontWeight: k === q.correct ? 600 : undefined }} dangerouslySetInnerHTML={{ __html: mdInline(o) }} />)}</ol>
            {q.explanation && <div className="explain ok prose" dangerouslySetInnerHTML={{ __html: mdInline(q.explanation) }} />}
          </details>)}
        </section>
        {!!i.formulations?.length && <section className="panel"><div className="phead"><h2>Как спрашивают</h2><span>перефразировано</span></div>
          <ul style={{ margin: 0, paddingLeft: 18 }}>{i.formulations.slice(0, 12).map((f, n) => <li key={n}>{f.text} {f.counts?.count ? <small>×{f.counts.count}</small> : null}</li>)}</ul>
        </section>}
      </div>
      <aside className="col">
        {i.program === 'ml' && i.shares && <section className="panel"><div className="phead"><h2>Частота на собесах</h2><span>{i.interviews} собесов</span></div>
          {(['ds', 'ml', 'ai'] as const).map(t => { const s = i.shares![t]; if (!s) return null; return <div key={t} style={{ marginBottom: 8 }}>
            <div className="bar-row" style={{ fontSize: 12 }}><b style={{ color: TRACK_COLOR[t] }}>{t.toUpperCase()}</b><span className="num">{fmt(s.share)}% · 95% ДИ {fmt(s.low)}–{fmt(s.high)}{s.frequent ? ' · часто' : ''}</span></div>
            <div className="meter" style={{ position: 'relative', marginTop: 4 }}><i style={{ width: `${s.share}%`, background: TRACK_COLOR[t] }} /></div>
          </div>; })}
        </section>}
        <section className="panel"><div className="phead"><h2>Опирается на</h2><span>вес = доля повторения</span></div>
          <EdgeList edges={pre} names={names} empty="Стартовая идея — без пререквизитов." />
          {imp.length > 0 && <><div className="phead" style={{ marginTop: 14 }}><h2>Неявно повторяет</h2><span>Codex Sol 6.1</span></div><EdgeList edges={imp} names={names} empty="" /></>}
          <p className="muted" style={{ fontSize: 11, marginBottom: 0 }}>* ключевой пререквизит: провал урока отправляет его на повторение.</p>
        </section>
        <section className="panel"><div className="phead"><h2>Закрывается задачами</h2><span>{i.encompassed_by.length}</span></div>
          <EdgeList edges={i.encompassed_by.slice(0, 30)} names={names} empty="Ни одна задача не повторяет эту идею неявно." />
        </section>
        <section className="panel"><div className="phead"><h2>API</h2></div><pre style={{ margin: 0, fontSize: 11, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>GET /ml/api/v1/ideas/{i.code}</pre></section>
      </aside>
    </div>
  </div>;
}
