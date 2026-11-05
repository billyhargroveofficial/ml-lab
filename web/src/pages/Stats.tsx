import { useApi, type Stats } from '../api';
import { Link } from '../App';
import { Bars, fmt, LineChart } from '../ui/charts';

// Only aggregates: counts, buckets and pass rates with at least min_sample attempts.
export function StatsPage() {
  const { data: s, error } = useApi<Stats>('/stats');
  if (error) return <div className="wrap page"><p className="alert">Статистика недоступна: {error.message}</p></div>;
  const days = s?.daily ?? [];
  const ticks = days.map((d, i) => ({ x: i, label: d.date.slice(5).split('-').reverse().join('.') })).filter((_, i) => i % 10 === 0);
  return <div className="wrap page">
    <div className="page-head"><div><h1>Статистика</h1><p className="muted" style={{ margin: '6px 0 0', maxWidth: '72ch' }}>Обезличенные агрегаты по всем ученикам: только суммы, корзины и доли. Ни одной строки про конкретного человека; доли считаются от {s?.min_sample ?? 5} попыток. Те же данные — в <Link to="/docs#stats">открытом API</Link>.</p></div></div>
    <div className="kpis">
      <div><strong className={s ? '' : 'skel'}>{s?.users ?? 0}</strong><span>учеников</span></div>
      <div><strong className={s ? '' : 'skel'}>{s?.active_7d ?? 0}</strong><span>активны за 7 дней</span></div>
      <div><strong className={s ? '' : 'skel'}>{s ? fmt(s.lessons_passed, 0) : 0}</strong><span>уроков пройдено</span></div>
      <div><strong className={s ? '' : 'skel'}>{s ? fmt(s.xp, 0) : 0}</strong><span>XP заработано</span></div>
    </div>
    <div className="stats-grid">
      <section className="panel"><div className="phead"><h2>Активность</h2><span>60 дней</span></div>
        {s && <LineChart label="Ученики и задания по дням" height={150} xTicks={ticks} xFormat={v => days[Math.round(v)]?.date ?? ''} yFormat={v => fmt(v, 0)}
          series={[{ key: 'a', label: 'заданий', color: 'var(--accent)', area: true, points: days.map((d, i) => [i, d.attempts]) }, { key: 'l', label: 'учеников', color: 'var(--fg)', points: days.map((d, i) => [i, d.learners]) }]} />}
      </section>
      <section className="panel"><div className="phead"><h2>FIRe в цифрах</h2></div>
        {s && <>
          <p style={{ marginTop: 0 }}>Явных повторений: <b>{fmt(s.implicit.explicit_reviews, 0)}</b>. Неявных зачётов: <b>{fmt(s.implicit.implicit_credits, 0)}</b> — столько раз решённая составная задача засчитала повторение более простых тем.</p>
          <Bars rows={[{ label: 'явные', value: s.implicit.explicit_reviews }, { label: 'неявные', value: s.implicit.implicit_credits }]} format={v => fmt(v, 0)} />
        </>}
      </section>
      <section className="panel"><div className="phead"><h2>Сколько выучено</h2><span>учеников по числу идей</span></div>{s && <Bars rows={s.learned_histogram.map(b => ({ label: b.label, value: b.count }))} />}</section>
      <section className="panel"><div className="phead"><h2>Покрытие собеса</h2><span>учеников по покрытию</span></div>{s && <Bars rows={s.coverage_histogram.map(b => ({ label: b.label, value: b.count }))} />}</section>
      <section className="panel"><div className="phead"><h2>Разделы</h2><span>урок с первого раза</span></div>
        {s && (s.sections.length ? <Bars rows={s.sections.map(x => ({ label: x.name, value: Math.round(x.pass_rate * 100) }))} format={v => `${v}%`} /> : <p className="empty">Пока мало попыток.</p>)}
      </section>
      <section className="panel"><div className="phead"><h2>Самые трудные идеи</h2><span>урок с первого раза</span></div>
        {s && (s.hardest_ideas.length ? <ul className="edges">{s.hardest_ideas.map(h => <li key={h.code}><Link to={`/idea/${h.code}`}>{h.name}</Link><span className="num">{Math.round(h.pass_rate * 100)}%</span></li>)}</ul> : <p className="empty">Пока мало попыток.</p>)}
      </section>
    </div>
  </div>;
}
