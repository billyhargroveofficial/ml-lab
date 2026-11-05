import { useState, type FormEvent } from 'react';
import { api, refresh } from '../api';

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
    <div className="seg" role="group" aria-label="Вход или регистрация">
      <button type="button" aria-pressed={mode === 'register'} onClick={() => setMode('register')}>регистрация</button>
      <button type="button" aria-pressed={mode === 'login'} onClick={() => setMode('login')}>вход</button>
    </div>
    <input type="text" placeholder="ник" autoComplete="username" value={name} onChange={e => setName(e.target.value)} maxLength={24} required aria-label="Ник" />
    <input type="password" placeholder={mode === 'register' ? 'пароль, от 6 символов' : 'пароль'} autoComplete={mode === 'register' ? 'new-password' : 'current-password'} value={password} onChange={e => setPassword(e.target.value)} required aria-label="Пароль" />
    <button className="btn primary" disabled={busy}>{mode === 'register' ? 'Начать учиться' : 'Войти'}</button>
    <span className="err" role="alert">{error}</span>
    <small>Храним только ник, хэш пароля и ваш прогресс. Ник виден в топе — не используйте настоящее имя. Аккаунт можно удалить в любой момент.</small>
  </form>;
}
