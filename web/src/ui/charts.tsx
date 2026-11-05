import { useRef, useState } from 'react';
import { motion } from 'framer-motion';

export const fmt = (v: number, digits = 1) => new Intl.NumberFormat('ru-RU', { maximumFractionDigits: digits }).format(v);

export interface Series { key: string; label: string; color: string; points: [number, number][]; marker?: [number, number]; dashed?: boolean; area?: boolean }

// One SVG line chart for every graph on the site: hover shows the nearest x.
export function LineChart({ series, height = 140, xTicks = [], yFormat = v => fmt(v), xFormat = v => fmt(v, 0), yMax, label }: {
  series: Series[]; height?: number; xTicks?: { x: number; label: string }[]; yFormat?: (v: number) => string; xFormat?: (v: number) => string; yMax?: number; label: string;
}) {
  const W = 600, H = height, L = 4, R = 4, T = 8, B = 18;
  const ref = useRef<HTMLDivElement>(null);
  const [hover, setHover] = useState<number | null>(null);
  const all = series.flatMap(s => s.points);
  const xMin = Math.min(...all.map(p => p[0]), 0), xMax = Math.max(...all.map(p => p[0]), 1);
  const top = yMax ?? Math.max(...all.map(p => p[1]), 0.0001) * 1.08;
  const x = (v: number) => L + (v - xMin) / (xMax - xMin || 1) * (W - L - R);
  const y = (v: number) => T + (1 - Math.min(1, v / top)) * (H - T - B);
  const path = (pts: [number, number][]) => pts.map((p, i) => `${i ? 'L' : 'M'}${x(p[0]).toFixed(1)},${y(p[1]).toFixed(1)}`).join('');
  const near = (s: Series, vx: number) => s.points.reduce((best, p) => Math.abs(p[0] - vx) < Math.abs(best[0] - vx) ? p : best, s.points[0]);
  const hx = hover;
  return <div className="chart" ref={ref} role="img" aria-label={label}
    onPointerMove={e => { const r = ref.current!.getBoundingClientRect(); const px = (e.clientX - r.left) / r.width * W; setHover(xMin + Math.max(0, Math.min(1, (px - L) / (W - L - R))) * (xMax - xMin)); }}
    onPointerLeave={() => setHover(null)}>
    <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" style={{ height }}>
      {[0.25, 0.5, 0.75, 1].map(f => <line key={f} className="grid-line" x1={L} x2={W - R} y1={y(top * f)} y2={y(top * f)} strokeDasharray="2 4" vectorEffect="non-scaling-stroke" />)}
      <line className="grid-line" x1={L} x2={W - R} y1={y(0)} y2={y(0)} vectorEffect="non-scaling-stroke" />
      {series.map(s => <g key={s.key}>
        {s.area && <motion.path initial={{ opacity: 0 }} animate={{ opacity: 0.12 }} transition={{ duration: 0.9 }} d={`${path(s.points)}L${x(s.points.at(-1)![0])},${y(0)}L${x(s.points[0][0])},${y(0)}Z`} fill={s.color} />}
        <motion.path initial={{ pathLength: 0 }} animate={{ pathLength: 1 }} transition={{ duration: 0.9, ease: [0.2, 0.8, 0.2, 1] }} d={path(s.points)} fill="none" stroke={s.color} strokeWidth={1.8} strokeDasharray={s.dashed ? '4 4' : undefined} vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
      </g>)}
      {hx !== null && <line x1={x(hx)} x2={x(hx)} y1={T} y2={H - B} stroke="var(--line-strong)" vectorEffect="non-scaling-stroke" />}
    </svg>
    {series.filter(s => s.marker).map(s => <span key={s.key} style={{ position: 'absolute', left: `${x(s.marker![0]) / W * 100}%`, top: y(s.marker![1]), width: 9, height: 9, borderRadius: '50%', background: s.color, border: '2px solid var(--panel)', transform: 'translate(-50%,-50%)', boxShadow: '0 0 0 1px ' + s.color }} />)}
    <div style={{ position: 'relative', height: 14 }}>{xTicks.map(t => <span key={t.label} className="axis" style={{ position: 'absolute', left: `${x(t.x) / W * 100}%`, transform: 'translateX(-50%)', fontSize: 10, color: 'var(--faint)' }}>{t.label}</span>)}</div>
    {hx !== null && series.length > 0 && <div className="tip" style={{ left: `${x(hx) / W * 100}%`, top: y(Math.max(...series.map(s => near(s, hx)[1]))) }}>
      {xFormat(near(series[0], hx)[0])} · {series.map(s => `${s.label} ${yFormat(near(s, hx)[1])}`).join(' · ')}
    </div>}
  </div>;
}

export function Bars({ rows, format = v => String(v) }: { rows: { label: string; value: number }[]; format?: (v: number) => string }) {
  const max = Math.max(1, ...rows.map(r => r.value));
  return <div className="bars">{rows.map((r, i) => <div key={r.label}><span>{r.label}</span><span><motion.i initial={{ width: 0 }} animate={{ width: `${r.value / max * 100}%` }} transition={{ type: 'spring', stiffness: 200, damping: 28, delay: i * 0.04 }} /></span><span>{format(r.value)}</span></div>)}</div>;
}

export function Ring({ done, goal }: { done: number; goal: number }) {
  const r = 22, c = 2 * Math.PI * r, f = Math.min(1, done / goal);
  return <svg className="ring" width="56" height="56" viewBox="0 0 56 56" role="img" aria-label={`${done} из ${goal} XP сегодня`}>
    <circle className="bg" cx="28" cy="28" r={r} />
    <motion.circle className="fg" cx="28" cy="28" r={r} strokeDasharray={c} initial={{ strokeDashoffset: c }} animate={{ strokeDashoffset: c * (1 - f) }} transition={{ type: 'spring', stiffness: 120, damping: 20 }} transform="rotate(-90 28 28)" />
    <text x="28" y="32" textAnchor="middle">{done}</text>
  </svg>;
}

