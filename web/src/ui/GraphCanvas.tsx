// Knowledge graph on a canvas (PixiJS, render on demand, d3-zoom for pan and
// zoom), the same approach as the roadmap scene in nareshka. The whole graph
// is laid out at once: columns go from simple to complex along `requires`.
// Clicking an idea lights up every path from it with weights: down to what it
// stands on (credit flows there), up to what is built on it.
import { Application, BitmapText, Container, Graphics } from 'pixi.js';
import { select } from 'd3-selection';
import { zoom, zoomIdentity, type D3ZoomEvent, type ZoomBehavior } from 'd3-zoom';
import { useEffect, useRef, useState } from 'react';

export interface GNode { code: string; name: string; program: string; sectionName: string; alt: boolean }
export interface GEdge { from: string; code: string; encompassing: number; kind: 'prerequisite' | 'implicit'; key?: boolean }
export interface GData { nodes: Map<string, GNode>; out: Map<string, GEdge[]>; inc: Map<string, GEdge[]> }
export interface Walk { w: Map<string, number>; via: Map<string, string> }

// Vertical: rows go from simple (top) to complex (bottom).
const W = 210, H = 42, SX = 232, SY = 150;

function palette() {
  const s = getComputedStyle(document.documentElement);
  const c = (v: string, d: string) => (s.getPropertyValue(v).trim() || d);
  return { bg: c('--bg', '#fff'), panel: c('--panel', '#fff'), fg: c('--fg', '#111'), muted: c('--muted', '#888'), line: c('--line', '#ddd'), strong: c('--line-strong', '#aaa'), accent: c('--accent', '#d9480f'), alt: c('--ds', '#1971c2'), uses: c('--ml', '#9c36b5'), font: c('--font', 'monospace') };
}

// Rows by the longest requires-chain below an idea, then two barycenter
// sweeps so linked ideas sit at similar horizontal positions.
function layout(g: GData) {
  const layer = new Map<string, number>();
  const depth = (c: string, seen: Set<string>): number => {
    if (layer.has(c)) return layer.get(c)!;
    if (seen.has(c)) return 0;
    seen.add(c);
    let d = 0;
    for (const e of g.out.get(c) ?? []) if (e.kind === 'prerequisite' && g.nodes.has(e.code)) d = Math.max(d, depth(e.code, seen) + 1);
    layer.set(c, d);
    return d;
  };
  for (const c of g.nodes.keys()) depth(c, new Set());
  const cols: string[][] = [];
  for (const c of g.nodes.keys()) (cols[layer.get(c)!] ??= []).push(c);
  const pos = new Map<string, { x: number; y: number }>();
  const place = () => cols.forEach((col, i) => col.forEach((c, j) => pos.set(c, { x: j * SX - (col.length * SX) / 2, y: i * SY })));
  place();
  for (let pass = 0; pass < 2; pass++) {
    for (let i = 1; i < cols.length; i++) {
      const bary = (c: string) => { const xs = (g.out.get(c) ?? []).map(e => pos.get(e.code)?.x).filter((x): x is number => x !== undefined); return xs.length ? xs.reduce((a, b) => a + b, 0) / xs.length : pos.get(c)!.x; };
      const keyed = cols[i].map((c, j) => ({ c, b: bary(c), j }));
      keyed.sort((a, b) => a.b - b.b || a.j - b.j);
      cols[i] = keyed.map(k => k.c);
      place();
    }
  }
  return pos;
}

const short = (s: string, n = 28) => (s.length > n ? s.slice(0, n - 1) + '…' : s);

export function GraphCanvas({ g, code, down, up, focus, onPick }: { g: GData; code: string; down: Walk; up: Walk; focus: number; onPick: (c: string) => void }) {
  const host = useRef<HTMLDivElement>(null);
  const st = useRef<{ app?: Application; scene?: Container; zb?: ZoomBehavior<HTMLCanvasElement, unknown>; nodes: Map<string, { box: Container; bg: Graphics }>; pos?: Map<string, { x: number; y: number }>; hi?: Container; base?: Graphics; ready: boolean }>({ nodes: new Map(), ready: false });
  const [theme, setTheme] = useState(0);
  const [built, setBuilt] = useState(0);
  const [tip, setTip] = useState<{ x: number; y: number; n: GNode } | null>(null);
  const pick = useRef(onPick);
  pick.current = onPick;

  // Theme or font change: rebuild with the new colours.
  useEffect(() => {
    const mo = new MutationObserver(() => setTheme(t => t + 1));
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme', 'data-font'] });
    const mq = matchMedia('(prefers-color-scheme: dark)');
    const fn = () => setTheme(t => t + 1);
    mq.addEventListener('change', fn);
    return () => { mo.disconnect(); mq.removeEventListener('change', fn); };
  }, []);

  // The Pixi application lives as long as the component.
  useEffect(() => {
    let dead = false;
    const el = host.current!;
    const app = new Application();
    const ro = new ResizeObserver(() => { const r = el.getBoundingClientRect(); if (st.current.ready) { app.renderer.resize(Math.max(1, r.width), Math.max(1, r.height)); app.render(); } });
    (async () => {
      const r = el.getBoundingClientRect();
      await app.init({ width: Math.max(1, r.width), height: Math.max(1, r.height), antialias: true, autoStart: false, autoDensity: true, backgroundAlpha: 0, resolution: devicePixelRatio || 1, preference: 'webgl' });
      if (dead) { app.destroy(true); return; }
      el.replaceChildren(app.canvas);
      Object.assign(app.canvas.style, { width: '100%', height: '100%', display: 'block', touchAction: 'none' });
      const scene = new Container();
      app.stage.addChild(scene);
      const zb = zoom<HTMLCanvasElement, unknown>().scaleExtent([0.05, 2.5])
        .filter((e: Event) => e.type === 'wheel' || e.type.startsWith('touch') || ((e as MouseEvent).button === 1 && e.type !== 'dblclick'))
        .on('zoom', (e: D3ZoomEvent<HTMLCanvasElement, unknown>) => { scene.position.set(e.transform.x, e.transform.y); scene.scale.set(e.transform.k); setTip(null); app.render(); });
      select(app.canvas).call(zb).on('dblclick.zoom', null);
      // Middle button pans; stop the browser's autoscroll on it.
      app.canvas.addEventListener('mousedown', e => { if (e.button === 1) e.preventDefault(); });
      app.canvas.addEventListener('auxclick', e => { if (e.button === 1) e.preventDefault(); });
      Object.assign(st.current, { app, scene, zb, ready: true });
      ro.observe(el);
      setBuilt(b => b + 1);
    })();
    return () => { dead = true; ro.disconnect(); st.current.ready = false; try { app.destroy(true, { children: true }); } catch { /* not initialised yet */ } };
  }, []);

  // Build the scene for this graph.
  useEffect(() => {
    const s = st.current;
    if (!s.ready || !s.app || !s.scene) return;
    const p = palette();
    s.scene.removeChildren().forEach(c => c.destroy({ children: true }));
    s.nodes.clear();
    const pos = layout(g);
    s.pos = pos;
    const base = new Graphics();
    for (const list of g.out.values()) for (const e of list) {
      const a = pos.get(e.from), b = pos.get(e.code);
      if (!a || !b) continue;
      curve(base, a, b);
      base.stroke({ width: 1, color: e.kind === 'implicit' ? p.uses : p.strong, alpha: 0.22 });
    }
    s.base = base;
    s.scene.addChild(base);
    const hi = new Container();
    s.hi = hi;
    const nodesLayer = new Container();
    s.scene.addChild(nodesLayer, hi);
    for (const n of g.nodes.values()) {
      const at = pos.get(n.code)!;
      const box = new Container();
      box.position.set(at.x, at.y);
      const bg = new Graphics().roundRect(0, 0, W, H, 6).fill({ color: p.panel }).stroke({ width: 1, color: n.alt ? p.alt : p.strong });
      const label = new BitmapText({ text: short(n.name), style: { fontFamily: 'Inter Variable, sans-serif', fontSize: 13, fill: p.fg } });
      label.position.set(10, 6);
      const sub = new BitmapText({ text: short(n.sectionName, 34), style: { fontFamily: 'Inter Variable, sans-serif', fontSize: 10, fill: p.muted } });
      sub.position.set(10, 24);
      box.addChild(bg, label, sub);
      box.eventMode = 'static';
      box.cursor = 'pointer';
      box.on('pointertap', () => pick.current(n.code));
      box.on('pointerover', ev => { const r = host.current!.getBoundingClientRect(); setTip({ x: ev.client.x - r.left, y: ev.client.y - r.top, n }); });
      box.on('pointerout', () => setTip(null));
      nodesLayer.addChild(box);
      s.nodes.set(n.code, { box, bg });
    }
    s.app.render();
    setBuilt(b => b + 1);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [g, theme, built === 0 ? 0 : 1]);

  // Highlight the selected idea and its paths.
  useEffect(() => {
    const s = st.current;
    if (!s.ready || !s.app || !s.hi || !s.pos) return;
    const p = palette();
    s.hi.removeChildren().forEach(c => c.destroy({ children: true }));
    const lit = new Set([code, ...down.w.keys(), ...up.w.keys()]);
    for (const [c, nd] of s.nodes) nd.box.alpha = lit.has(c) ? 1 : 0.16;
    if (s.base) s.base.alpha = 0.45;
    const lines = new Graphics();
    s.hi.addChild(lines);
    const draw = (walk: Walk, color: string) => {
      for (const [c, via] of walk.via) {
        const a = s.pos!.get(c), b = s.pos!.get(via);
        if (!a || !b) continue;
        const e = [...(g.out.get(via) ?? []), ...(g.out.get(c) ?? [])].find(x => (x.from === via && x.code === c) || (x.from === c && x.code === via));
        const w = e?.encompassing ?? 0;
        const m = curve(lines, a, b);
        lines.stroke({ width: 1 + w * 4, color: e?.kind === 'implicit' ? p.uses : color, alpha: 0.9 });
        const mx = m.x, my = m.y;
        const t = new BitmapText({ text: w.toFixed(2), style: { fontFamily: 'Inter Variable, sans-serif', fontSize: 11, fill: color } });
        const pad = new Graphics().roundRect(mx - 16, my - 9, 32, 18, 4).fill({ color: p.bg }).stroke({ width: 1, color, alpha: 0.6 });
        t.anchor.set(0.5);
        t.position.set(mx, my);
        s.hi!.addChild(pad, t);
      }
      for (const [c, w] of walk.w) {
        const at = s.pos!.get(c);
        if (!at) continue;
        const badge = new Graphics().roundRect(at.x + W - 50, at.y - 9, 50, 18, 9).fill({ color });
        const t = new BitmapText({ text: `путь ${w.toFixed(2)}`, style: { fontFamily: 'Inter Variable, sans-serif', fontSize: 10, fill: p.bg } });
        t.anchor.set(0.5);
        t.position.set(at.x + W - 25, at.y);
        s.hi!.addChild(badge, t);
      }
    };
    draw(down, p.accent);
    draw(up, p.alt);
    const at = s.pos.get(code);
    if (at) s.hi.addChild(new Graphics().roundRect(at.x - 3, at.y - 3, W + 6, H + 6, 8).stroke({ width: 2.5, color: p.fg }));
    s.app.render();
  }, [code, down, up, g, built, theme]);

  // Bring the selected idea into view: fit the whole graph first time, then
  // glide to the idea whenever it is chosen from outside the canvas.
  useEffect(() => {
    const s = st.current;
    if (!s.ready || !s.app || !s.zb || !s.pos) return;
    const at = s.pos.get(code);
    if (!at) return;
    const cw = s.app.renderer.width / (s.app.renderer.resolution || 1), ch = s.app.renderer.height / (s.app.renderer.resolution || 1);
    const k = 0.9;
    const target = zoomIdentity.translate(cw / 2 - (at.x + W / 2) * k, ch / 2 - (at.y + H / 2) * k).scale(k);
    const sel = select(s.app.canvas);
    const from = { x: s.scene!.position.x, y: s.scene!.position.y, k: s.scene!.scale.x };
    const t0 = performance.now();
    const step = (t: number) => {
      const u = Math.min(1, (t - t0) / 380), e = 1 - Math.pow(1 - u, 3);
      const k2 = from.k + (target.k - from.k) * e;
      s.zb!.transform(sel, zoomIdentity.translate(from.x + (target.x - from.x) * e, from.y + (target.y - from.y) * e).scale(k2));
      if (u < 1) requestAnimationFrame(step);
    };
    requestAnimationFrame(step);
  }, [focus, built]);

  return <div className="gcanvas">
    <div ref={host} className="gcanvas-host" />
    {tip && <div className="gtip" style={{ left: tip.x + 14, top: tip.y + 14 }}><b>{tip.n.name}</b><small>{tip.n.sectionName} · {tip.n.program}</small></div>}
  </div>;
}

// Edges run from the bottom of the simpler idea to the top of the harder
// one; an edge that skips rows bends sideways so it does not cut through the
// boxes in between. Returns the midpoint (for the weight label).
function curve(gfx: Graphics, a: { x: number; y: number }, b: { x: number; y: number }) {
  const [u, v] = a.y <= b.y ? [a, b] : [b, a];
  if (u.y === v.y) {
    const y = u.y + H, x1 = a.x + W / 2, x2 = b.x + W / 2;
    gfx.moveTo(x1, y).bezierCurveTo(x1, y + 60, x2, y + 60, x2, y);
    return { x: (x1 + x2) / 2, y: y + 45 };
  }
  const x1 = u.x + W / 2, y1 = u.y + H, x2 = v.x + W / 2, y2 = v.y;
  const span = Math.round((v.y - u.y) / SY);
  const bend = span > 1 ? Math.min(260, 40 * span) * (x2 >= x1 ? 1 : -1) : 0;
  const dy = (y2 - y1) * 0.45;
  gfx.moveTo(x1, y1).bezierCurveTo(x1 + bend, y1 + dy, x2 + bend, y2 - dy, x2, y2);
  return { x: (x1 + 3 * (x1 + bend) + 3 * (x2 + bend) + x2) / 8, y: (y1 + 3 * (y1 + dy) + 3 * (y2 - dy) + y2) / 8 };
}
