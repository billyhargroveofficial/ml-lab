import { createContext, useContext, useEffect, useState, type AnchorHTMLAttributes, type MouseEvent, type ReactNode } from 'react';
import { useApi, type Info, type Me, type PackInfo } from './api';
import { AnimatePresence, MotionConfig, motion } from 'framer-motion';
import { Dashboard } from './pages/Dashboard';
import { Docs } from './pages/Docs';
import { PacksPage } from './pages/Packs';
import { ProgramPage } from './pages/Program';
import { IdeaPage } from './pages/Idea';
import { GraphPage } from './pages/Graph';

// ---- routing ----
const current = () => decodeURIComponent(location.pathname || '/');
let setPathGlobal: (p: string) => void = () => {};
export function go(to: string) { history.pushState(null, '', to); setPathGlobal(to.split(/[?#]/)[0]); window.scrollTo(0, 0); }
export function Link({ to, children, ...rest }: { to: string; children: ReactNode } & Omit<AnchorHTMLAttributes<HTMLAnchorElement>, 'href'> & { 'data-s'?: string }) {
  return <a href={to} {...rest} onClick={(e: MouseEvent) => { if (e.metaKey || e.ctrlKey || e.shiftKey || e.button) return; e.preventDefault(); go(to); }}>{children}</a>;
}

// ---- the local learner and the loaded packs ----
// There are no accounts: progress belongs to whoever runs the app.
interface Session { me?: Me; guest: false; loading: boolean; packs: PackInfo[] }
const SessionCtx = createContext<Session>({ guest: false, loading: true, packs: [] });
export const useSession = () => useContext(SessionCtx);
export const packTitle = (packs: PackInfo[], id: string) => packs.find(p => p.id === id)?.title ?? id;

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
  const info = useApi<Info>('/info');
  const session: Session = { me: me.data, guest: false, loading: me.loading && !me.data, packs: info.data?.packs ?? [] };
  const { dark, toggle } = useTheme();
  useEffect(() => {
    const titles: Record<string, string> = { '/docs': 'Как это работает · FIRe Lab', '/packs': 'Пакеты · FIRe Lab', '/program': 'Программа · FIRe Lab', '/graph': 'Граф знаний · FIRe Lab' };
    document.title = titles[path] ?? 'FIRe Lab';
  }, [path]);

  let page: ReactNode;
  if (path === '/' || path === '') page = <Dashboard />;
  else if (path === '/docs') page = <Docs />;
  else if (path === '/packs') page = <PacksPage />;
  else if (path === '/program') page = <ProgramPage />;
  else if (path === '/graph') page = <GraphPage />;
  else if (path.startsWith('/idea/')) page = <IdeaPage code={path.slice(6)} />;
  else page = <div className="wrap page"><h1>404</h1><p className="muted">Такой страницы нет. <Link to="/">На главную</Link></p></div>;
  const nav = [['/', 'учиться'], ['/program', 'программа'], ['/graph', 'граф знаний'], ['/packs', 'пакеты'], ['/docs', 'как это работает']] as const;

  return <MotionConfig reducedMotion="user"><SessionCtx.Provider value={session}>
    <header className="top"><div className="wrap">
      <nav className="nav">{nav.map(([to, label]) => <Link key={to} to={to} aria-current={path === to ? 'page' : undefined}>{path === to && <motion.span layoutId="nav-pill" className="nav-pill" transition={{ type: 'spring', stiffness: 420, damping: 36 }} />}<span className="seg-label">{label}</span></Link>)}</nav>
      <span className="spacer" />
      <motion.button whileTap={{ scale: 0.9 }} className="iconbtn" aria-label={dark ? 'Светлая тема' : 'Тёмная тема'} title={dark ? 'Светлая тема' : 'Тёмная тема'} onClick={toggle}><AnimatePresence mode="wait" initial={false}><motion.span key={String(dark)} initial={{ rotate: -90, opacity: 0 }} animate={{ rotate: 0, opacity: 1 }} exit={{ rotate: 90, opacity: 0 }} transition={{ duration: 0.18 }}>{dark ? '☀' : '☾'}</motion.span></AnimatePresence></motion.button>
    </div></header>
    <main><AnimatePresence mode="wait" initial={false}><motion.div key={path} initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -6 }} transition={{ type: 'spring', stiffness: 300, damping: 32, opacity: { duration: 0.16 } }}>{page}</motion.div></AnimatePresence></main>
    <footer className="foot"><div className="wrap">
      <span>FIRe Lab — интервальное повторение по графу знаний, на ваших собственных пакетах. Прогресс хранится только на этом компьютере.</span>
      <span><a href="https://github.com/billyhargroveofficial/ml-lab">github</a> · код MIT</span>
    </div></footer>
  </SessionCtx.Provider></MotionConfig>;
}
