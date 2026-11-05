import { useEffect, useState } from 'react';
import { api, refresh, type PublicQuestion, type Track } from '../api';
import { mdInline } from '../md';
import { AnimatePresence, motion, Num, soft, spring } from './motion';

interface View {
  done: boolean; probe: number; max: number; question?: PublicQuestion;
  correct?: boolean; correct_index?: number; explanation?: string;
  known: number; total: number;
}
const letters = ['A', 'B', 'C', 'D', 'E', 'F'];

// Placement test: one question per probe; a right answer settles the idea and
// everything below it, a wrong one or "не знаю" everything built on it.
export function Placement({ track, onClose }: { track: Track; onClose: () => void }) {
  const [view, setView] = useState<View>();
  const [question, setQuestion] = useState<PublicQuestion>();
  const [answer, setAnswer] = useState<{ choice: number; view: View }>();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api<View>(`/placement?track=${track}`, {}).then(v => { setView(v); setQuestion(v.question); }).catch(e => setError(e.message));
  }, [track]);
  const choose = async (choice: number) => {
    if (answer || busy || !question) return;
    setBusy(true);
    try { const v = await api<View>('/placement/answer', { choice }); setAnswer({ choice, view: v }); if (v.done) refresh(); }
    catch (e) { setError((e as Error).message); } finally { setBusy(false); }
  };
  const proceed = () => {
    if (!answer) return;
    setView(answer.view);
    setQuestion(answer.view.done ? undefined : answer.view.question);
    setAnswer(undefined);
  };
  const close = () => { refresh(); onClose(); };
  useEffect(() => {
    const fn = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close();
      if (!answer && question && /^[1-6]$/.test(e.key)) void choose(Number(e.key) - 1);
      if (!answer && question && e.key === '0') void choose(-1);
      if (e.key === 'Enter' && answer) proceed();
    };
    addEventListener('keydown', fn);
    return () => removeEventListener('keydown', fn);
  });

  const done = view?.done && !answer;
  const probe = answer ? answer.view.probe - 1 : (view?.probe ?? 1);
  return <motion.div className="overlay" onMouseDown={e => { if (e.target === e.currentTarget) close(); }} initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={{ duration: 0.18 }}>
    <motion.div className="sheet" role="dialog" aria-modal="true" aria-label="Вступительный тест" initial={{ opacity: 0, y: 28, scale: 0.98 }} animate={{ opacity: 1, y: 0, scale: 1 }} exit={{ opacity: 0, y: 18, scale: 0.98 }} transition={spring} layout="size">
      <header>
        <span className="k">тест</span>
        <h3>Вступительный тест</h3>
        {view && !done && <span className="muted num">{probe} / до {view.max}</span>}
        <button className="iconbtn" aria-label="Закрыть" onClick={close}>×</button>
      </header>
      {view && !done && <div className="steps" aria-hidden="true"><motion.i className="cur" animate={{ scaleX: probe / view.max }} transition={soft} style={{ originX: 0, gridColumn: '1 / -1' }} /></div>}
      <div className="body">
        {error && <p className="alert">{error}</p>}
        {!view && !error && <p className="muted">Подбираем вопросы…</p>}
        {view?.probe === 1 && !answer && !done && <p className="muted" style={{ marginTop: 0 }}>Вопросы подбираются по графу знаний: верный ответ засчитывает тему и всё, на чём она стоит, ошибка снимает то, что на ней построено. Не угадывайте — честное «не знаю» лучше: ошибочно засчитанную тему придётся выяснять на повторениях.</p>}
        <AnimatePresence mode="wait" initial={false}>
          {question && !done && <motion.div key={`${question.code}:${probe}`} initial={{ opacity: 0, x: 24 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: -24 }} transition={soft}>
            <p className="muted" style={{ marginTop: 0 }}>{question.title}</p>
            <div className="q prose" dangerouslySetInnerHTML={{ __html: mdInline(question.question) }} />
            <div className="opts">{question.options.map((o, i) => {
              const state = answer ? (i === answer.view.correct_index ? 'right' : i === answer.choice ? 'wrong' : undefined) : undefined;
              return <motion.button key={i} className="opt" data-state={state} disabled={!!answer || busy} onClick={() => void choose(i)} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ ...soft, delay: answer ? 0 : 0.04 * i }} whileHover={answer ? undefined : { x: 3 }}>
                <span className="l">{letters[i]}</span><span className="prose" dangerouslySetInnerHTML={{ __html: mdInline(o) }} />
              </motion.button>;
            })}
              <motion.button className="opt" data-state={answer?.choice === -1 ? 'wrong' : undefined} disabled={!!answer || busy} onClick={() => void choose(-1)} whileHover={answer ? undefined : { x: 3 }}><span className="l">0</span><span>не знаю</span></motion.button>
            </div>
            <AnimatePresence>{answer && <motion.div className={`explain ${answer.view.correct ? 'ok' : 'no'}`} initial={{ opacity: 0, height: 0, marginTop: 0 }} animate={{ opacity: 1, height: 'auto', marginTop: 14 }} exit={{ opacity: 0, height: 0 }} transition={soft} style={{ overflow: 'hidden' }}>
              <b>{answer.view.correct ? 'Верно.' : answer.choice === -1 ? 'Хорошо, начнём с этого.' : 'Не совсем.'}</b>{' '}
              {answer.view.explanation && <span className="prose" dangerouslySetInnerHTML={{ __html: mdInline(answer.view.explanation) }} />}
            </motion.div>}</AnimatePresence>
          </motion.div>}
          {done && <motion.div key="result" className="result" initial={{ opacity: 0, scale: 0.94 }} animate={{ opacity: 1, scale: 1 }} transition={spring}>
            <motion.div className="big" initial={{ scale: 0.6 }} animate={{ scale: [0.6, 1.12, 1] }} transition={{ duration: 0.5 }}><Num value={view!.known} /> тем</motion.div>
            <p>{view!.known > 0 ? `Засчитаны как уже знакомые — из ${view!.total} в программе. Они не придут уроками, а первые повторения покажут, правда ли они держатся.` : 'Начнём с самого начала программы — так надёжнее.'}</p>
          </motion.div>}
        </AnimatePresence>
      </div>
      <footer>
        {answer && <button className="btn primary" onClick={proceed}>{answer.view.done ? 'Итог' : 'Дальше'} ↵</button>}
        {(done || error) && <button className="btn primary" onClick={close}>К ленте →</button>}
      </footer>
    </motion.div>
  </motion.div>;
}
