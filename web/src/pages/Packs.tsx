import { useRef, useState } from 'react';
import { api, BASE, refresh, useApi, type PackStatus } from '../api';
import { Panel } from '../ui/motion';

export function PacksPage() {
  const packs = useApi<PackStatus[]>('/packs');
  const file = useRef<HTMLInputElement>(null);
  const [note, setNote] = useState('');
  const importFile = async (f: File) => {
    if (!confirm('Заменить весь текущий прогресс прогрессом из файла?')) return;
    const res = await fetch(BASE + '/progress/import', { method: 'POST', headers: { 'X-Requested-With': 'fire', 'Content-Type': 'application/json' }, body: await f.text() });
    const data = await res.json().catch(() => ({}));
    setNote(res.ok ? 'Прогресс загружен.' : `Ошибка: ${data.error ?? res.statusText}`);
    refresh(true);
  };
  const reset = async () => {
    if (prompt('Это сотрёт весь прогресс. Напишите «reset», чтобы подтвердить.') !== 'reset') return;
    await api('/progress/reset', { confirm: 'reset' }).then(() => setNote('Прогресс сброшен.')).catch(e => setNote((e as Error).message));
    refresh(true);
  };
  return <div className="wrap page">
    <div className="page-head"><div><h1>Пакеты</h1><p className="muted" style={{ margin: '6px 0 0' }}>Папки из <code>packs/</code>. Изменения подхватываются сами через пару секунд.</p></div></div>
    <Panel order={1} style={{ marginBottom: 16 }}>
      <div className="phead"><h2>Загружено</h2><span>{packs.data ? `${packs.data.filter(p => p.loaded).length} из ${packs.data.length}` : ''}</span></div>
      {!packs.data ? <p className="skel">загрузка</p> : !packs.data.length ? <p className="muted">Пакетов нет. Попросите агента составить тему (см. <code>AGENTS.md</code>) или скопируйте пример: <code>cp -r examples/packs/softmax packs/</code>.</p> :
        packs.data.map(p => <div className="packrow" key={p.id}>
          <b>{p.title || p.id}</b> <span className="muted">· <code>{p.id}</code> · {p.topics} тем, {p.ideas} уроков · {p.loaded ? 'загружен' : <span className="err">не загружен</span>}</span>
          {!!p.errors?.length && <ul className="err">{p.errors.slice(0, 20).map((e, i) => <li key={i}>{e}</li>)}{p.errors.length > 20 && <li>…и ещё {p.errors.length - 20}, полный список: <code>./fire validate {p.id}</code></li>}</ul>}
          {!!p.warnings?.length && <ul className="muted">{p.warnings.slice(0, 10).map((e, i) => <li key={i}>{e}</li>)}</ul>}
        </div>)}
    </Panel>
    <Panel order={2}>
      <div className="phead"><h2>Прогресс</h2><span>data/progress.db на этом компьютере</span></div>
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        <a className="btn" href={BASE + '/progress/export'} download>экспорт в JSON</a>
        <button className="btn" onClick={() => file.current?.click()}>импорт из JSON</button>
        <button className="btn" onClick={() => void reset()}>сбросить</button>
        <input ref={file} type="file" accept="application/json" hidden onChange={e => { const f = e.target.files?.[0]; if (f) void importFile(f); e.target.value = ''; }} />
      </div>
      {note && <p className="muted" style={{ marginBottom: 0 }}>{note}</p>}
    </Panel>
  </div>;
}
