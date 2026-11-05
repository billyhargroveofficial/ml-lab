import { useMemo, useState, type CSSProperties } from 'react';
import { useApi, type HistoryItem, type Leader, type Me, type Program, type Task } from '../api';
import { Link, useSession } from '../App';
import { Auth } from '../ui/Auth';
import { Session } from '../ui/Session';
import { fmt, LineChart, Ring } from '../ui/charts';
import { AnimatePresence, Grow, motion, Num, Panel, Seg, soft, spring } from '../ui/motion';
import { lessons, setPin } from '../ui/Pin';

type Prog = 'ml' | 'math';
const DAY = 864e5;
const plural = (n: number, one: string, few: string, many: string) => { const a = n % 10, b = n % 100; return a === 1 && b !== 11 ? one : a >= 2 && a <= 4 && (b < 12 || b > 14) ? few : many; };
const longDate = (d: Date) => d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short', year: 'numeric' }).replace(' г.', '');
const store = { get: (k: string, d: string) => { try { return localStorage.getItem(k) ?? d; } catch { return d; } }, set: (k: string, v: string) => { try { localStorage.setItem(k, v); } catch { /* convenience only */ } } };
const etaDays = (remaining: number, pace: number) => remaining / (pace * Math.pow(pace / 40, 0.1));
const icon: Record<Task['kind'], string> = { lesson: '+', review: '↻', quiz: '?' };
const kindText: Record<Task['kind'], string> = { lesson: 'урок', review: 'повторение', quiz: 'проверка' };

export function Dashboard() {
  const { me: session, guest, loading } = useSession();
  const [program, setProgramState] = useState<Prog>(() => (new URLSearchParams(location.search).get('program') ?? store.get('program', 'ml')) === 'math' ? 'math' : 'ml');
  const [goal, setGoalState] = useState(() => Number(store.get('goal', '40')) || 40);
  const [task, setTask] = useState<{ kind: Task['kind']; code?: string; n: number }>();
  const setProgram = (p: Prog) => { setProgramState(p); store.set('program', p); };
  const setGoal = (g: number) => { setGoalState(g); store.set('goal', String(g)); };
  const track = program === 'math' ? 'math' : 'all';
  const authed = !!session;

  const outline = useApi<Program>(`/program/${program}`);
  const me = useApi<Me>(authed ? `/me?track=${track}` : null);
  const next = useApi<Task[]>(authed ? `/next?track=${track}` : null);
  const history = useApi<HistoryItem[]>(authed ? '/history' : null);
  const [period, setPeriod] = useState<'all' | 'week'>('all');
  const leaders = useApi<Leader[]>(`/leaders?period=${period}`);
  const m = me.data;

  const openNext = () => { const t = next.data?.[0]; setTask(t ? { kind: t.kind, code: t.code, n: Date.now() } : undefined); };
  const failed = outline.error || (authed && (me.error || next.error));

  return <div className="wrap">
    <section className="hero">
      <div>
        <motion.h1 initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={soft}>ML Lab<motion.span animate={{ opacity: [1, 0, 1] }} transition={{ duration: 1.1, repeat: Infinity, ease: 'linear' }}>_</motion.span></motion.h1>
        <div className="stack">
          <motion.p animate={{ opacity: program === 'ml' ? 1 : 0, y: program === 'ml' ? 0 : -4 }} transition={soft} data-on={program === 'ml' || undefined}>427 идей машинного обучения: уроки, повторения и проверки. Интервальное повторение FIRe: решая сложное, повторяешь простое.</motion.p>
          <motion.p animate={{ opacity: program === 'math' ? 1 : 0, y: program === 'math' ? 0 : 4 }} transition={soft} data-on={program === 'math' || undefined}>Математика для ML: анализ, линейная алгебра, вероятность и оптимизация. Каждая ML-идея неявно повторяет математику, на которой стоит.</motion.p>
        </div>
      </div>
      <div className="controls">
        <Seg big label="Программа" value={program} items={[['ml', 'ML теория'], ['math', 'ML математика']]} onChange={setProgram} />
      </div>
    </section>

    {failed && <p className="alert" role="alert">Не удалось загрузить часть данных. <button className="btn small" onClick={() => location.reload()}>Повторить</button></p>}

    <motion.div className="kpis" aria-label="Ваш прогресс" initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={soft}>
      <div><strong className={m || guest ? '' : 'skel'}><Num value={m?.learned ?? 0} />/{m?.total ?? (guest ? 0 : '000')}</strong><span>выучено</span></div>
      <div><strong className={m || guest ? '' : 'skel'}><Num value={m ? m.learned / Math.max(1, m.total) * 100 : 0} format={v => `${fmt(v, 1)}%`} /></strong><span>программы</span></div>
      <div><strong className={m || guest ? '' : 'skel'}><Num value={m?.due ?? 0} /></strong><span>пора повторить</span></div>
      <div><strong className={m || guest ? '' : 'skel'}><Num value={m?.xp ?? 0} /></strong><span>XP</span></div>
    </motion.div>

    <div className="grid">
      <div className="col">
        <Panel order={1} aria-label="Сегодня">
          <div className="phead"><h2>Сегодня</h2>{m && <span>{m.xp_to_quiz > 0 ? `до проверки ${m.xp_to_quiz} XP` : 'проверка доступна'}</span>}</div>
          {guest && !loading ? <div className="intro">
            <ul>
              <li><b>{fmt(427, 0)} ML-идей</b> — у каждой урок и 3–6 вопросов с разбором.</li>
              <li><b>{fmt(1293, 0)} темы математики</b> — от производных до SVD, оптимизации и статистики.</li>
              <li><b>FIRe</b>: составные задачи засчитывают повторение мелких — меньше повторений, тот же результат.</li>
              <li>XP, дневная цель, серия дней и дата, к которой вы пройдёте программу.</li>
            </ul>
            <Auth />
          </div> : <Feed tasks={next.data} loading={!next.data} onOpen={t => setTask({ kind: t.kind, code: t.code, n: Date.now() })} done={!!m && m.learned >= m.total} />}
        </Panel>
        {authed && <Panel order={2} aria-label="История"><div className="phead"><h2>История</h2><span>последние задания</span></div><History items={history.data} /></Panel>}
      </div>

      <aside className="col">
        <Panel order={2} aria-label="Прогноз"><ForecastPanel me={m} guest={guest} goal={goal} onGoal={setGoal} /></Panel>
        <Panel order={3} aria-label="Темы"><Topics outline={outline.data} me={m} /></Panel>
        <Panel order={4} aria-label="Топ учеников">
          <div className="phead"><h2>Топ</h2><Seg label="Период" value={period} items={[['all', 'всё время'], ['week', '7 дней']]} onChange={setPeriod} /></div>
          <Leaders rows={leaders.data} period={period} />
        </Panel>
      </aside>
    </div>
    <AnimatePresence>{task && <Session key={task.n} kind={task.kind} code={task.code} onClose={() => setTask(undefined)} onNext={openNext} />}</AnimatePresence>
  </div>;
}

function Feed({ tasks, loading, onOpen, done }: { tasks?: Task[]; loading: boolean; onOpen: (t: Task) => void; done: boolean }) {
  if (loading && !tasks) return <div className="tasks">{[0, 1, 2].map(i => <div key={i} className="task skel" style={{ height: 50 }} />)}</div>;
  if (!tasks?.length) return <p className="empty">{done ? 'Всё пройдено. Повторения появятся по кривой забывания.' : 'Сейчас нечего делать — загляните позже.'}</p>;
  return <div className="tasks"><AnimatePresence initial={true} mode="popLayout">{tasks.map((t, i) => <motion.button layout key={`${t.kind}:${t.code}`} className={`task${i === 0 ? ' first' : ''}${t.goal ? ' pinned' : ''}`} data-kind={t.kind} onClick={() => onOpen(t)}
    initial={{ opacity: 0, x: -12 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: 24, transition: { duration: 0.15 } }} transition={{ ...spring, delay: i * 0.035 }} whileHover={{ x: 3 }} whileTap={{ scale: 0.985 }}>
    <span className="ico" aria-hidden="true">{icon[t.kind]}</span>
    <span className="t">{t.goal && <span className="goal">{t.goal_kind === 'topic' ? `тема: ${t.goal} · осталось ${t.goal_left} ${lessons(t.goal_left ?? 0)}` : t.goal_code !== t.code ? `цель: ${t.goal} · осталось ${t.goal_left} ${lessons(t.goal_left ?? 0)}` : 'цель'}</span>}<b>{t.title}</b><small>{kindText[t.kind]}{t.topic ? ` · ${t.topic}` : ''}{t.covers?.length ? ` · закроет повторение: ${t.covers.join(', ')}` : ''}</small></span>
    <span className="xp">+{t.xp} XP{t.goal && <span role="button" tabIndex={0} className="unpin" title="Снять цель" aria-label="Снять цель" onClick={e => { e.stopPropagation(); void setPin(''); }} onKeyDown={e => { if (e.key === 'Enter') { e.stopPropagation(); void setPin(''); } }}>×</span>}</span>
  </motion.button>)}</AnimatePresence></div>;
}

function History({ items }: { items?: HistoryItem[] }) {
  if (!items) return <p className="empty skel">загрузка</p>;
  if (!items.length) return <p className="empty">Здесь появятся пройденные уроки и повторения.</p>;
  const groups = new Map<string, HistoryItem[]>();
  for (const h of items.slice(0, 12)) { const d = new Date(h.finished_at).toLocaleDateString('ru-RU', { weekday: 'short', day: 'numeric', month: 'long' }); groups.set(d, [...(groups.get(d) ?? []), h]); }
  return <div className="hist">{[...groups].map(([day, rows]) => <div key={day}>
    <div className="day">{day}</div>
    {rows.map((h, i) => <motion.div className="row" key={h.finished_at + i} initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={{ ...soft, delay: i * 0.02 }}>
      <span className={`dot ${h.passed ? 'ok' : 'no'}`} aria-label={h.passed ? 'засчитано' : 'не засчитано'} />
      <span>{h.code ? <Link to={`/idea/${h.code}`}>{h.title}</Link> : h.title} <small>· {kindText[h.kind]}</small></span>
      <span className="num">{h.correct}/{h.total}</span>
      <span className="num">+{h.xp}</span>
    </motion.div>)}
  </div>)}</div>;
}

function ForecastPanel({ me, guest, goal, onGoal }: { me?: Me; guest: boolean; goal: number; onGoal: (g: number) => void }) {
  const now = Date.now();
  let streak = 0;
  if (me) for (let i = me.days.length - 1 - (me.days.at(-1)?.xp ? 0 : 1); i >= 0 && me.days[i].xp > 0; i--) streak++;
  const end = me ? new Date(now + Math.ceil(etaDays(me.forecast.all.remaining_xp, goal)) * DAY) : undefined;
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
  return <>
    {me && <div className="today">
      <Ring done={me.today_xp} goal={goal} />
      <div><strong>{me.today_xp >= goal ? 'Цель на сегодня выполнена' : `Сегодня ${me.today_xp} из ${goal} XP`}</strong>
        <small>{streak > 0 ? `🔥 ${streak} ${plural(streak, 'день', 'дня', 'дней')} подряд` : 'Серия начнётся с первого задания'}</small></div>
    </div>}
    <div className="phead"><h2>Прогноз</h2>{me && <span>{Math.round(me.learned / Math.max(1, me.total) * 100)}% тем</span>}</div>
    {me && burn.length > 0 && <LineChart label="Накопленный XP и план" series={burn} height={110} xFormat={v => v <= 0 ? `${-Math.round(v)} дн. назад` : `через ${Math.round(v)} дн.`} yFormat={v => `${fmt(v, 0)}`} xTicks={[{ x: 0, label: 'сегодня' }]} />}
    {guest && <p className="muted" style={{ fontSize: 12 }}>Войдите, чтобы увидеть даты при своём темпе.</p>}
    <label className="pace">
      <span>Цель — <b>{goal} XP</b> в день</span>
      <input type="range" min={10} max={150} step={10} value={goal} aria-label="Дневная цель, XP" style={{ '--fill': `${(goal - 10) / 140 * 100}%` } as CSSProperties} onChange={e => onGoal(Number(e.target.value))} />
    </label>
    {end && <motion.dl className="milestones" initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={soft}>
      <div><dt>осталось</dt><dd><Num value={me!.forecast.all.remaining_xp} format={v => `${fmt(Math.round(v), 0)} XP`} /></dd></div>
      <div><dt>вся программа</dt><dd><AnimatePresence mode="wait" initial={false}><motion.span key={longDate(end)} style={{ display: 'inline-block' }} initial={{ opacity: 0, y: -6 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: 6 }} transition={{ duration: 0.15 }}>{longDate(end)}</motion.span></AnimatePresence></dd></div>
    </motion.dl>}
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
    {!outline ? <p className="skel">загрузка</p> : <div className="topics">{rows.map((r, i) => <motion.div className="topic" key={r.id} initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ delay: i * 0.02 }}>
      <span title={r.name}>{r.name}</span><span className="num">{r.learned}/{r.total}</span>
      <span className="meter"><Grow width={(r.learned - r.due) / r.total * 100} /><Grow className="due" width={r.due / r.total * 100} /></span>
    </motion.div>)}</div>}
  </>;
}

function Leaders({ rows, period }: { rows?: Leader[]; period: 'all' | 'week' }) {
  if (!rows) return <p className="skel">загрузка</p>;
  if (!rows.length) return <p className="empty">Пока пусто — станьте первым.</p>;
  return <table className="leaders">
    <thead><tr><th>#</th><th>ник</th><th>XP</th><th>ML</th><th>мат</th></tr></thead>
    <tbody>{rows.map((r, i) => <motion.tr layout key={r.name} className={r.me ? 'me' : undefined} initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ ...spring, delay: i * 0.03 }}>
      <td>{r.rank}</td><td className="n">{r.name}</td><td><Num value={period === 'week' ? r.week_xp : r.xp} /></td><td>{r.learned_ml}</td><td>{r.learned_math}</td>
    </motion.tr>)}</tbody>
  </table>;
}

