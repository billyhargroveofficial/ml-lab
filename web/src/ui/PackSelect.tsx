import { useEffect, useRef, useState } from 'react';
import type { PackInfo } from '../api';
import { AnimatePresence, motion, spring } from './motion';

// One dropdown for choosing a pack on every page. `all` adds "все пакеты".
export function PackSelect({ packs, value, onChange, all }: { packs: PackInfo[]; value: string; onChange: (v: string) => void; all?: boolean }) {
  const [open, setOpen] = useState(false);
  const [hover, setHover] = useState(0);
  const box = useRef<HTMLDivElement>(null);
  const items = [...(all ? [{ id: 'all', title: 'все пакеты', ideas: packs.reduce((n, p) => n + p.ideas, 0) }] : []), ...packs.map(p => ({ id: p.id, title: p.title, ideas: p.ideas }))];
  const current = items.find(i => i.id === value) ?? items[0];
  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false); };
    addEventListener('mousedown', away);
    return () => removeEventListener('mousedown', away);
  }, [open]);
  const pick = (id: string) => { onChange(id); setOpen(false); };
  const key = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') setOpen(false);
    else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      if (!open) { setOpen(true); setHover(Math.max(0, items.findIndex(i => i.id === value))); return; }
      setHover(h => (h + (e.key === 'ArrowDown' ? 1 : items.length - 1)) % items.length);
    } else if (e.key === 'Enter' && open) { e.preventDefault(); pick(items[hover].id); }
  };
  if (!current) return null;
  return <div className="dropdown" ref={box} onKeyDown={key}>
    <button type="button" className="dd-btn" aria-haspopup="listbox" aria-expanded={open} onClick={() => { setOpen(o => !o); setHover(Math.max(0, items.findIndex(i => i.id === value))); }}>
      <span className="dd-label">пакет</span><b>{current.title}</b><span className="num muted">{current.ideas}</span>
      <motion.span className="chev" animate={{ rotate: open ? 180 : 0 }} transition={spring} aria-hidden="true">▾</motion.span>
    </button>
    <AnimatePresence>{open && <motion.ul className="dd-list" role="listbox" initial={{ opacity: 0, y: -6, scale: 0.98 }} animate={{ opacity: 1, y: 0, scale: 1 }} exit={{ opacity: 0, y: -4, scale: 0.98 }} transition={{ duration: 0.14 }}>
      {items.map((i, n) => <li key={i.id} role="option" aria-selected={i.id === value} data-hover={n === hover || undefined} onMouseEnter={() => setHover(n)} onClick={() => pick(i.id)}>
        <span>{i.title}</span><span className="num muted">{i.ideas}</span>
      </li>)}
    </motion.ul>}</AnimatePresence>
  </div>;
}
