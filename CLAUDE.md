# CLAUDE.md

Guidance for Claude (claude.ai, Claude Code) when working with this repository.

## Project Overview

**euromancer** is a research and publishing project by Alég Lutohin exploring the aesthetic and conceptual relationship between Eastern Bloc samizdat practices and modern CLI/terminal interfaces. The repository hosts a static website and is being developed into a self-publishing pipeline where Markdown notes written in Obsidian become pages on the site.

This is a hybrid research/publishing project, not a conventional software product. euromancer is both the aesthetic and the operating principle — work with it should prioritize terminal-based interaction (git, npm, Eleventy) over GUI tools.

Live site: https://aleglutz.github.io/euromancer/

## Stack

- **Static site generator:** Eleventy 3.1.5 (ES Modules, config uses `export default`)
- **Template language:** Nunjucks (`.njk`) for layouts and pages
- **Content:** Markdown with YAML frontmatter (equivalent to Obsidian Properties)
- **Deployment:** GitHub Actions builds and deploys on push to `main`
- **Hosting:** GitHub Pages, project site at `/euromancer/` path prefix
- **Node version:** 20 (set in `.github/workflows/deploy.yml`)

## Development Workflow

### Local preview

```sh
npx @11ty/eleventy --serve --pathprefix=/euromancer/
```

Serves on `http://localhost:8080/euromancer/` — the pathPrefix mirrors the production URL, so internal links work identically in dev and on Pages.

### Build once (no server)

```sh
npx @11ty/eleventy
```

Output goes to `_site/` (gitignored).

### Publishing a change

1. Edit `.md` or `.njk` in Obsidian / any editor
2. `git add -A && git commit -m "..." && git push`
3. GitHub Actions rebuilds `_site/` and deploys to Pages
4. Verify at https://aleglutz.github.io/euromancer/

No manual build or deploy steps — push is the publish action.

## Repository Structure

```
.
├── eleventy.config.js          # ES module, pathPrefix: "/euromancer/"
├── .eleventyignore             # README, pipeline docs, templates/, node_modules, .obsidian
├── .github/workflows/deploy.yml
├── .nojekyll                   # Tells Pages to skip Jekyll processing
├── package.json                # "type": "module", devDependency @11ty/eleventy
├── render.js                   # Playwright PNG pipeline (Instagram carousel output)
├── _includes/
│   └── post.njk                # Layout for individual posts
├── index.njk                   # Homepage
├── archive/
│   ├── index.njk               # Archive list page ("texts+images"); layout: false
│   ├── archive.json            # Directory data: applies layout: post.njk to posts
│   └── {NNNN}/                 # One folder per post, zero-padded number
│       ├── {Name}.md           # Post content with frontmatter
│       └── attachments/        # Post-specific images (optional)
├── bin/
│   └── dither.sh               # Image dithering helper
├── css/
│   └── styles.css
└── assets/
    ├── fonts/
    ├── images/
    └── js/
        └── collage-editor.js   # ?edit=1 panel for typewriting collages
```

## URL Conventions

Disk path and URL are kept parallel for clarity:

| Disk | URL |
|---|---|
| `index.njk` | `/` |
| `archive/index.njk` | `/archive/` |
| `archive/0001/CityNowhen.md` | `/archive/0001/CityNowhen/` |

No custom `permalink` is configured — Eleventy's default path-to-URL mapping is used. The numbered folder pattern (`0001/`, `0002/`) is the canonical way to identify a post on the site.

## Templates and Path Prefix

All internal links use the `url` filter, never hardcoded paths:

```njk
<link rel="stylesheet" href="{{ '/css/styles.css' | url }}">
<a href="{{ '/archive/' | url }}">archive</a>
```

The `url` filter automatically prepends `/euromancer/` from `pathPrefix` in `eleventy.config.js`. If the site ever moves to a custom domain, changing the prefix in one place updates every link.

**Important:** `.html` files are passthrough-copied without template processing. To use `{{ ... }}` filters, the file must be `.njk`.

## Data Cascade (how `layout` is resolved)

Eleventy resolves data (including `layout`) in this precedence order, high to low:

1. File's own frontmatter
2. Template-specific data file (`{filename}.json`)
3. Directory data file (`{foldername}.json`)
4. Parent directory data files
5. Global data (`_data/`)

Current layout wiring:
- `archive/archive.json` sets `layout: post.njk` for everything in `archive/`
- `archive/index.njk` overrides with `layout: false` in its own frontmatter — the archive grid page is not a post and must not be wrapped in `post.njk`

When adding new sections (e.g., `essays/`, `journal/`), follow the same pattern: one directory data file per section, one layout per content type.

## Post Frontmatter

```yaml
---
title: City Nowhen
subtitle: Discovering Seoul Memory Places through Dark Tourism with Kids
date: 2026-04-09
location: Seoul, South Korea
tags: [seoul, memory, dark-tourism]
status: ready
---
```

These fields are:
- **Editable in Obsidian as Properties** (UI for YAML frontmatter)
- **Queryable by Dataview** inside the vault
- **Accessible in Nunjucks** as `{{ title }}`, `{{ subtitle }}`, etc.
- **Eleventy-reserved keys:** `title`, `date`, `tags`, `layout`, `permalink`

## Content Model: Slides

Posts are structured as series of slides, separated by `---` with an HTML comment marking the type of each slide:

```markdown
<!-- slide:cover -->
<!-- slide:text -->
<!-- slide:image -->
<!-- slide:combo -->
<!-- slide:typewriting -->
```

Slide parsing is implemented as an Eleventy transform (`"slides"` in `eleventy.config.js`). It runs on every HTML output page, finds `<article class="slides">`, splits on `<!-- slide:type -->` markers, and wraps each chunk in `<section class="slide slide--{type}">`. `post.njk` wraps `{{ content }}` in `<article class="slides">`, so the selector is always present for post pages. The cover slide in `post.njk` renders only when `subtitle`, `tags`, or `cover_image` is present in frontmatter.

### slide:typewriting — collages

A character-grid collage after Ruth Wolf-Rehfeldt: a `<pre>` with ASCII art plus absolutely positioned images snapped to the character grid.

```html
<div class="typewriting" style="--tw-font: 'IBM_VGA_8x16'; --tw-lh: 1">
<pre>…ascii…</pre>
<img src="../attachments/file.webp" style="--col: 29; --row: 7; --w: 32">
</div>
```

- `--col`/`--w` are in `ch`, `--row` in `lh` — images move with font and line-height changes
- `--tw-font` / `--tw-lh` / `--tw-size` on the container set the monofont, line spacing, and font size (defaults live in `.typewriting` in styles.css); `.typewriting--erika` switches to Erica Type
- **Such posts need `templateEngineOverride: njk` in frontmatter** — markdown-it breaks HTML blocks at blank lines inside `<pre>` (see Gotchas)
- **Collage editor:** open the post with `?edit=1` — a panel (assets/js/collage-editor.js) adjusts font, line-height, and per-image col/row/w; images are draggable, snapped to the grid; outputs a paste-ready snippet for the `.md`

## Dual Output (planned)

The same Markdown source is intended to produce two representations:

1. **Web page** — vertical slide stream, scrolled (current site work is here)
2. **Instagram carousel** — PNG 1080×1350, rendered by Playwright against the same HTML templates

`render.js` is implemented. Usage: `node render.js /euromancer/archive/0001/CityNowhen/` — requires the dev server running on port 8080. It opens the page at 1080×1350 (@2x), locates `.slide` elements, and screenshots each to `slides/{NNNN}-{Name}/slide-01.png` etc.

**render-mode (open):** Instagram render needs larger font sizes than the web view. Planned approach: `--font-base` CSS custom property + a `.render-mode` class on `<body>` that overrides it, toggled by Playwright before screenshotting. Do not hardcode font sizes for this — use the CSS var override pattern.

## Visual System (`css/styles.css`)

Role-based design tokens on `:root` — the theme switches in one place:

- `--bg` / `--bg-warm` / `--border` — paper: warm off-white `#f5f0e6` family
- `--fg` / `--fg-dim` / `--fg-faint` — ink: `#1a1a18` family
- `--accent` / `--accent-faint` — blue ribbon `#2f6f9c`, after the RWR Kleine Galerie Wurzen poster (1989)

**Dark mode** = `.euromancer` class on `<body>`: a single block overrides the role tokens with the terminal palette (`#0d1117` bg, `#8b949e` fg). No separate `--terminal-*` token layer — the whole site is a terminal. The dark block exists in CSS; no toggle is wired yet.

Spacing scale `--space-xs` (8px) through `--space-xl` (96px). Typography: Cascadia Mono (body), Syne Mono (`h2`), IBM VGA 8x16 (`h3` and ASCII headlines), iA Writer Mono / Erica Type (typewriting collages).

Links render with terminal brackets `[link]` via `a::before/::after`; the nav path is excluded. Link hover is inverse video (accent background), like a selected item in a terminal menu — no underlines, no transitions.

Layout: one column — `header`, `main`, `footer` share `max-width: 960px`, left-anchored (TUI, not centered print), symmetric `--space-md` side padding. Headline `pre` reserves `min-height: 6em` so the nav sits at the same height on every page regardless of ASCII descenders.

TUI furniture: the nav is a Midnight Commander / lynx-style menu bar — bracketed accent links (same affordance as every other link on the site), current item in inverse video, no brackets: `[home] texts+images` on the archive, `[home] [texts+images] Triptych_A.md` on a post. One line, every element either navigates or marks "you are here". The footer is an inverse-video statusline (tmux-style). Prompt cosplay in navigation was tried and removed — hiding links inside a fake shell prompt killed discoverability (clig.dev: human-first beats clever).

### ASCII headlines (tdfiglet)

All `h1` headlines (homepage, archive, posts) are generated at build time by the `tdfiglet` filter in `eleventy.config.js`, which shells out to the [tdfiglet](https://github.com/tat3r/tdfiglet) binary (TheDraw fonts). Current font: `forgotex.tdf` (Forgotten Bl). Posts can override via `cover_font` frontmatter (a `.tdf` font name; ~600 available in `/usr/local/share/tdfiglet/fonts/`).

- Output renders in `IBM_VGA_8x16`, color `--fg-dim`
- Responsive sizing: the `maxcols` filter measures the art width, templates put it in `--cols` on the `<h1>`, CSS computes `font-size: 100cqw / (--cols × 0.5)` (0.5em = VGA advance) — any headline exactly fills the container at every viewport
- tdf fonts lack some glyphs: `+` is drawn by the filter itself (half-block art, parts joined line-by-line); `_` is silently dropped by tdfiglet in fonts that don't have it
- CI: `deploy.yml` builds tdfiglet from source on the runner; if the binary is missing the filter falls back to classic figlet (Slant) instead of failing the build

### Image Scatter Principle

Images in `.row-media` cells must never appear visually aligned or stacked. Each image in its grid cell should feel casually placed, not composed.

Rules:
- Every `.row-with-media:nth-child(N) .row-media img` must have distinctly different `margin-top`, `margin-left`, and `width`
- `margin-top` range: 8px–80px (vary by ≥30px between sections)
- `margin-left` range: 0px–40px
- `width` range: 55%–80% (never 100%, never identical between sections)
- No two consecutive sections may share the same `margin-top`
- On mobile (`@media max-width: 640px`), all scatter resets: `margin: 0`, `width: 100%`

Goal: the right column reads as a loose collection, not a grid.

## Current Roadmap

Completed:
- Eleventy + Actions + Pages pipeline
- pathPrefix-aware internal linking
- Post layout (`post.njk`) with frontmatter-driven header
- Directory-based URL structure (`archive/NNNN/`)
- Slide parsing (`<!-- slide:type -->` → `<section class="slide slide--{type}>`)
- Wikilink handling (`![[image.png]]` → `<img>` via Eleventy transform)
- `render.js` Playwright PNG pipeline for Instagram
- tdfiglet ASCII headlines on all pages (`tdfiglet` + `maxcols` filters, `--cols` sizing)
- Archive list ("texts+images") — cDc-style flat list `date - [title]` from `collections.posts`, filtered by `status: ready`
- `slide:typewriting` collages + `?edit=1` editor
- Role-based color tokens + dark-mode block (`body.euromancer`)

Active (next up):
- **render-mode font override** — `--font-base` CSS var + `.render-mode` body class for Instagram-size fonts, toggled by Playwright
- **dark-mode toggle** — `body.euromancer` block exists, needs a switch

Backlog:
- Per-slide CSS styling (cover, text, image, combo types)
- `slide:video` type — parser + template support
- `slide:combo` with background image — `<!-- bg: filename.png -->` syntax (not yet in `eleventy.config.js`)
- Auto-dithering pipeline for `attachments/` (ImageMagick, Floyd-Steinberg/Atkinson/Bayer, triggered per-post via `dither: true` frontmatter flag)
- Mini-essay translation for Instagram captions
- Phone sync for published PNGs (Syncthing/AirDrop)
- Instagram Graph API integration (long-term)

## Working Principles

- **CLI-first.** Prefer `git`, `npm`, `npx eleventy`, file editing, over GUI tools. euromancer is both subject and method.
- **Pure terminal aesthetic.** Decorative layers (noise textures, scanlines, fade animations, hover overlays) have been deliberately stripped in prior sessions. Terminal purity is the concept — do not reintroduce effects without explicit request.
- **Plain-text minimalism.** Reference point: https://cultdeadcow.com/ — monospace, ASCII-art headers, lists of dated links, zero chrome. When in doubt, less.
- **Inline styles in HTML are avoided in site files.** Styles belong in `css/styles.css`. Exception: historical witness-page prototypes use inline styles for Claude-preview environment constraints; those files are separate from the main site.
- **Valid shell semantics.** CLI prompts in content (e.g., `n_euromancer@typedeck:~$`) should use plausible command structures, not decorative fake syntax.
- **One source, multiple outputs.** The `.md` file is canonical. Web and Instagram are views of the same content.

## Known Gotchas

- Eleventy's `--dryrun` reports "Wrote 0 files" even on successful runs because nothing is physically written. For diagnostics, run without `--dryrun` and inspect `_site/`.
- Pages aggressively caches. After deploy, use Cmd+Shift+R or an incognito window to see changes.
- `package.json` must have `"type": "module"` at the top level. A duplicated `"type": "commonjs"` key caused ES module loading to fail earlier — JSON takes the last value for duplicate keys.
- Both `.gitignore` and `.eleventyignore` should exclude `node_modules/`. When `.eleventyignore` exists, Eleventy's behavior around `.gitignore` can shift between versions; duplicating is safer.
- markdown-it terminates an HTML block at the first blank line — a `<pre>` with empty lines inside (ASCII art) gets mangled into paragraphs/code blocks. Posts that are raw HTML (typewriting collages) must set `templateEngineOverride: njk` in frontmatter to skip markdown processing entirely.
- Relative `src` in post HTML resolves from the page URL (`…/NNNN/Name/`), but attachments live one level up — use `../attachments/file.webp`.
- `pre` has a UA-stylesheet `font-family: monospace` that beats inheritance — set `font-family: inherit` explicitly when a container defines the font.
## render-mode (active task)
CSS var `--font-base` + `.render-mode` class on `<body>`.
Playwright adds the class before screenshotting.
URL param `?render=1` adds it in browser (for preview).
Do NOT hardcode font sizes — always use the var override pattern.
