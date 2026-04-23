# codemap — Docs Style Guide

The visual system for every codemap doc page. Terminal aesthetic: monospaced, neutral greyscale body, phosphor accents. Pages built against this guide feel like one continuous piece of software, not a mixed bag of marketing.

---

## 1. Principles

1. **Monospace everywhere.** One font family, one feel.
2. **Neutral by default; colour signals meaning.** Body text is cool grey. Colour lands on the things that need to signal — primitives, build stages, prompts, metrics.
3. **Every heading reads like a shell line.** `>` prefix for section labels, `$ ` for CTAs.
4. **Structure carries the page.** Boxes, rules, and grids do the heavy lifting; gradients and glow are garnish.
5. **No italics for emphasis.** Use weight (500/600) or colour.
6. **The page is a terminal.** Scanlines and a subtle CRT vignette sit over everything.

---

## 2. Tokens

All tokens live on `:root`.

### 2.1 Colour

| Token               | Value                          | Use                                               |
|---------------------|--------------------------------|---------------------------------------------------|
| `--bg`              | `#0a0f0d`                      | Page background                                   |
| `--bg-surface`      | `#12181a`                      | Cards, callouts, code blocks                      |
| `--bg-surface-2`    | `#1a2022`                      | Hover / active elevated surface                   |
| `--text`            | `#c9d1c9`                      | Body text                                         |
| `--text-muted`      | `#6e7a73`                      | Labels, captions, secondary                       |
| `--text-bright`     | `#eaf3ea`                      | Headings, `<strong>`, foreground highlights       |
| `--green`           | `#5fd38c`                      | Primary — build, prompts, brand dot, agent card   |
| `--amber`           | `#ffb347`                      | Discovery / stages / metrics / warnings           |
| `--cyan`            | `#6ec9d7`                      | Select / system map / informational               |
| `--magenta`         | `#d98bb0`                      | MCP / integration / tertiary accents              |
| `--red`             | `#e07a5f`                      | Errors only                                       |
| `--border`          | `rgba(95, 211, 140, 0.10)`     | Subtle divider                                    |
| `--border-strong`   | `rgba(95, 211, 140, 0.24)`     | Card border, section rule                         |

**Accent assignment by concept (for codemap):**
- `--green` — codemap build, indexing, the "core" primitive
- `--cyan`  — codemap select, auto-context, information flow
- `--magenta` — MCP, external integrations
- `--amber` — metrics, build stages, warnings

### 2.2 Typography

One family: **IBM Plex Mono** via Google Fonts.

```html
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@300;400;500;600;700&display=swap" rel="stylesheet">
```

| Role                  | Size                      | Weight | Notes                                     |
|-----------------------|---------------------------|--------|-------------------------------------------|
| Body                  | `1rem`                    | 400    | Line-height 1.65                          |
| Masthead H1           | `clamp(1.9rem, 4vw, 3rem)`| 600    | Trailing blinking cursor                  |
| Section label         | `1.05rem`                 | 600    | Prefixed with `>`                         |
| Card title            | `1rem`                    | 600    |                                           |
| Metric value          | `1.8rem`                  | 600    | `--amber`                                 |
| Eyebrow / label / tag | `0.68rem`                 | 500    | UPPERCASE, `letter-spacing: 0.14em`       |
| Meta / caption        | `0.7rem`                  | 400    | `--text-muted`                            |
| Code                  | `0.82em`                  | 400    | `--bg-surface` bg, thin border            |

**Base:** `html { font-size: 17.5px; }` (drops to 16px under 560px).

### 2.3 Spacing & layout

| Token             | Value                    | Use                          |
|-------------------|--------------------------|------------------------------|
| `--max-width`     | `58rem`                  | Shell / nav / footer width   |
| Shell padding     | `3rem 1.5rem 5rem`       | Top/side/bottom              |
| Section gap       | `3.5rem`                 | Between `<section>`          |
| Card gap          | `1rem`                   | Inter-card grid              |
| Pipeline gap      | `0.5rem`                 | Six-up build-pass strip      |
| Radius            | `0.15–0.4rem`            | Keep crisp; never above 0.4  |

---

## 3. Global motifs

These belong on every codemap page.

### 3.1 CRT scanlines + vignette

```css
body::before {
  content: ''; position: fixed; inset: 0; pointer-events: none;
  background: repeating-linear-gradient(to bottom,
    rgba(255,255,255,0) 0, rgba(255,255,255,0) 2px,
    rgba(255,255,255,0.015) 3px, rgba(255,255,255,0) 4px);
  z-index: 100;
}
body::after {
  content: ''; position: fixed; inset: 0; pointer-events: none;
  background: radial-gradient(ellipse at center, transparent 60%, rgba(0,0,0,0.4) 120%);
  z-index: 99;
}
```

### 3.2 Phosphor dot

```css
.dot {
  display: inline-block; width: 8px; height: 8px;
  background: var(--green); box-shadow: 0 0 8px var(--green);
  animation: phosphor 1.8s ease-in-out infinite;
}
@keyframes phosphor { 0%, 100% { opacity: 1; } 50% { opacity: 0.55; } }
```

### 3.3 Blinking cursor

```css
.cursor {
  display: inline-block; width: 0.55em; height: 1em;
  background: var(--green); box-shadow: 0 0 12px var(--green);
  vertical-align: -0.12em; margin-left: 0.1em;
  animation: blink 1.1s step-end infinite;
}
@keyframes blink { 0%, 49% { opacity: 1; } 50%, 100% { opacity: 0; } }
```

Append to H1: `<h1>Title<span class="cursor"></span></h1>`.

### 3.4 Shell prompts

- Section labels: `>` prefix in `--green`.
- CTA buttons: `$ ` prefix in `--green`.
- Example setups: leading `$ ` in green for commands.

---

## 4. Components

### 4.1 Site nav

```html
<nav class="site-nav">
  <div class="wrap">
    <a class="brand" href="#top">
      <span class="dot"></span>codemap<span class="ver">@latest</span>
    </a>
    <div class="links"><a>How</a><a>Primitives</a>…</div>
  </div>
</nav>
```

Brand = phosphor dot + name + muted version tag. Links muted, brighten on hover.

### 4.2 Masthead + CRT mockup

Two-column grid (collapses under 900px). Left: eyebrow, H1, lede, CTA, stats. Right: a CRT monitor mockup showing a real `codemap build` / `codemap select` run.

```html
<section class="masthead">
  <div class="mh-left">
    <div class="mh-eyebrow">
      <span class="dot"></span>codemap · Repo intelligence for AI agents
      <span class="sep"></span><span>Go · MCP</span>
    </div>
    <h1>Code map your repo<span class="cursor"></span></h1>
    <p class="dek">One-paragraph lede.</p>
    <div class="mh-cta"><a class="primary">quick-start</a><a>github</a></div>
    <div class="mh-stats">
      <div><b>2688</b>files indexed</div>
      <div><b>94%</b>cache hit rate</div>
    </div>
  </div>
  <div class="crt" aria-hidden="true">
    <div class="screen">…terminal output with .prompt / .ok / .dim spans…</div>
  </div>
</section>
```

The CRT mockup is essential — it's the page's icon. Keep its text a real `codemap` session (prompt, check-marks, blink cursor).

### 4.3 Section bar

Every section opens with one.

```html
<div class="section-bar">
  <div class="sb-num">01</div>
  <div class="sb-label">Label — <em>subtitle in muted</em></div>
  <div class="sb-count">meta</div>
</div>
```

- `sb-num` — green border + bg
- `sb-label` — bright, `>` prefix; `<em>` = non-italic, muted
- `sb-count` — right-aligned meta (time, count, type)

### 4.4 Card (primitive)

```html
<div class="card index">
  <div class="icon"><svg>…</svg></div>
  <span class="tag">Code map</span>
  <h3>Title</h3>
  <p>Paragraph.</p>
  <ul><li>Bullet</li></ul>
  <dl class="meta"><dt>Command</dt><dd><code>codemap build</code></dd></dl>
</div>
```

**Codemap variants:**
- `.index`  — green  (code map / build)
- `.select` — cyan   (auto-context / select)
- `.mcp`    — magenta (MCP server)

Each variant colours `.tag` + `.icon`. Max three on a row.

### 4.5 Pill

Inline labels inside prose, tables, example headers.

```html
<span class="pill index">build</span>
<span class="pill select">select</span>
<span class="pill metric">recall</span>
```

Variants: `.index`, `.select`, `.mcp`, `.metric` (amber for numbers).

### 4.6 Callout

```html
<div class="callout info"><strong>Heading.</strong> Body.</div>
```

Variants:
- default — muted left border
- `.info`  — cyan (observations, context)
- `.ok`    — green (wins, validation, success)
- `.warn`  — amber (constraints, gotchas)

### 4.7 Build pipeline strip

codemap-specific: a 6-column grid for the build passes. Use this instead of the "scaling stages" pattern from `domain-agents`.

```html
<div class="pipeline">
  <div class="pipe">
    <div class="n">Pass 1</div>
    <div class="name">Scan</div>
    <div class="desc">Walk the tree, honour ignore rules.</div>
  </div>
  …
</div>
```

Neutral boxes; no "active" state needed — every pass runs on a full build.

### 4.8 Metrics grid

codemap-specific: a 4-up grid with big amber numbers. For precision, recall, context-saved, overhead — or any observed numbers.

```html
<div class="metrics">
  <div class="metric">
    <div class="k">Precision</div>
    <div class="v">65%</div>
    <div class="d">Of selected files, how many were actually needed.</div>
  </div>
</div>
```

Big number in `--amber` ties the whole "measuring it" section together.

### 4.9 Worked example

```html
<div class="example">
  <h3><span class="pill select">task</span> <span class="sub">one-line scenario</span></h3>
  <div class="setup">Setup paragraph in mono on dark surface.</div>
  <ol class="flow">
    <li><strong>Step one</strong> — narrative sentence.</li>
  </ol>
</div>
```

Steps numbered by CSS counter into green-ringed circles.

### 4.10 Accordion

Native `<details>` / `<summary>` with mono `+ / −` indicator. Use for Key decisions and FAQ.

```html
<details>
  <summary>Heading</summary>
  <div class="body"><p>Answer body.</p></div>
</details>
```

### 4.11 KV tables

For command lists, provider tables, tool listings, interaction matrices.

```html
<table class="kv">
  <thead><tr><th>Command</th><th>Description</th></tr></thead>
  <tbody>
    <tr><td><code>codemap build</code></td><td>…</td></tr>
  </tbody>
</table>
```

All mono, uppercase header, dashed row dividers.

### 4.12 Code blocks

```html
<pre><span class="c"># comment</span>
<span class="k">key:</span> <span class="s">'string'</span></pre>
```

- `.c` — muted (comments)
- `.k` — green (commands, keys)
- `.s` — amber (strings, paths)
- `.f` — cyan (functions)

Inline: `<code class="inline">`.

### 4.13 SVG pipeline diagrams

For the "how it works" diagram only. Keep viewBox around `900 × 360`.

- Boxes: `rx="4"`, 1px border at ~50% alpha of role colour
- Label row: 10px weight 600 `letter-spacing="1.5"` in role colour
- Main string: 13–14px `--text-bright`
- Metadata rows: 9px in role colour
- Arrows: 1.5px solid in the target's role colour
- Supporting arrows: 1.2px dashed in neutral grey `#7a7a7a` with `arrDash` marker
- Grid backdrop at 4% alpha, masked to a radial fade (inherit from `.diagram::before`)

---

## 5. Do / Don't

**Do**
- Prefix every major section heading with `>` (in green)
- Use colour-coded pills when naming a primitive
- Keep every paragraph under ~44rem for scannability
- Show real metrics, not estimates
- Quote real CLI output in the CRT mockup

**Don't**
- Don't italicise for emphasis — use weight or colour
- Don't add a fourth font
- Don't stack glow effects (one per element max)
- Don't use red for anything except errors
- Don't write "seamless" or "leverage" in display copy

---

## 6. Writing voice

- Active verbs, short declarative sentences
- One idea per paragraph
- Code identifiers always in `<code>`
- Numbers with their unit inline (`27 KB`, `94%`, `~$0.50`) — no free-floating figures
- Lead with the shape, back up with the detail

---

## 7. Page scaffold

Minimum viable codemap page:

```html
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1"/>
  <title>Page — codemap</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@300;400;500;600;700&display=swap" rel="stylesheet">
  <style>/* inline the :root + component CSS from docs/index.html */</style>
</head>
<body>
  <nav class="site-nav">…</nav>
  <main class="shell">
    <section class="masthead">…</section>
    <section id="…">
      <div class="section-bar">…</div>
      <div class="prose">…</div>
    </section>
  </main>
  <footer class="site-footer">…</footer>
</body>
</html>
```

Copy the `<style>` block from `docs/index.html` verbatim. If you fork it, keep `:root` in sync.

---

## 8. Checklist for a new page

- [ ] Fonts preconnect + IBM Plex Mono loaded
- [ ] `:root` tokens copied unchanged
- [ ] `body::before` / `body::after` scanlines + vignette present
- [ ] `.site-nav` brand with phosphor dot + `@latest` version tag
- [ ] H1 ends with `<span class="cursor"></span>`
- [ ] Every major section opens with `.section-bar` with `> ` prefix
- [ ] No italics on display headings
- [ ] All inline identifiers wrapped in `<code>`
- [ ] Max content width `58rem`
- [ ] Metrics shown as observed numbers, never estimates
- [ ] Footer repeats site nav links in muted mono

That's the whole system. If you add something that doesn't fit one of the components above, add it to this guide first, then use it.
