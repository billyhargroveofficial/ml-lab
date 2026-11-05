import { animate, AnimatePresence, motion, useMotionValue, useTransform, type HTMLMotionProps, type Transition } from 'framer-motion';
import { useEffect, useId, useState, type CSSProperties, type ReactNode } from 'react';

// One motion vocabulary for the whole site: springs for things that move,
// short tweens for things that fade.
export const spring: Transition = { type: 'spring', stiffness: 420, damping: 36, mass: 0.8 };
export const soft: Transition = { type: 'spring', stiffness: 260, damping: 30 };
export const fade: Transition = { duration: 0.18, ease: [0.2, 0.8, 0.2, 1] };

// Panels rise in once, staggered by their order on the page.
export function Panel({ order = 0, className = 'panel', children, ...rest }: { order?: number; className?: string; children: ReactNode } & HTMLMotionProps<'section'>) {
  return <motion.section className={className} initial={{ opacity: 0, y: 14 }} animate={{ opacity: 1, y: 0 }} transition={{ ...soft, delay: order * 0.05 }} {...rest}>{children}</motion.section>;
}

// Segmented control with a sliding pill under the active option.
export function Seg<T extends string>({ value, items, onChange, label, big }: { value: T; items: [T, string][]; onChange: (v: T) => void; label: string; big?: boolean }) {
  const id = useId();
  return <div className={`seg${big ? ' big' : ''}`} role="group" aria-label={label}>{items.map(([v, l]) => <button key={v} type="button" aria-pressed={value === v} onClick={() => onChange(v)}>
    {value === v && <motion.span layoutId={`seg-${id}`} className="seg-pill" transition={spring} />}
    <span className="seg-label">{l}</span>
  </button>)}</div>;
}

// Counts up to the value; later changes animate from the previous value.
export function Num({ value, format = v => String(Math.round(v)) }: { value: number; format?: (v: number) => string }) {
  const mv = useMotionValue(0);
  const text = useTransform(mv, v => format(v));
  useEffect(() => { const c = animate(mv, value, { duration: 0.8, ease: [0.2, 0.8, 0.2, 1] }); return () => c.stop(); }, [value, mv]);
  return <motion.span>{text}</motion.span>;
}

// Height-animated disclosure (replaces <details>, which cannot animate).
export function Collapse({ open, children }: { open: boolean; children: ReactNode }) {
  return <AnimatePresence initial={false}>{open && <motion.div key="c" initial={{ height: 0, opacity: 0 }} animate={{ height: 'auto', opacity: 1 }} exit={{ height: 0, opacity: 0 }} transition={soft} style={{ overflow: 'hidden' }}>{children}</motion.div>}</AnimatePresence>;
}

// Bars and meters grow to their width.
export function Grow({ width, className, style }: { width: number; className?: string; style?: CSSProperties }) {
  return <motion.i className={className} style={style} initial={{ width: 0 }} animate={{ width: `${Math.max(0, Math.min(100, width))}%` }} transition={soft} />;
}

export function useMounted() { const [m, setM] = useState(false); useEffect(() => setM(true), []); return m; }

export { AnimatePresence, motion };
