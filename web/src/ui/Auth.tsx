import { useState, type FormEvent } from 'react';
import { api, refresh } from '../api';
import { AnimatePresence, motion, Seg } from './motion';

// Registration is a nickname and a password: no email, no confirmation.
export function Auth() {
  const [mode, setMode] = useState<'register' | 'login'>('register');
  const [name, setName] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true); setError('');
    try { await api(`/auth/${mode}`, { name, password }); refresh(true); } catch (err) { setError((err as Error).message); } finally { setBusy(false); }
  };
  return <form className="auth" onSubmit={submit}>
    <Seg label="Вход или регистрация" value={mode} items={[['register', 'регистрация'], ['login', 'вход']]} onChange={setMode} />
    <input type="text" placeholder="ник" autoComplete="username" value={name} onChange={e => setName(e.target.value)} maxLength={24} required aria-label="Ник" />
    <input type="password" placeholder={mode === 'register' ? 'пароль, от 6 символов' : 'пароль'} autoComplete={mode === 'register' ? 'new-password' : 'current-password'} value={password} onChange={e => setPassword(e.target.value)} required aria-label="Пароль" />
    <motion.button whileTap={{ scale: 0.98 }} className="btn primary" disabled={busy}>{mode === 'register' ? 'Начать учиться' : 'Войти'}</motion.button>
    <span className="err" role="alert"><AnimatePresence mode="wait">{error && <motion.span key={error} style={{ display: 'inline-block' }} initial={{ opacity: 0, x: -6 }} animate={{ opacity: 1, x: [0, -4, 4, 0] }} exit={{ opacity: 0 }} transition={{ duration: 0.3 }}>{error}</motion.span>}</AnimatePresence></span>
    <small>Храним только ник, хэш пароля и ваш прогресс. Ник виден в топе — не используйте настоящее имя. Аккаунт можно удалить в любой момент.</small>
  </form>;
}
