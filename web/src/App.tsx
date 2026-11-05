import { createContext, useContext, useEffect, useState, type AnchorHTMLAttributes, type MouseEvent, type ReactNode } from 'react';
import { api, refresh, useApi, type Me } from './api';
import { Dashboard } from './pages/Dashboard';
import { Docs } from './pages/Docs';
import { StatsPage } from './pages/Stats';
import { ProgramPage } from './pages/Program';
import { IdeaPage } from './pages/Idea';

// ---- routing: everything lives under /ml ----
const ROOT = '/ml';
const current = () => decodeURIComponent(location.pathname.replace(/^\/ml/, '') || '/');
let setPathGlobal: (p: string) => void = () => {};
export function go(to: string) { history.pushState(null, '', ROOT + to); setPathGlobal(to); window.scrollTo(0, 0); }
export function Link({ to, children, ...rest }: { to: string; children: ReactNode } & Omit<AnchorHTMLAttributes<HTMLAnchorElement>, 'href'> & { 'data-s'?: string }) {
  return <a href={ROOT + to} {...rest} onClick={(e: MouseEvent) => { if (e.metaKey || e.ctrlKey || e.shiftKey || e.button) return; e.preventDefault(); go(to); }}>{children}</a>;
}

// ---- session ----
interface Session { me?: Me; guest: boolean; loading: boolean; logout: () => void; remove: () => void }
const SessionCtx = createContext<Session>({ guest: true, loading: true, logout: () => {}, remove: () => {} });
export const useSession = () => useContext(SessionCtx);

function useTheme() {
  const [theme, setTheme] = useState<string | null>(() => { try { return localStorage.getItem('theme'); } catch { return null; } });
  useEffect(() => { if (theme) document.documentElement.dataset.theme = theme; else delete document.documentElement.dataset.theme; }, [theme]);
  const dark = theme ? theme === 'dark' : matchMedia('(prefers-color-scheme: dark)').matches;
  const toggle = () => { const next = dark ? 'light' : 'dark'; setTheme(next); try { localStorage.setItem('theme', next); } catch { /* per-viewer convenience */ } };
  return { dark, toggle };
}

export function App() {
  const [path, setPath] = useState(current);
  setPathGlobal = setPath;
  useEffect(() => { const fn = () => setPath(current()); addEventListener('popstate', fn); return () => removeEventListener('popstate', fn); }, []);
  const me = useApi<Me>('/me?track=all');
  const guest = me.error?.status === 401 || (!me.data && !me.loading);
  const session: Session = {
    me: guest ? undefined : me.data, guest, loading: me.loading && !me.data,
    logout: () => { void api('/auth/logout', {}).finally(() => refresh(true)); },
    remove: () => {
      const name = me.data?.user;
      const password = name ? window.prompt(`Удалить аккаунт @${name} вместе со всем прогрессом? Это необратимо. Введите пароль:`) : null;
      if (!name || !password) return;
      api('/auth/delete', { name, password }).then(() => refresh(true)).catch(e => alert((e as Error).message));
    },
  };
  const { dark, toggle } = useTheme();
  useEffect(() => {
    const titles: Record<string, string> = { '/docs': 'API · ML Lab', '/stats': 'Статистика · ML Lab', '/program': 'Программа · ML Lab' };
    document.title = titles[path] ?? 'ML Lab';
  }, [path]);

  let page: ReactNode;
  if (path === '/' || path === '') page = <Dashboard />;
  else if (path === '/docs') page = <Docs />;
  else if (path === '/stats') page = <StatsPage />;
  else if (path === '/program') page = <ProgramPage />;
  else if (path.startsWith('/idea/')) page = <IdeaPage code={path.slice(6)} />;
  else page = <div className="wrap page"><h1>404</h1><p className="muted">Такой страницы нет. <Link to="/">На главную</Link></p></div>;
  const nav = [['/', 'учиться'], ['/program', 'программа'], ['/stats', 'статистика'], ['/docs', 'api']] as const;

  return <SessionCtx.Provider value={session}>
    <header className="top"><div className="wrap">
      <Link to="/" className="brand"><i>ml</i>ML Lab</Link>
      <nav className="nav">{nav.map(([to, label]) => <Link key={to} to={to} aria-current={path === to ? 'page' : undefined}>{label}</Link>)}</nav>
      <span className="spacer" />
      {session.me && <span className="who"><span className="name muted">@{session.me.user}</span><button className="btn small ghost" onClick={session.logout}>выйти</button></span>}
      <button className="iconbtn" aria-label={dark ? 'Светлая тема' : 'Тёмная тема'} title={dark ? 'Светлая тема' : 'Тёмная тема'} onClick={toggle}>{dark ? '☀' : '☾'}</button>
    </div></header>
    <main>{page}</main>
    <footer className="foot"><div className="wrap">
      <span>ML Lab — открытая программа подготовки к собесам DS/ML/AI. Не связан с Math Academy.{session.me && <> · <button className="btn small ghost" style={{ height: 'auto', padding: 0, color: 'inherit' }} onClick={session.remove}>удалить аккаунт</button></>}</span>
      <span><a href="https://github.com/billyhargroveofficial/ml-lab">github</a> · <Link to="/docs">api</Link> · <a href="/ml/data/content.db">content.db</a> · контент CC BY 4.0, код MIT</span>
    </div></footer>
  </SessionCtx.Provider>;
}
