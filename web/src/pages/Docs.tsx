import type { ReactNode } from 'react';
import { useApi } from '../api';
import { Link } from '../App';

const origin = typeof location === 'undefined' ? 'https://billyhargrove.ru' : location.origin;
const B = `${origin}/ml/api/v1`;

function Ep({ m = 'GET', path, children }: { m?: string; path: string; children: ReactNode }) {
  return <><div className="endpoint"><span className={`m${m === 'POST' ? ' post' : ''}`}>{m}</span><code>{path}</code></div>{children}</>;
}

const toc = [['about', 'Что это'], ['data', 'Данные'], ['fire', 'Алгоритм FIRe'], ['weights', 'Веса Codex'], ['api', 'Открытый API'], ['learning', 'API ученика'], ['stats', 'Статистика'], ['privacy', 'Данные и права'], ['selfhost', 'Свой сервер']];

export function Docs() {
  const info = useApi<{ ideas: Record<string, number>; info: { sources: Record<string, string> } }>('/info');
  return <div className="wrap page docs">
    <div className="page-head"><div><h1>Документация и API</h1><p className="muted" style={{ margin: '6px 0 0' }}>Всё содержимое открыто: программа, уроки, вопросы с ответами, классификация и веса. Код — MIT, контент — CC BY 4.0.</p></div>
      <a className="btn primary" href="/ml/data/content.db">скачать content.db</a></div>
    <div className="docs-grid">
      <nav className="toc" aria-label="Содержание">{toc.map(([id, t]) => <a key={id} href={`#${id}`}>{t}</a>)}</nav>
      <div>
        <h2 id="about">Что это</h2>
        <p>ML Lab — тренажёр подготовки к собеседованиям DS / ML / AI. Две программы: <b>ML теория</b> — {info.data?.ideas.ml ?? 427} идей, которые реально спрашивали на 167 разобранных собесах, и <b>ML математика</b> — {info.data?.ideas.math ?? 1642} тем от школьной арифметики до SVD, выпуклой оптимизации и статистики. У каждой идеи есть урок и 3–6 вопросов с четырьмя вариантами и разбором.</p>
        <p>Расписание строит FIRe — интервальное повторение по знаниям-графу, описанное в книге Джастина Скайкака «The Math Academy Way». Главная идея: если решил составную задачу, ты заодно повторил простые темы внутри неё, и отдельно их повторять не нужно.</p>

        <h2 id="data">Данные</h2>
        <p>База <code>content.db</code> — обычный SQLite, только для чтения. Её можно скачать целиком и разбирать как угодно:</p>
        <pre>{`curl -o ml-lab.db ${origin}/ml/data/content.db
sqlite3 ml-lab.db "SELECT program, count(*) FROM ideas GROUP BY program"`}</pre>
        <table><thead><tr><th>таблица</th><th>что внутри</th></tr></thead><tbody>
          <tr><td><code>sections</code>, <code>topics</code></td><td>дерево программы: раздел → тема; у ML — сколько собесов затронули</td></tr>
          <tr><td><code>ideas</code></td><td>идея (единица обучения): <code>code</code>, <code>program</code> (ml|math), уровень 1–3, вес, урок в Markdown с формулами LaTeX</td></tr>
          <tr><td><code>questions</code></td><td>вопросы квизов: варианты (JSON), индекс верного, объяснение, сложность</td></tr>
          <tr><td><code>interview_shares</code></td><td>доля собесов трека (ds/ml/ai), где спрашивали идею, 95 % интервал Клоппера — Пирсона, флаг «часто» (FDR 10 %)</td></tr>
          <tr><td><code>formulations</code></td><td>перефразированные формулировки интервьюеров со счётчиками</td></tr>
          <tr><td><code>edges</code></td><td>граф: <code>kind=prerequisite</code> — пререквизит внутри программы (блокирует урок), <code>kind=implicit</code> — ML-идея неявно повторяет тему математики; <code>encompassing</code> 0–1, <code>source</code></td></tr>
          <tr><td><code>meta</code></td><td><code>info</code> — версия схемы и SHA-256 исходников; <code>market</code> — индекс рынка: кривые офферов, форматы, перегретость</td></tr>
        </tbody></table>
        <p>Классификация ML-идей построена по корпусу из 167 собеседований (57 DS, 78 ML, 32 AI); доли считаются на эффективное число собесов с поправкой на кластеры по компаниям. Уроки и вопросы сгенерированы моделями и выборочно проверены — если нашли ошибку, присылайте issue.</p>

        <h2 id="fire">Алгоритм FIRe</h2>
        <p>У каждой идеи ученика есть состояние: число повторений <code>rep</code>, память <code>memory</code> ∈ [0, 1], интервал и скорость. Память затухает экспоненциально с периодом полураспада, равным интервалу; идея «пора повторить», когда память падает ниже 0,5. Интервалы растут по лестнице 1, 3, 7, 14, 30, 60, 120, 240, 365 дней в зависимости от <code>rep</code>.</p>
        <p><b>Урок</b> засчитан, когда подряд даны два верных ответа (один, если урок уже начинали); два неверных подряд — урок вернётся. <b>Повторение</b> — два вопроса: все верны — полный зачёт (+1), часть — половина, ни одного — провал. <b>Проверка</b> появляется каждые 150 XP: 10 вопросов по выученным идеям, чаще по тем, что подзабыты.</p>
        <p><b>Неявное повторение.</b> Когда задача решена с оценкой <i>r</i>, зачёт растекается по графу вниз: тема на пути получает <i>r · w₁ · w₂ · …</i>, где <i>w</i> — веса <code>encompassing</code> рёбер (берётся путь с максимальным весом, отсечка 0,1). Зачёт масштабируется на <code>1 − memory</code>: свежая тема почти ничего не получает, подзабытая — почти полное повторение. Провал, наоборот, идёт вверх: более сложные идеи, опирающиеся на проваленную, теряют память, а ключевые пререквизиты провального урока сразу ставятся на повторение.</p>
        <p><b>Лента</b>: сначала проверка, если пора; затем повторения, но если есть доступный урок, который покрывает просроченную тему с весом ≥ 0,5, вместо повторения дают урок — он закроет её неявно. Не меньше четверти ленты — новые уроки, по убыванию веса идеи на собесах выбранного трека.</p>

        <h2 id="weights">Веса Codex</h2>
        <p>Рёбра <code>implicit</code> (ML → математика) и уточнённые веса ML → ML расставил Codex <code>gpt-6.1-sol</code> (effort high) по правилу: 1,0 — составная задача целиком отрабатывает простую тему, 0,5 — существенную часть, 0,1–0,3 — касается слегка. Для каждой из 427 ML-идей ему давали полный каталог из 1642 тем математики и просили выбрать до 8 самых прямых тем, не их цепочку пререквизитов. Итог: 559 рёбер у 273 идей. Промпт и скрипт — в <code>tools/codex-weights</code> репозитория. Рёбра <code>implicit</code> никогда не блокируют урок — они только раздают повторение.</p>

        <h2 id="api">Открытый API</h2>
        <p>Без ключей и регистрации, CORS открыт, ответы — JSON. Кэшируются на 5 минут. Базовый адрес: <code>{B}</code></p>
        <Ep path="/info"><p>Версия схемы, лицензия, хэши исходников и число идей по программам.</p></Ep>
        <Ep path="/program/{ml|math}"><p>Дерево программы: разделы → темы → коды идей, плюс краткая карточка каждой идеи (название, уровень, вес, число собесов).</p><pre>{`curl ${B}/program/ml | jq '.sections[0].name'`}</pre></Ep>
        <Ep path="/ideas?q=&program=&limit=&offset="><p>Поиск по названию, определению и коду. <code>limit</code> до 200.</p><pre>{`curl "${B}/ideas?q=бустинг&limit=5"`}</pre></Ep>
        <Ep path="/ideas/{code}"><p>Идея целиком: урок, вопросы с ответами и объяснениями, доли собесов, формулировки, <code>prerequisites</code> (на что опирается) и <code>encompassed_by</code> (какие задачи её повторяют).</p><pre>{`curl ${B}/ideas/nlp.attention_transformer/attention-mechanism`}</pre></Ep>
        <Ep path="/edges?kind=prerequisite|implicit"><p>Весь граф одним списком: <code>{'{from, code, key, encompassing, kind, source}'}</code>.</p></Ep>
        <Ep path="/market"><p>Индекс рынка: по трекам — люди, вакансии, кривые «покрытие → вероятность оффера» (101 точка), порядок идей по частоте; форматы работы и перегретость (×1 — средний IT).</p></Ep>
        <Ep path="/leaders?period=all|week"><p>Топ по XP: ник, XP, выучено ML / математики, покрытие и ожидаемые офферы.</p></Ep>
        <Ep path="/stats"><p id="stats">Обезличенная статистика — см. ниже.</p></Ep>

        <h2 id="learning">API ученика</h2>
        <p>Сессия — cookie <code>mllab</code> (HttpOnly). Все POST требуют заголовок <code>X-Requested-With: mllab</code>. Трек — <code>all | ds | ml | ai | math</code>.</p>
        <Ep m="POST" path="/auth/register · /auth/login · /auth/logout"><pre>{`{"name": "ник", "password": "от 6 символов"}`}</pre></Ep>
        <Ep path="/me?track="><p>Прогресс: выучено, пора повторить, покрытие, ожидаемые офферы, XP по дням, состояние каждой идеи, прогноз оставшегося XP.</p></Ep>
        <Ep path="/next?track="><p>Лента FIRe — до 8 заданий <code>{'{kind, code, title, xp, covers}'}</code>.</p></Ep>
        <Ep m="POST" path="/attempts"><pre>{`{"kind": "lesson" | "review" | "quiz", "code": "py.functions/decorators"}`}</pre><p>Возвращает урок (для lesson) и первый вопрос без ответа.</p></Ep>
        <Ep m="POST" path="/attempts/{id}/answer"><pre>{`{"choice": 2}  →  {"correct", "correct_index", "explanation", "done", "next" | "result"}`}</pre><p>В <code>result.implicit</code> — какие идеи получили неявное повторение и сколько.</p></Ep>
        <Ep path="/history"><p>Последние 40 заданий.</p></Ep>

        <h2>Статистика</h2>
        <p>Эндпоинт <code>/stats</code> и страница <Link to="/stats">статистики</Link> отдают только агрегаты: число учеников и активных, активность по дням, гистограммы «сколько выучено» и «какое покрытие», доля уроков, сданных с первого раза, по разделам и идеям — только там, где не меньше 5 попыток. Ники и персональные строки туда не попадают. Пересчитывается раз в минуту.</p>

        <h2 id="privacy">Данные пользователей и права</h2>
        <p>При регистрации сохраняются только ник, хэш пароля (PBKDF2-SHA256) и учебный прогресс: ответы, XP, состояние повторений. Почта, телефон, имя не запрашиваются; IP-адреса сервер не записывает, сторонней аналитики и трекеров нет, cookie одна — сессионная. Ник и XP видны всем в топе, поэтому настоящее имя в нике лучше не указывать. Кнопка «удалить аккаунт» внизу страницы удаляет пользователя вместе со всем прогрессом безвозвратно. В публичную статистику попадают только агрегаты.</p>
        <p>Уроки, вопросы и объяснения сгенерированы языковыми моделями и выборочно проверены; в них могут быть ошибки. Формулировки вопросов с собесов пересказаны своими словами, без цитат кандидатов, названий компаний и ссылок на записи. Проект — независимый, не связан с Math Academy и её авторами; книга «The Math Academy Way» упоминается как источник идеи алгоритма, её текст и материалы платформы не используются. Названия компаний и продуктов принадлежат их владельцам.</p>

        <h2 id="selfhost">Свой сервер</h2>
        <pre>{`git clone https://github.com/billyhargroveofficial/ml-lab && cd ml-lab
python3 tools/build_content.py           # data/source/*.json → data/content.db
(cd web && npm ci && npm run build)      # React → web/dist
(cd server && go build -o ../bin/ml-lab .)
./bin/ml-lab -addr 127.0.0.1:17490       # http://127.0.0.1:17490/ml/`}</pre>
        <p>Прогресс учеников — в <code>data/users.db</code> (SQLite). Пароли хранятся как PBKDF2-SHA256, сессии — как SHA-256 от токена.</p>
        {info.data && <p className="muted" style={{ fontSize: 11, wordBreak: 'break-all' }}>Исходники этой сборки: {Object.entries(info.data.info.sources).map(([k, v]) => `${k} ${v.slice(0, 12)}`).join(' · ')}</p>}
      </div>
    </div>
  </div>;
}
