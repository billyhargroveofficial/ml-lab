import { useState } from 'react';
import { api, refresh, useApi } from '../api';
import { useSession } from '../App';
import { AnimatePresence, motion, soft } from './motion';

export interface PinState { code: string; kind: 'idea' | 'topic'; title: string; left: number; next: string }

const lessons = (n: number) => { const a = n % 10, b = n % 100; return a === 1 && b !== 11 ? 'урок' : a >= 2 && a <= 4 && (b < 12 || b > 14) ? 'урока' : 'уроков'; };

export async function setPin(code: string) {
  const res = await api<{ pin: PinState | null }>('/pin', { code });
  refresh();
  return res.pin;
}

// "Study next": puts this idea (or the next lesson on the way to it) first in the feed.
export function PinButton({ code, learned, label = '↑ учить следующей', compact }: { code: string; learned?: boolean; label?: string; compact?: boolean }) {
  const { me } = useSession();
  const pin = useApi<{ pin: PinState | null }>(me ? '/pin' : null);
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState('');
  if (!me) return null;
  const mine = pin.data?.pin?.code === code;
  const click = async () => {
    setBusy(true); setNote('');
    try {
      const next = await setPin(mine ? '' : code);
      if (!mine && !next) setNote(code.includes('-top-') ? 'Вся тема уже выучена.' : 'Уже выучено — повторится по расписанию.');
    } catch (e) { setNote((e as Error).message); } finally { setBusy(false); }
  };
  return <span style={{ display: 'inline-flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
    <motion.button whileTap={{ scale: 0.96 }} className={`btn small${mine || compact ? '' : ' accent'}`} disabled={busy || learned} onClick={() => void click()} title={learned ? 'Уже выучено' : undefined}>
      {mine ? (compact ? '✓ снять' : '✓ цель · снять') : learned ? 'выучено' : label}
    </motion.button>
    <AnimatePresence mode="wait">{(mine || note) && !compact && <motion.small key={note || 'left'} className="muted" initial={{ opacity: 0, x: -6 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0 }} transition={soft}>
      {note || (pin.data?.pin && `${pin.data.pin.left === 1 ? 'следующий урок в ленте' : `путь — ${pin.data.pin.left} ${lessons(pin.data.pin.left)}`}`)}
    </motion.small>}</AnimatePresence>
  </span>;
}

export { lessons };
