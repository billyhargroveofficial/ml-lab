import type { ReactNode } from 'react';
import { Link } from '../App';
import { md } from '../md';
import algorithm from './algorithm.md?raw';

const GH = 'https://github.com/billyhargroveofficial/ml-lab/blob/main';

function Toc({ items }: { items: [string, string][] }) {
  return <nav className="toc">{items.map(([id, t]) => <a key={id} href={`#${id}`}>{t}</a>)}</nav>;
}
function Sec({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return <><h2 id={id}>{title}</h2>{children}</>;
}

export function Docs() {
  return <div className="wrap page">
    <div className="page-head"><div><h1>Как это работает</h1><p className="muted" style={{ margin: '6px 0 0' }}>FIRe Lab — локальное приложение: интервальное повторение по графу знаний, а темы для него пишет ваш агент.</p></div></div>
    <div className="docs docs-grid">
      <Toc items={[['packs', 'Пакеты'], ['agent', 'Новая тема через агента'], ['fire', 'Алгоритм'], ['placement', 'Вступительный тест'], ['local', 'Данные и прогресс']]} />
      <div className="prose">
        <Sec id="packs" title="Пакеты">
          <p>Всё содержимое — это <b>пакеты</b>: папки <code>packs/&lt;id&gt;/</code> в репозитории. В пакете файл <code>pack.json</code> с темами, уроками на Markdown с формулами и вопросами с разбором, плюс связи с идеями из других пакетов. Приложение перечитывает папку само: новый или исправленный пакет появляется через пару секунд. Если в пакете ошибка, он не загрузится, а на странице <Link to="/packs">пакеты</Link> будет видно, что не так.</p>
          <p>Формат с каждым полем — <a href={`${GH}/docs/pack-format.md`}>docs/pack-format.md</a> (генерируется из кода), схема для редактора — <a href={`${GH}/docs/pack.schema.json`}>pack.schema.json</a>, пример — <code>examples/packs/softmax</code>.</p>
        </Sec>
        <Sec id="agent" title="Новая тема через агента">
          <p>Откройте папку репозитория в Claude Code, Codex, Cursor или другом агенте и попросите, например: <i>«Я плохо читаю кванторы. Составь 6 уроков по 5–7 задач, от самых простых до жёстких»</i>. Инструкции для агента лежат в <code>AGENTS.md</code> и в навыке <code>.agents/skills/add-topic</code>: он поищет похожие идеи командой <code>./fire search</code>, напишет уроки и задачи, свяжет их с тем, что уже есть, расставит веса по рубрике и прогонит <code>./fire validate</code>, пока ошибок не останется.</p>
          <p>Без терминала (ChatGPT в браузере): дайте ему <a href={`${GH}/docs/pack-format.md`}>docs/pack-format.md</a> и попросите вернуть один <code>pack.json</code>; положите его в <code>packs/&lt;id&gt;/</code> и посмотрите ошибки на странице «пакеты».</p>
        </Sec>
        <Sec id="fire" title="Алгоритм">
          <div dangerouslySetInnerHTML={{ __html: md(algorithm) }} />
        </Sec>
        <Sec id="placement" title="Вступительный тест">
          <p>До 48 вопросов, каждый на одну идею. Верный ответ засчитывает идею и всё, на чём она стоит; ошибка или «не знаю» снимает всё, что на ней построено. Следующей спрашивается идея, ответ на которую решит больше всего в худшем случае — двоичный поиск, перенесённый с прямой на граф. Засчитанные идеи получают стабильность 10–30 дней, так что первые повторения проверят, правда ли они держатся.</p>
        </Sec>
        <Sec id="local" title="Данные и прогресс">
          <p>Аккаунтов нет. Прогресс лежит в <code>data/progress.db</code> на этом компьютере и привязан к кодам идей <code>&lt;пакет&gt;.&lt;тема&gt;/&lt;идея&gt;</code>: правка урока прогресс не сбрасывает, а смена кода — сбрасывает для этой идеи. Экспорт, импорт и сброс — на странице <Link to="/packs">пакеты</Link> или командой <code>./fire progress</code>. HTTP API для своих скриптов — <a href={`${GH}/docs/api.md`}>docs/api.md</a>.</p>
        </Sec>
      </div>
    </div>
  </div>;
}
