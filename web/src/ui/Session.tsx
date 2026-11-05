import { useEffect, useState } from 'react';
import { api, refresh, type AnswerView, type Attempt, type AttemptResult, type Kind, type PublicQuestion } from '../api';
import { md, mdInline } from '../md';
import { Link } from '../App';

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
  return <div className="overlay" onMouseDown={e => { if (e.target === e.currentTarget) close(); }}>
    <div className="sheet" role="dialog" aria-modal="true" aria-label={attempt?.title ?? 'Задание'}>
      <header>
        <span className="k">{kindName[kind]}</span>
        <h3>{attempt?.title ?? '…'}</h3>
        {attempt?.code && <Link to={`/idea/${attempt.code}`} className="muted">идея ↗</Link>}
        <button className="iconbtn" aria-label="Закрыть" onClick={close}>×</button>
      </header>
      {attempt && stage === 'question' && !result && <div className="steps" aria-hidden="true">{Array.from({ length: steps }, (_, i) => <i key={i} className={i < marks.length ? (marks[i] ? 'ok' : 'no') : i === marks.length ? 'cur' : ''} />)}</div>}
      <div className="body">
        {error && <p className="alert">{error === 'task unavailable' ? 'Это задание сейчас недоступно — возможно, его уже прошли. Обновите список.' : error}</p>}
        {!attempt && !error && <p className="muted">Готовим задание…</p>}
        {attempt && stage === 'lesson' && <>
          {attempt.partial && <p className="muted">Вы уже начинали этот урок — хватит одного верного ответа.</p>}
          <div className="prose" dangerouslySetInnerHTML={{ __html: md(attempt.lesson ?? '') }} />
        </>}
        {attempt && stage === 'question' && question && !result && <>
          {kind === 'quiz' && <p className="muted" style={{ marginTop: 0 }}>{question.title}</p>}
          <div className="q prose" dangerouslySetInnerHTML={{ __html: mdInline(question.question) }} />
          <div className="opts">{question.options.map((o, i) => {
            const state = answer ? (i === answer.view.correct_index ? 'right' : i === answer.choice ? 'wrong' : undefined) : undefined;
            return <button key={i} className="opt" data-state={state} disabled={!!answer || busy} onClick={() => void choose(i)}>
              <span className="l">{letters[i]}</span><span className="prose" dangerouslySetInnerHTML={{ __html: mdInline(o) }} />
            </button>;
          })}</div>
          {answer && <div className={`explain ${answer.view.correct ? 'ok' : 'no'}`}>
            <b>{answer.view.correct ? 'Верно.' : 'Не совсем.'}</b>{' '}
            {answer.view.explanation && <span className="prose" dangerouslySetInnerHTML={{ __html: mdInline(answer.view.explanation) }} />}
          </div>}
        </>}
        {result && <div className="result">
          <div className="big">+{result.xp} XP</div>
          <p>{result.passed ? (result.perfect ? 'Без единой ошибки.' : 'Засчитано.') : kind === 'lesson' ? 'Пока не засчитано: урок вернётся в ленту, во второй раз хватит одного верного ответа.' : 'Не засчитано — идея вернётся на повторение раньше.'} {result.correct} из {result.total} верно.</p>
          {result.implicit.length > 0 && <div className="implicit">
            <b>Неявно повторено ({result.implicit.length})</b>
            <p className="muted" style={{ margin: '4px 0 0' }}>Эта задача включает более простые темы — им засчитана доля повторения по весам FIRe.</p>
            <ul>{result.implicit.map(i => <li key={i.code}><Link to={`/idea/${i.code}`}>{i.title}</Link><span className="num">+{i.delta.toFixed(2)}</span></li>)}</ul>
          </div>}
        </div>}
      </div>
      <footer>
        {attempt && stage === 'lesson' && <button className="btn primary" onClick={() => setStage('question')}>К вопросам →</button>}
        {answer && !result && <button className="btn primary" onClick={proceed}>{answer.view.done ? 'Итог' : 'Дальше'} ↵</button>}
        {result && <><button className="btn" onClick={close}>Закрыть</button><button className="btn primary" onClick={() => { refresh(); onNext(); }}>Следующее задание →</button></>}
        {error && <button className="btn" onClick={close}>Закрыть</button>}
      </footer>
    </div>
  </div>;
}
