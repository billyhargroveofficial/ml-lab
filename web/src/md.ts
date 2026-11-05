import katex from 'katex';
import { marked } from 'marked';

marked.setOptions({ gfm: true, breaks: false });

const escape = (s: string) => s.replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!);

// Math is cut out before Markdown so `_` and `*` inside formulas survive, then
// rendered by KaTeX into the placeholders.
function withMath(src: string, render: (s: string) => string): string {
  const parts: string[] = [];
  const hold = (tex: string, display: boolean) => {
    let html: string;
    try { html = katex.renderToString(tex, { displayMode: display, throwOnError: false, output: 'html' }); } catch { html = escape(tex); }
    parts.push(html);
    return `\u0000${parts.length - 1}\u0000`;
  };
  const text = src
    .replace(/\$\$([\s\S]+?)\$\$/g, (_, tex) => hold(tex, true))
    .replace(/\\\[([\s\S]+?)\\\]/g, (_, tex) => hold(tex, true))
    .replace(/\\\(([\s\S]+?)\\\)/g, (_, tex) => hold(tex, false))
    .replace(/(^|[^\\$])\$([^$\n]+?)\$/g, (_, pre, tex) => pre + hold(tex, false));
  return render(text).replace(/\u0000(\d+)\u0000/g, (_, i) => parts[Number(i)]);
}

export const md = (src: string) => withMath(src, s => marked.parse(s, { async: false }) as string);
export const mdInline = (src: string) => withMath(src, s => marked.parseInline(s, { async: false }) as string);
