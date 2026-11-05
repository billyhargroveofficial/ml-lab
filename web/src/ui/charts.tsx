import { useMemo, useRef, useState, type CSSProperties } from 'react';
import type { MarketTrack, Me } from '../api';

export const TRACK_COLOR = { all: 'var(--fg)', ds: 'var(--ds)', ml: 'var(--ml)', ai: 'var(--ai)' } as const;
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
        {s.area && <path d={`${path(s.points)}L${x(s.points.at(-1)![0])},${y(0)}L${x(s.points[0][0])},${y(0)}Z`} fill={s.color} opacity={0.12} />}
        <path d={path(s.points)} fill="none" stroke={s.color} strokeWidth={1.8} strokeDasharray={s.dashed ? '4 4' : undefined} vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
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
  return <div className="bars">{rows.map(r => <div key={r.label}><span>{r.label}</span><span><i style={{ width: `${r.value / max * 100}%` }} /></span><span>{format(r.value)}</span></div>)}</div>;
}

export function Ring({ done, goal }: { done: number; goal: number }) {
  const r = 22, c = 2 * Math.PI * r, f = Math.min(1, done / goal);
  return <svg className="ring" width="56" height="56" viewBox="0 0 56 56" role="img" aria-label={`${done} из ${goal} XP сегодня`}>
    <circle className="bg" cx="28" cy="28" r={r} />
    <circle className="fg" cx="28" cy="28" r={r} strokeDasharray={c} strokeDashoffset={c * (1 - f)} transform="rotate(-90 28 28)" />
    <text x="28" y="32" textAnchor="middle">{done}</text>
  </svg>;
}

// ---- the market model (same as the old ML LAB) ----
export const curveAt = (curve: number[], coverage: number) => {
  const x = Math.min(100, Math.max(0, coverage * 100));
  const lo = Math.floor(x), hi = Math.min(100, lo + 1);
  return (curve[lo] ?? 0) + ((curve[hi] ?? 0) - (curve[lo] ?? 0)) * (x - lo);
};

// x is the number of ideas learned in interview-frequency order.
export function offerCurve(track: MarketTrack): [number, number][] {
  const total = track.order.reduce((s, i) => s + i.asked, 0);
  let acc = 0;
  return [[0, track.interviews * curveAt(track.curve.per_interview, 0)], ...track.order.map((idea, i) => {
    acc += idea.asked;
    return [i + 1, track.interviews * curveAt(track.curve.per_interview, total ? acc / total * (1 - track.unseen) : 0)] as [number, number];
  })];
}

export function equivalentIdeas(track: MarketTrack, coverage: number) {
  const total = track.order.reduce((s, i) => s + i.asked, 0);
  let acc = 0, prev = 0;
  for (let i = 0; i < track.order.length; i++) {
    acc += track.order[i].asked;
    const cur = acc / total * (1 - track.unseen);
    if (cur >= coverage) return i + (cur > prev ? (coverage - prev) / (cur - prev) : 1);
    prev = cur;
  }
  return track.order.length;
}

export function OfferChart({ tracks, me }: { tracks: Record<string, MarketTrack>; me: Partial<Record<'ds' | 'ml' | 'ai', Me>> }) {
  const series = useMemo(() => (['ds', 'ml', 'ai'] as const).map(key => {
    const t = tracks[key];
    const pts = offerCurve(t);
    const own = me[key];
    const pos = own ? equivalentIdeas(t, own.coverage) : undefined;
    return { key, label: key.toUpperCase(), color: TRACK_COLOR[key], points: pts, marker: pos !== undefined ? [pos, t.interviews * curveAt(t.curve.per_interview, own!.coverage)] as [number, number] : undefined };
  }), [tracks, me]);
  const n = Math.max(...series.map(s => s.points.length - 1));
  return <>
    <LineChart label="Ожидаемые офферы в зависимости от числа выученных идей" series={series} height={120} xTicks={[0, 100, 200, 300, 400].filter(v => v <= n).map(v => ({ x: v, label: String(v) }))} xFormat={v => `${fmt(v, 0)} идей`} yFormat={v => fmt(v, 2)} />
    <div className="legend">{series.map(s => <span key={s.key}><i style={{ background: s.color } as CSSProperties} />{s.label}</span>)}<span>x — идей по частоте, y — офферов за {fmt(tracks.all.interviews, 0)} собесов</span></div>
  </>;
}
