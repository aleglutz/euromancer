# CLAUDE.md — euromancer-ssh

TUI edition of euromancer, served over SSH. Third output next to web (Eleventy)
and Instagram (render.js). Reads the same `../archive/NNNN/*.md` — one source,
three presses.

Reference for the genre: `ssh ssh.morilliu.com`, terminal.shop.

## Stack

- **Go** + charm libraries: wish (SSH server, no openssh involved),
  bubbletea (TUI), glamour (markdown → ANSI), lipgloss (styles)
- Pinned versions in go.mod (bubbletea v1.1.0 etc.) were chosen for a go1.22
  sandbox; on this machine with current Go feel free to upgrade all charm
  deps to latest — API changes are minor (check wish v2 import paths if
  bumping major).
- Deploy target: **fly.io**, raw TCP port 22 → internal 23234, config in
  repo-root `fly.toml`, image built by repo-root `Dockerfile` (content is
  baked into the image: push = publish).

## Current state (verified working end-to-end)

- Server starts, loads posts with `status: ready`, real ssh sessions work
- Three screens, mirroring the web nav: home (cover art + `home.md` prose,
  scrollable) → archive list → post; enter/l forward, h/esc back, q back/quit
- Archive list: cDc style `date - [title]`, j/k + enter navigation
- Post view: glamour "dark" theme, scrollable, statusline `title · N/M`;
  tdfiglet headline (title, `cover_font` override) rendered once at startup
  by shelling out to tdfiglet — Dockerfile builds it into the image; falls
  back to no art when the binary is missing or the terminal is narrower
  than the art
- Typewriting collages pass through as raw ASCII inside a fenced block
  (regex in `prepare()` unwraps `<div class="typewriting"><pre>…`)
- `<img>` tags → `[image — see the web edition]`
- Two bugs already fixed, don't reintroduce:
  1. View panicked on zero terminal height (slice with negative index) —
     viewport math is now clamped; keep it clamped
  2. WindowSizeMsg with 0×0 from degenerate PTYs overwrote the 80×24
     fallback — zero values are now ignored in Update

## Conventions

- Palette = `body.euromancer` from `../css/styles.css`: bg `#0d1117`,
  fg `#8b949e`, accent `#2f6f9c` (RWR Kleine Galerie Wurzen blue).
  Keep TUI flat: no borders/shadows-emulation, complexity comes from the
  ASCII content itself, not the chrome.
- Ruth Wolf-Rehfeldt's works are "typewritings", never "typograms"
- Status/help lines mimic tmux statusline (inverse video)
- Env config: `CONTENT_DIR` (default `../archive`), `PORT` (23234),
  `HOST` (0.0.0.0), `HOST_KEY` (.ssh/euromancer_ed25519)

## Dev loop

```sh
go build -o euromancer-ssh . && ./euromancer-ssh
# other pane:
ssh -p 23234 localhost
```

`~/.ssh/config` should have the localhost block (UserKnownHostsFile
/dev/null, StrictHostKeyChecking no) to skip fingerprint prompts.

## Deploy (fly.io)

```sh
fly launch --no-deploy        # once; app name may need a suffix
fly volumes create ssh_key --size 1 -r ams   # persistent host key
fly deploy                    # every publish
ssh euromancer.fly.dev
```

New post = commit to archive/ + `fly deploy` (content is in the image).

## Backlog

- [x] raw `<pre>` passthrough bypassing glamour (no code-block frame around
      typewritings) — split body into segments, render only prose via glamour
- [x] tdfiglet/figlet cover header on the list screen — same forgotex art as
      the web h1, baked at build time into `header.txt` (`go:generate` +
      `go:embed`); falls back to the one-line title when the terminal is
      narrower than the art (71 cols) or too short
- [ ] `.dur` durdraw playback on connect (cover animation)
- [ ] chafa half-block preview for dithered webp attachments
- [ ] hot content reload on SIGHUP (for future VPS; on fly redeploy is fine)
- [ ] guestbook: append-only file, write via `s` key — BBS gesture
- [ ] publish host key fingerprint on the web site ("verify the press")

## Repo hygiene

- Add `euromancer-ssh/` build artifacts to .gitignore: the binary, `.ssh/`
- Add `TUI.md` and `euromancer-ssh/` to `.eleventyignore` so Eleventy
  doesn't publish them as pages
- Never commit `.ssh/euromancer_ed25519` (host private key)
