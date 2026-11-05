import { useEffect, useState } from 'react';
import { api, refresh, type AnswerView, type Attempt, type AttemptResult, type Kind, type PublicQuestion } from '../api';
import { md, mdInline } from '../md';
import { Link } from '../App';
import { AnimatePresence, motion, Num, soft, spring } from './motion';

const kindName: Record<Kind, string> = { lesson: 'урок', review: 'повторение', quiz: 'проверка' };
const letters = ['A', 'B', 'C', 'D', 'E', 'F'];

// One sheet runs a whole task: the lesson text, questions one by one with
// feedback, then the result with the ideas FIRe repeated implicitly.
export function Session({ kind, code, onClose, onNext }: { kind: Kind; code?: string; onClose: () => void; onNext: () => void }) {
  const [attempt, setAttempt] = useState<Attempt>();
  const [error, setError] = useState<string>();
  const [stage, setStage] = useState<'lesson' | 'question'>('question');
  const [question, setQuestion] = useState<PublicQuestion>();
  const [answer, setAnswer] = useState<{ choice: number; view: AnswerView }>();
  const [marks, setMarks] = useState<boolean[]>([]);
  const [result, setResult] = useState<AttemptResult>();
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api<Attempt>('/attempts', { kind, code }).then(a => { setAttempt(a); setQuestion(a.question); setStage(a.lesson ? 'lesson' : 'question'); }).catch(e => setError(e.message));
  }, [kind, code]);
  useEffect(() => {
    const fn = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close();
      if (stage === 'question' && !answer && question && /^[1-6]$/.test(e.key)) choose(Number(e.key) - 1);
      if (e.key === 'Enter' && answer && !result) proceed();
    };
    addEventListener('keydown', fn);
    return () => removeEventListener('keydown', fn);
  });
  const close = () => { if (result) refresh(); onClose(); };

  const choose = async (choice: number) => {
    if (!attempt || answer || busy) return;
    setBusy(true);
    try {
      const view = await api<AnswerView>(`/attempts/${attempt.id}/answer`, { choice });
      setAnswer({ choice, view });
      setMarks(m => [...m, view.correct]);
    } catch (e) { setError((e as Error).message); } finally { setBusy(false); }
  };
  const proceed = () => {
    if (!answer) return;
    if (answer.view.done) { setResult(answer.view.result); refresh(); return; }
    setQuestion(answer.view.next); setAnswer(undefined);
  };

  const steps = attempt ? Math.max(attempt.kind === 'lesson' ? Math.min(attempt.total, marks.length + 1) : attempt.total, marks.length) : 0;
  return <motion.div className="overlay" onMouseDown={e => { if (e.target === e.currentTarget) close(); }} initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={{ duration: 0.18 }}>
    <motion.div className="sheet" role="dialog" aria-modal="true" aria-label={attempt?.title ?? 'Задание'} initial={{ opacity: 0, y: 28, scale: 0.98 }} animate={{ opacity: 1, y: 0, scale: 1 }} exit={{ opacity: 0, y: 18, scale: 0.98 }} transition={spring} layout="size">
      <header>
        <span className="k">{kindName[kind]}</span>
        <h3>{attempt?.title ?? '…'}</h3>
        {attempt?.code && <Link to={`/idea/${attempt.code}`} className="muted">идея ↗</Link>}
        <button className="iconbtn" aria-label="Закрыть" onClick={close}>×</button>
      </header>
      {attempt && stage === 'question' && !result && <div className="steps" aria-hidden="true">{Array.from({ length: steps }, (_, i) => <motion.i key={i} layout initial={{ scaleX: 0 }} animate={{ scaleX: 1 }} transition={spring} style={{ originX: 0 }} className={i < marks.length ? (marks[i] ? 'ok' : 'no') : i === marks.length ? 'cur' : ''} />)}</div>}
      <div className="body">
        {error && <p className="alert">{error === 'task unavailable' ? 'Это задание сейчас недоступно — возможно, его уже прошли. Обновите список.' : error}</p>}
        {!attempt && !error && <p className="muted">Готовим задание…</p>}
        <AnimatePresence mode="wait" initial={false}>
        {attempt && stage === 'lesson' && <motion.div key="lesson" initial={{ opacity: 0, x: 16 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: -16 }} transition={soft}>
          {attempt.partial && <p className="muted">Вы уже начинали этот урок — хватит одного верного ответа.</p>}
          <div className="prose" dangerouslySetInnerHTML={{ __html: md(attempt.lesson ?? '') }} />
        </motion.div>}
        {attempt && stage === 'question' && question && !result && <motion.div key={`q${question.position}`} initial={{ opacity: 0, x: 24 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: -24 }} transition={soft}>
          {kind === 'quiz' && <p className="muted" style={{ marginTop: 0 }}>{question.title}</p>}
          <div className="q prose" dangerouslySetInnerHTML={{ __html: mdInline(question.question) }} />
          <div className="opts">{question.options.map((o, i) => {
            const state = answer ? (i === answer.view.correct_index ? 'right' : i === answer.choice ? 'wrong' : undefined) : undefined;
            return <motion.button key={i} className="opt" data-state={state} disabled={!!answer || busy} onClick={() => void choose(i)}
              initial={{ opacity: 0, y: 8 }} animate={state === 'wrong' ? { opacity: 1, y: 0, x: [0, -6, 6, -4, 4, 0] } : state === 'right' ? { opacity: 1, y: 0, scale: [1, 1.015, 1] } : { opacity: 1, y: 0 }} transition={{ ...soft, delay: answer ? 0 : 0.04 * i, x: { duration: 0.35 } }} whileHover={answer ? undefined : { x: 3 }} whileTap={answer ? undefined : { scale: 0.99 }}>
              <span className="l">{letters[i]}</span><span className="prose" dangerouslySetInnerHTML={{ __html: mdInline(o) }} />
            </motion.button>;
          })}</div>
          <AnimatePresence>{answer && <motion.div className={`explain ${answer.view.correct ? 'ok' : 'no'}`} initial={{ opacity: 0, height: 0, marginTop: 0 }} animate={{ opacity: 1, height: 'auto', marginTop: 14 }} exit={{ opacity: 0, height: 0 }} transition={soft} style={{ overflow: 'hidden' }}>
            <b>{answer.view.correct ? 'Верно.' : 'Не совсем.'}</b>{' '}
            {answer.view.explanation && <span className="prose" dangerouslySetInnerHTML={{ __html: mdInline(answer.view.explanation) }} />}
          </motion.div>}</AnimatePresence>
        </motion.div>}
        {result && <motion.div key="result" className="result" initial={{ opacity: 0, scale: 0.94 }} animate={{ opacity: 1, scale: 1 }} transition={spring}>
          <motion.div className="big" initial={{ scale: 0.6 }} animate={{ scale: [0.6, 1.12, 1] }} transition={{ duration: 0.5 }}>+<Num value={result.xp} /> XP</motion.div>
          <p>{result.passed ? (result.perfect ? 'Без единой ошибки.' : 'Засчитано.') : kind === 'lesson' ? 'Пока не засчитано: урок вернётся в ленту, во второй раз хватит одного верного ответа.' : 'Не засчитано — идея вернётся на повторение раньше.'} {result.correct} из {result.total} верно.</p>
          {result.implicit.length > 0 && <div className="implicit">
            <b>Неявно повторено ({result.implicit.length})</b>
            <p className="muted" style={{ margin: '4px 0 0' }}>Эта задача включает более простые темы — им засчитана доля повторения по весам FIRe.</p>
            <ul>{result.implicit.map((i, n) => <motion.li key={i.code} initial={{ opacity: 0, x: -8 }} animate={{ opacity: 1, x: 0 }} transition={{ ...soft, delay: 0.25 + n * 0.04 }}><Link to={`/idea/${i.code}`}>{i.title}</Link><span className="num">+{i.delta.toFixed(2)}</span></motion.li>)}</ul>
          </div>}
        </motion.div>}
        </AnimatePresence>
      </div>
      <footer>
        {attempt && stage === 'lesson' && <button className="btn primary" onClick={() => setStage('question')}>К вопросам →</button>}
        {answer && !result && <button className="btn primary" onClick={proceed}>{answer.view.done ? 'Итог' : 'Дальше'} ↵</button>}
        {result && <><button className="btn" onClick={close}>Закрыть</button><button className="btn primary" onClick={() => { refresh(); onNext(); }}>Следующее задание →</button></>}
        {error && <button className="btn" onClick={close}>Закрыть</button>}
      </footer>
    </motion.div>
  </motion.div>;
}
