import { useMemo, useState, type CSSProperties } from 'react';
import { useApi, type HistoryItem, type Leader, type Market, type Me, type Program, type Task, type Track } from '../api';
import { Link, useSession } from '../App';
import { Auth } from '../ui/Auth';
import { Session } from '../ui/Session';
import { fmt, LineChart, OfferChart, Ring, TRACK_COLOR } from '../ui/charts';

type Prog = 'ml' | 'math';
const DAY = 864e5;
const plural = (n: number, one: string, few: string, many: string) => { const a = n % 10, b = n % 100; return a === 1 && b !== 11 ? one : a >= 2 && a <= 4 && (b < 12 || b > 14) ? few : many; };
const longDate = (d: Date) => d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short', year: 'numeric' }).replace(' г.', '');
const store = { get: (k: string, d: string) => { try { return localStorage.getItem(k) ?? d; } catch { return d; } }, set: (k: string, v: string) => { try { localStorage.setItem(k, v); } catch { /* convenience only */ } } };
const etaDays = (remaining: number, pace: number) => remaining / (pace * Math.pow(pace / 40, 0.1));
const icon: Record<Task['kind'], string> = { lesson: '+', review: '↻', quiz: '?' };
const kindText: Record<Task['kind'], string> = { lesson: 'урок', review: 'повторение', quiz: 'проверка' };

function Seg<T extends string>({ value, items, onChange, label, big }: { value: T; items: [T, string][]; onChange: (v: T) => void; label: string; big?: boolean }) {
  return <div className={`seg${big ? ' big' : ''}`} role="group" aria-label={label}>{items.map(([v, l]) => <button key={v} type="button" aria-pressed={value === v} onClick={() => onChange(v)}>{l}</button>)}</div>;
}

export function Dashboard() {
  const { me: session, guest, loading } = useSession();
  const [program, setProgramState] = useState<Prog>(() => (new URLSearchParams(location.search).get('program') ?? store.get('program', 'ml')) === 'math' ? 'math' : 'ml');
  const [mlTrack, setMlTrackState] = useState<Exclude<Track, 'math'>>(() => store.get('track', 'all') as Exclude<Track, 'math'>);
  const [goal, setGoalState] = useState(() => Number(store.get('goal', '40')) || 40);
  const [task, setTask] = useState<{ kind: Task['kind']; code?: string; n: number }>();
  const setProgram = (p: Prog) => { setProgramState(p); store.set('program', p); };
  const setMlTrack = (t: Exclude<Track, 'math'>) => { setMlTrackState(t); store.set('track', t); };
  const setGoal = (g: number) => { setGoalState(g); store.set('goal', String(g)); };
  const track: Track = program === 'math' ? 'math' : mlTrack;
  const authed = !!session;

  const market = useApi<Market>('/market');
  const outline = useApi<Program>(`/program/${program}`);
  const me = useApi<Me>(authed ? `/me?track=${track}` : null);
  const next = useApi<Task[]>(authed ? `/next?track=${track}` : null);
  const history = useApi<HistoryItem[]>(authed ? '/history' : null);
  const meDs = useApi<Me>(authed ? '/me?track=ds' : null), meMl = useApi<Me>(authed ? '/me?track=ml' : null), meAi = useApi<Me>(authed ? '/me?track=ai' : null);
  const meAll = useApi<Me>(authed ? '/me?track=all' : null);
  const [period, setPeriod] = useState<'all' | 'week'>('all');
  const leaders = useApi<Leader[]>(`/leaders?period=${period}`);
  const m = me.data;

  const openNext = () => { const t = next.data?.[0]; setTask(t ? { kind: t.kind, code: t.code, n: Date.now() } : undefined); };
  const failed = market.error || outline.error || (authed && (me.error || next.error));

  return <div className="wrap">
    <section className="hero">
      <div>
        <h1>ML Lab<span>_</span></h1>
        <p>{program === 'math'
          ? 'Математика от школьной базы до уровня ML: уроки, повторения и проверки. Каждая ML-идея неявно повторяет математику, на которой стоит.'
          : 'Идеи с собеседований DS/ML/AI в порядке их веса. Интервальное повторение FIRe: решая сложное, повторяешь простое.'}</p>
      </div>
      <div className="controls">
        <Seg big label="Программа" value={program} items={[['ml', 'ML теория'], ['math', 'ML математика']]} onChange={setProgram} />
        {program === 'ml' && <Seg label="Направление" value={mlTrack} items={[['all', 'все'], ['ds', 'DS'], ['ml', 'ML'], ['ai', 'AI']]} onChange={setMlTrack} />}
      </div>
    </section>

    {failed && <p className="alert" role="alert">Не удалось загрузить часть данных. <button className="btn small" onClick={() => location.reload()}>Повторить</button></p>}

    <div className="kpis" aria-label="Ваш прогресс">
      <div><strong className={m || guest ? '' : 'skel'}>{m ? `${m.learned}/${m.total}` : guest ? '0' : '00/000'}</strong><span>выучено</span></div>
      <div><strong className={m || guest ? '' : 'skel'}>{m ? `${fmt(m.coverage * 100, 1)}%` : '0%'}</strong><span>{program === 'math' ? 'программы' : 'покрытие собеса'}</span></div>
      <div><strong className={m || guest ? '' : 'skel'}>{m?.due ?? 0}</strong><span>пора повторить</span></div>
      <div><strong className={m || guest ? '' : 'skel'}>{m?.xp ?? 0}</strong><span>XP</span></div>
    </div>

    <div className="grid">
      <div className="col">
        <section className="panel" aria-label="Сегодня">
          <div className="phead"><h2>Сегодня</h2>{m && <span>{m.xp_to_quiz > 0 ? `до проверки ${m.xp_to_quiz} XP` : 'проверка доступна'}</span>}</div>
          {guest && !loading ? <div className="intro">
            <ul>
              <li><b>{fmt(427, 0)} ML-идей</b> из 167 реальных собесов DS/ML/AI, у каждой — урок и 3–6 вопросов.</li>
              <li><b>{fmt(1642, 0)} тем математики</b> от дробей до SVD, оптимизации и статистики.</li>
              <li><b>FIRe</b>: составные задачи засчитывают повторение мелких — меньше повторений, тот же результат.</li>
              <li>Прогноз офферов по рынку и дата, к которой вы успеете.</li>
            </ul>
            <Auth />
          </div> : <Feed tasks={next.data} loading={!next.data} onOpen={t => setTask({ kind: t.kind, code: t.code, n: Date.now() })} done={!!m && m.learned >= m.total} />}
        </section>
        {authed && <section className="panel" aria-label="История"><div className="phead"><h2>История</h2><span>последние задания</span></div><History items={history.data} /></section>}
        {program === 'ml' && market.data && <section className="panel" aria-label="Офферы">
          <div className="phead"><h2>Офферы vs идеи</h2><span>точка — вы</span></div>
          <OfferChart tracks={market.data.tracks} me={{ ds: meDs.data, ml: meMl.data, ai: meAi.data }} />
        </section>}
      </div>

      <aside className="col">
        <section className="panel" aria-label="Прогноз"><ForecastPanel me={m} guest={guest} goal={goal} onGoal={setGoal} market={program === 'ml' ? market.data : undefined} track={mlTrack} /></section>
        <section className="panel" aria-label="Темы"><Topics outline={outline.data} me={program === 'ml' ? meAll.data : m} /></section>
        <section className="panel" aria-label="Топ учеников">
          <div className="phead"><h2>Топ</h2><Seg label="Период" value={period} items={[['all', 'всё время'], ['week', '7 дней']]} onChange={setPeriod} /></div>
          <Leaders rows={leaders.data} period={period} />
        </section>
      </aside>
    </div>
    {task && <Session key={task.n} kind={task.kind} code={task.code} onClose={() => setTask(undefined)} onNext={openNext} />}
  </div>;
}

function Feed({ tasks, loading, onOpen, done }: { tasks?: Task[]; loading: boolean; onOpen: (t: Task) => void; done: boolean }) {
  if (loading) return <div className="tasks">{[0, 1, 2].map(i => <div key={i} className="task skel" style={{ height: 50 }} />)}</div>;
  if (!tasks?.length) return <p className="empty">{done ? 'Всё пройдено. Повторения появятся по кривой забывания.' : 'Сейчас нечего делать — загляните позже.'}</p>;
  return <div className="tasks">{tasks.map((t, i) => <button key={`${t.kind}:${t.code}`} className={`task${i === 0 ? ' first' : ''}`} data-kind={t.kind} onClick={() => onOpen(t)}>
    <span className="ico" aria-hidden="true">{icon[t.kind]}</span>
    <span className="t"><b>{t.title}</b><small>{kindText[t.kind]}{t.topic ? ` · ${t.topic}` : ''}{t.covers?.length ? ` · закроет повторение: ${t.covers.join(', ')}` : ''}</small></span>
    <span className="xp">+{t.xp} XP</span>
  </button>)}</div>;
}

function History({ items }: { items?: HistoryItem[] }) {
  if (!items) return <p className="empty skel">загрузка</p>;
  if (!items.length) return <p className="empty">Здесь появятся пройденные уроки и повторения.</p>;
  const groups = new Map<string, HistoryItem[]>();
  for (const h of items.slice(0, 12)) { const d = new Date(h.finished_at).toLocaleDateString('ru-RU', { weekday: 'short', day: 'numeric', month: 'long' }); groups.set(d, [...(groups.get(d) ?? []), h]); }
  return <div className="hist">{[...groups].map(([day, rows]) => <div key={day}>
    <div className="day">{day}</div>
    {rows.map((h, i) => <div className="row" key={i}>
      <span className={`dot ${h.passed ? 'ok' : 'no'}`} aria-label={h.passed ? 'засчитано' : 'не засчитано'} />
      <span>{h.code ? <Link to={`/idea/${h.code}`}>{h.title}</Link> : h.title} <small>· {kindText[h.kind]}</small></span>
      <span className="num">{h.correct}/{h.total}</span>
      <span className="num">+{h.xp}</span>
    </div>)}
  </div>)}</div>;
}

function offersAt(market: Market, trackName: string, tier: string, coverage: number) {
  const rows = market.formats.filter(r => r.track === trackName && r.tier === tier);
  return rows.map(r => {
    const pts: [number, number][] = [[0, 0], ...Object.entries(r.offers).map(([k, v]) => [Number(k), v.offers] as [number, number]).sort((a, b) => a[0] - b[0])];
    const x = coverage * 100;
    let value = pts.at(-1)![1];
    for (let i = 1; i < pts.length; i++) if (x <= pts[i][0]) { const [x0, y0] = pts[i - 1], [x1, y1] = pts[i]; value = y0 + (y1 - y0) * (x - x0) / (x1 - x0); break; }
    return { fmt: r.fmt, value };
  }).sort((a, b) => Number(a.fmt.includes('удал')) - Number(b.fmt.includes('удал')));
}

function ForecastPanel({ me, guest, goal, onGoal, market, track }: { me?: Me; guest: boolean; goal: number; onGoal: (g: number) => void; market?: Market; track: Exclude<Track, 'math'> }) {
  const tiers = market ? [...new Set(market.formats.map(r => r.tier))] : [];
  const [tier, setTier] = useState(0);
  const now = Date.now();
  let streak = 0;
  if (me) for (let i = me.days.length - 1 - (me.days.at(-1)?.xp ? 0 : 1); i >= 0 && me.days[i].xp > 0; i--) streak++;
  const end = me ? new Date(now + Math.ceil(etaDays(me.forecast.all.remaining_xp, goal)) * DAY) : undefined;
  const offerEnd = me?.forecast.offer ? new Date(now + Math.ceil(etaDays(me.forecast.offer.remaining_xp, goal)) * DAY) : undefined;
  const burn = useMemo(() => {
    if (!me || !end) return [];
    let acc = me.xp;
    const past: [number, number][] = me.days.slice(-30).reverse().map((d, i) => { const p: [number, number] = [-i, acc]; acc -= d.xp; return p; }).reverse();
    const total = me.xp + me.forecast.all.remaining_xp;
    const span = (end.getTime() - now) / DAY;
    return [
      { key: 'past', label: 'XP', color: 'var(--accent)', points: past, area: true },
      { key: 'plan', label: 'план', color: 'var(--fg)', points: [[0, me.xp], [span, total]] as [number, number][], dashed: true },
    ];
  }, [me, end?.getTime()]);
  const tname = market?.tracks[track].name;
  return <>
    {me && <div className="today">
      <Ring done={me.today_xp} goal={goal} />
      <div><strong>{me.today_xp >= goal ? 'Цель на сегодня выполнена' : `Сегодня ${me.today_xp} из ${goal} XP`}</strong>
        <small>{streak > 0 ? `🔥 ${streak} ${plural(streak, 'день', 'дня', 'дней')} подряд` : 'Серия начнётся с первого задания'}</small></div>
    </div>}
    <div className="phead"><h2>Прогноз</h2>{me && <span>{Math.round(me.learned / Math.max(1, me.total) * 100)}% тем</span>}</div>
    {market && tname && <>
      <div className="bar-row">
        <span className="muted" style={{ fontSize: 12 }}>офферов за {fmt(market.interviews_per_hire, 0)} собесов</span>
        {tiers.length > 1 && <Seg label="Опыт" value={String(tier)} items={tiers.map((t, i) => [String(i), t] as [string, string])} onChange={v => setTier(Number(v))} />}
      </div>
      <div className="offers">{offersAt(market, tname, tiers[tier], me?.coverage ?? 0).map(r => <div key={r.fmt}><span>{r.fmt}</span><strong>{fmt(r.value, 2)}</strong></div>)}</div>
      {me && <p className="muted" style={{ margin: '0 0 8px', fontSize: 12 }}>Шанс хотя бы одного оффера: <b style={{ color: 'var(--fg)' }}>{fmt(me.any_offer * 100, 0)}%</b> при покрытии {fmt(me.coverage * 100, 1)}%.</p>}
    </>}
    {me && burn.length > 0 && <LineChart label="Накопленный XP и план" series={burn} height={84} xFormat={v => v <= 0 ? `${-Math.round(v)} дн. назад` : `через ${Math.round(v)} дн.`} yFormat={v => `${fmt(v, 0)}`} xTicks={[{ x: 0, label: 'сегодня' }]} />}
    {guest && <p className="muted" style={{ fontSize: 12 }}>Войдите, чтобы увидеть даты при своём темпе.</p>}
    <label className="pace">
      <span>Цель — <b>{goal} XP</b> в день</span>
      <input type="range" min={10} max={150} step={10} value={goal} aria-label="Дневная цель, XP" style={{ '--fill': `${(goal - 10) / 140 * 100}%` } as CSSProperties} onChange={e => onGoal(Number(e.target.value))} />
    </label>
    {end && <dl className="milestones">
      {offerEnd && <div><dt>хотя бы 1 оффер</dt><dd>{longDate(offerEnd)}</dd></div>}
      <div><dt>вся программа</dt><dd>{longDate(end)}</dd></div>
    </dl>}
  </>;
}

function Topics({ outline, me }: { outline?: Program; me?: Me }) {
  const mem = useMemo(() => new Map(me?.ideas.map(i => [i.code, i]) ?? []), [me]);
  const now = Date.now();
  const rows = outline?.sections.map(s => {
    const codes = s.topics.flatMap(t => t.ideas);
    const learned = codes.filter(c => mem.get(c)?.status === 'learned').length;
    const due = codes.filter(c => { const x = mem.get(c); return x?.due_at && new Date(x.due_at).getTime() <= now; }).length;
    return { id: s.id, name: s.name, total: codes.length, learned, due };
  }) ?? [];
  return <>
    <div className="phead"><h2>Разделы</h2><Link to={`/program?program=${outline?.program ?? 'ml'}`} className="muted">карта →</Link></div>
    {!outline ? <p className="skel">загрузка</p> : <div className="topics">{rows.map(r => <div className="topic" key={r.id}>
      <span title={r.name}>{r.name}</span><span className="num">{r.learned}/{r.total}</span>
      <span className="meter"><i style={{ width: `${(r.learned - r.due) / r.total * 100}%` }} /><i className="due" style={{ width: `${r.due / r.total * 100}%` }} /></span>
    </div>)}</div>}
  </>;
}

function Leaders({ rows, period }: { rows?: Leader[]; period: 'all' | 'week' }) {
  if (!rows) return <p className="skel">загрузка</p>;
  if (!rows.length) return <p className="empty">Пока пусто — станьте первым.</p>;
  return <table className="leaders">
    <thead><tr><th>#</th><th>ник</th><th>XP</th><th>ML</th><th>мат</th><th>офферы</th></tr></thead>
    <tbody>{rows.map(r => <tr key={r.rank} className={r.me ? 'me' : undefined}>
      <td>{r.rank}</td><td className="n">{r.name}</td><td>{period === 'week' ? r.week_xp : r.xp}</td><td>{r.learned_ml}</td><td>{r.learned_math}</td><td>{fmt(r.offers, 2)}</td>
    </tr>)}</tbody>
  </table>;
}

