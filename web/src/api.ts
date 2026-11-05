import { useEffect, useState } from 'react';

export const BASE = '/ml/api/v1';

export type Track = 'all' | 'math';
export type Kind = 'lesson' | 'review' | 'quiz';

export interface MemoryView { code: string; status: 'new' | 'partial' | 'learned'; rep: number; memory_now: number; due_at: string | null }
export interface Forecast { remaining_xp: number; days: number; eta: string }
export interface Me {
  user: string; track: Track; total: number; learned: number; due: number; progress: number;
  xp: number; today_xp: number; xp_to_quiz: number; days: { date: string; xp: number }[]; ideas: MemoryView[];
  forecast: { all: Forecast };
}
export interface Task { kind: Kind; code?: string; title: string; topic?: string; level?: number; xp: number; due_at?: string; covers?: string[]; goal?: string; goal_code?: string; goal_left?: number; goal_kind?: 'idea' | 'topic' }
export interface PublicQuestion { position: number; code: string; title: string; question: string; options: string[] }
export interface Attempt { id: number; kind: Kind; code?: string; title: string; lesson?: string; partial?: boolean; total: number; question: PublicQuestion }
export interface AttemptResult { correct: number; total: number; xp: number; passed: boolean; perfect: boolean; implicit: { code: string; title: string; delta: number }[] }
export interface AnswerView { correct: boolean; correct_index: number; explanation?: string; done: boolean; next?: PublicQuestion; result?: AttemptResult }
export interface HistoryItem { kind: Kind; code?: string; title: string; finished_at: string; correct: number; total: number; xp: number; passed: boolean }
export interface Leader { rank: number; name: string; xp: number; week_xp: number; learned_ml: number; learned_math: number; me?: boolean }

export interface Topic { id: string; section_id: string; code?: string; name: string; definition?: string; ideas: string[] }
export interface Section { id: string; program: string; name: string; description?: string; topics: Topic[] }
export interface ProgramIdea { code: string; name: string; level: number; weight: number }
export interface Program { program: 'ml' | 'math'; sections: Section[]; ideas: Record<string, ProgramIdea> }

export interface Edge { code: string; key: boolean; encompassing: number; kind: 'prerequisite' | 'implicit'; source: string }
export interface Idea {
  code: string; id: string; program: 'ml' | 'math'; topic_id: string; name: string; definition?: string; level: number; weight: number;
  lesson?: string;
  questions?: { question: string; options: string[]; correct: number; explanation?: string }[];
  prerequisites: Edge[]; encompassed_by: Edge[];
}

export interface Stats {
  generated_at: string; users: number; active_7d: number; active_30d: number; attempts: number; lessons_passed: number; xp: number;
  daily: { date: string; learners: number; attempts: number; xp: number }[];
  learned_histogram: { label: string; count: number }[];
  sections: { program: string; name: string; attempts: number; pass_rate: number }[];
  hardest_ideas: { code: string; name: string; attempts: number; pass_rate: number }[];
  implicit: { explicit_reviews: number; implicit_credits: number }; min_sample: number;
}

export class ApiError extends Error { constructor(public status: number, message: string) { super(message); } }

export async function api<T>(path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const res = await fetch(BASE + path, {
    method: body === undefined ? 'GET' : 'POST',
    headers: body === undefined ? {} : { 'Content-Type': 'application/json', 'X-Requested-With': 'mllab' },
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: 'same-origin',
    signal,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(res.status, (data as { error?: string }).error ?? res.statusText);
  return data as T;
}

// A tiny query layer: hooks keep the last data while refetching, and every
// hook refetches when refresh() bumps the epoch (after answers, login, logout).
let epoch = 0;
const listeners = new Set<() => void>();
const cache = new Map<string, unknown>();
export function refresh(clear = false) { if (clear) cache.clear(); epoch++; listeners.forEach(fn => fn()); }

export function useApi<T>(path: string | null): { data?: T; error?: ApiError; loading: boolean } {
  const [ep, setEp] = useState(epoch);
  useEffect(() => { const fn = () => setEp(epoch); listeners.add(fn); return () => { listeners.delete(fn); }; }, []);
  const [data, setData] = useState<T | undefined>(() => (path ? (cache.get(path) as T | undefined) : undefined));
  const [error, setError] = useState<ApiError>();
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    if (!path) { setData(undefined); setLoading(false); return; }
    const ctrl = new AbortController();
    if (cache.has(path)) setData(cache.get(path) as T);
    setLoading(true);
    api<T>(path, undefined, ctrl.signal)
      .then(d => { cache.set(path, d); setData(d); setError(undefined); setLoading(false); })
      .catch(e => { if (!ctrl.signal.aborted) { setError(e); setLoading(false); } });
    return () => ctrl.abort();
  }, [path, ep]);
  return { data: path ? data : undefined, error, loading };
}
