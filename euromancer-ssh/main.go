// euromancer-ssh — the same archive/NNNN/*.md, served over SSH as a TUI.
// Third output next to web (Eleventy) and Instagram (render.js).
//
//	go build && ./euromancer-ssh          # CONTENT_DIR=../archive PORT=23234
//	ssh -p 23234 localhost
package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	bm "github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	"github.com/muesli/termenv"
)

// ── samizdat palette (from css/styles.css, body.euromancer) ──
var (
	ink    = lipgloss.Color("#8b949e")
	paper  = lipgloss.Color("#0d1117")
	accent = lipgloss.Color("#2f6f9c") // RWR Kleine Galerie Wurzen blue
)

// theme is the style set of one session. Styles must be built from the
// session's lipgloss renderer (bm.MakeRenderer): the color profile comes
// from the client terminal — package-level styles would inherit the
// headless server's profile and silently drop all color.
type theme struct {
	title, dim, cursor, active, status, brackets lipgloss.Style
}

func newTheme(r *lipgloss.Renderer) theme {
	return theme{
		title: r.NewStyle().Foreground(accent).Bold(true),
		dim:   r.NewStyle().Foreground(ink),
		// nav semantics mirror the web css: interactive items are accent
		// [links], the focused one gets a:hover accent inverse (cursor);
		// the current location (nav .active) is fg inverse, no brackets,
		// and can never take the hover highlight
		cursor:   r.NewStyle().Foreground(paper).Background(accent),
		active:   r.NewStyle().Foreground(paper).Background(ink).Padding(0, 1),
		status:   r.NewStyle().Foreground(paper).Background(ink), // tmux statusline
		brackets: r.NewStyle().Foreground(accent),
	}
}

// newRenderer builds a glamour renderer in the site palette: prose in fg,
// headings and links in the accent blue.
func newRenderer(width int, profile termenv.Profile) *glamour.TermRenderer {
	fg, ac := "#8b949e", "#2f6f9c"
	style := styles.DarkStyleConfig
	style.Document.Color = &fg
	style.Heading.Color = &ac
	style.Link.Color = &ac
	style.LinkText.Color = &ac
	r, _ := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithColorProfile(profile),
		glamour.WithWordWrap(min(width-2, 100)),
	)
	return r
}

// ── cover header ─────────────────────────────────────────
// Baked at build time — same tdfiglet art as the web h1, no runtime binary.
//go:generate sh -c "tdfiglet -f forgotex.tdf -w 200 euromancer | perl -pe 's/\\e\\[[0-9;]*m//g; s/[ \\t]+$//' | sed '/^$/d' > header.txt"

//go:embed header.txt
var headerArt string

//go:embed home.md
var homeMD string

var (
	headerLines = strings.Split(strings.TrimRight(headerArt, "\n"), "\n")
	headerCols  = maxCols(headerLines)
	ansiRe      = regexp.MustCompile(`\x1b\[[0-9;]*m`)
)

func maxCols(lines []string) int {
	w := 0
	for _, l := range lines {
		w = max(w, utf8.RuneCountInString(l))
	}
	return w
}

// tdfHeader renders a post headline with tdfiglet, like the web h1.
// Runs once per post at startup; returns nil when the binary or the
// font is missing — the post view then falls back to the plain title.
func tdfHeader(title, font string) []string {
	if font == "" {
		font = "forgotex"
	}
	out, err := exec.Command("tdfiglet", "-f", font+".tdf", "-w", "200", title).Output()
	if err != nil {
		return nil
	}
	var lines []string
	for _, l := range strings.Split(ansiRe.ReplaceAllString(string(out), ""), "\n") {
		lines = append(lines, strings.TrimRight(l, " \t"))
	}
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// ── content ─────────────────────────────────────────────

type post struct {
	number, name, title, date string
	segments                  []segment
	header                    []string // tdfiglet art, nil if unavailable
}

// segment is a chunk of post body: prose (rendered through glamour),
// raw ascii (typewriting, passed straight to the terminal) or an image
// reference (braille-rendered at post-open time, to the session width).
type segKind int

const (
	segProse segKind = iota
	segRaw
	segImage // text holds the attachment path
)

type segment struct {
	kind segKind
	text string
}

var (
	fmRe   = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n?`)
	kvRe   = regexp.MustCompile(`(?m)^(\w[\w-]*):\s*(.*)$`)
	slide  = regexp.MustCompile(`<!--\s*slide:\w+\s*-->`)
	twDiv  = regexp.MustCompile(`(?s)<div class="typewriting"[^>]*>\s*<pre>\n?(.*?)</pre>\s*(?:<img[^>]*>\s*)*</div>`)
	imgTag = regexp.MustCompile(`<img[^>]*>`)
	imgRef = regexp.MustCompile(`!\[\[([^\]]+?)\]\]|<img[^>]*\bsrc="([^"]+)"[^>]*>`)
)

func loadPosts(root string) ([]post, error) {
	var posts []post
	dirs, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, d := range dirs {
		if !d.IsDir() || !regexp.MustCompile(`^\d{4}$`).MatchString(d.Name()) {
			continue
		}
		files, _ := os.ReadDir(filepath.Join(root, d.Name()))
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".md") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(root, d.Name(), f.Name()))
			if err != nil {
				continue
			}
			src := string(raw)
			fm := map[string]string{}
			if m := fmRe.FindStringSubmatch(src); m != nil {
				for _, kv := range kvRe.FindAllStringSubmatch(m[1], -1) {
					fm[kv[1]] = strings.Trim(kv[2], "[] ")
				}
				src = src[len(m[0]):]
			}
			if fm["status"] != "ready" {
				continue
			}
			p := post{
				number:   d.Name(),
				name:     strings.TrimSuffix(f.Name(), ".md"),
				title:    fm["title"],
				date:     fm["date"],
				segments: prepare(src),
			}
			if p.title == "" {
				p.title = p.name
			}
			p.header = tdfHeader(p.title, fm["cover_font"])
			posts = append(posts, p)
		}
	}
	sort.Slice(posts, func(i, j int) bool { return posts[i].number < posts[j].number })
	return posts, nil
}

// prepare adapts web-oriented markdown for the terminal, splitting the body
// into prose segments (rendered through glamour) and raw typewriting
// segments (already ASCII — the terminal is its home medium, passed through
// untouched, no code-block frame). Leftover img tags in prose are noted.
func prepare(src string) []segment {
	src = slide.ReplaceAllString(src, "\n---\n")

	var segments []segment
	last := 0
	for _, loc := range twDiv.FindAllStringSubmatchIndex(src, -1) {
		if loc[0] > last {
			segments = append(segments, segment{text: imgTag.ReplaceAllString(src[last:loc[0]], "`[image — see the web edition]`")})
		}
		segments = append(segments, segment{kind: segRaw, text: strings.Trim(src[loc[2]:loc[3]], "\n")})
		last = loc[1]
	}
	if last < len(src) {
		segments = append(segments, segment{text: imgTag.ReplaceAllString(src[last:], "`[image — see the web edition]`")})
	}
	return segments
}

// proseSegments splits a prose chunk around image references —
// `![[file.webp]]` wikilinks and `<img src="…">` tags.
func proseSegments(chunk, attachDir string) []segment {
	var segments []segment
	last := 0
	for _, loc := range imgRef.FindAllStringSubmatchIndex(chunk, -1) {
		var name string
		if loc[2] >= 0 {
			name = chunk[loc[2]:loc[3]]
		} else {
			name = chunk[loc[4]:loc[5]]
		}
		name = filepath.Base(strings.SplitN(name, "|", 2)[0])
		if loc[0] > last {
			segments = append(segments, segment{text: chunk[last:loc[0]]})
		}
		segments = append(segments, segment{kind: segImage, text: filepath.Join(attachDir, name)})
		last = loc[1]
	}
	if last < len(chunk) {
		segments = append(segments, segment{text: chunk[last:]})
	}
	return segments
}

// ── bubbletea model ─────────────────────────────────────

type screen int

const (
	scrHome screen = iota
	scrList
	scrPost
)

type model struct {
	posts         []post
	cursor        int
	screen        screen
	rendered      string // scrollable body of the current home/post screen
	scroll        int
	width, height int
	renderer      *glamour.TermRenderer
	th            theme
	profile       termenv.Profile
}

func (m model) Init() tea.Cmd { return nil }

// renderMD runs prose through glamour, falling back to the source text.
func (m model) renderMD(src string) string {
	if m.renderer != nil {
		if r, err := m.renderer.Render(src); err == nil {
			return r
		}
	}
	return src
}

// scatter mirrors the web's Image Scatter Principle: each braille image
// gets its own width and indent — a loose collection, not a grid.
var scatter = []struct {
	frac   float64
	indent int
}{{0.62, 6}, {0.75, 0}, {0.5, 12}}

// renderSegments renders a segment list to the session width: prose via
// glamour, raw ascii untouched, images as braille art.
func (m model) renderSegments(segs []segment) string {
	textW := min(m.width-2, 100)
	var b strings.Builder
	img := 0
	for _, seg := range segs {
		switch seg.kind {
		case segRaw:
			b.WriteString(m.th.dim.Render(seg.text))
			b.WriteString("\n")
		case segImage:
			sc := scatter[img%len(scatter)]
			img++
			if art, err := brailleRender(seg.text, max(10, int(float64(textW)*sc.frac))); err == nil {
				pad := strings.Repeat(" ", sc.indent)
				lines := strings.Split(strings.TrimRight(art, "\n"), "\n")
				for i, l := range lines {
					lines[i] = pad + l
				}
				b.WriteString(m.th.dim.Render(strings.Join(lines, "\n")) + "\n\n")
			} else {
				b.WriteString(m.renderMD("`[image — see the web edition]`"))
			}
		default:
			if strings.TrimSpace(seg.text) != "" {
				b.WriteString(m.renderMD(seg.text))
			}
		}
	}
	return b.String()
}

// homeSegments is home.md split around its image wikilinks, resolved
// against the site's assets/images at startup.
var homeSegments []segment

func (m model) renderHome() string { return m.renderSegments(homeSegments) }

func (m model) renderPost(p post) string { return m.renderSegments(p.segments) }

// cover is the euromancer headline of the home and list screens; on
// terminals the art doesn't fit it degrades to a one-line accent title.
func (m model) cover() string {
	if m.width >= headerCols && m.height >= len(headerLines)+8 {
		return m.th.dim.Render(strings.Join(headerLines, "\n")) + "\n"
	}
	return m.th.title.Render("euromancer") + "\n"
}

// topBlock is the fixed chrome above the content, mirroring the web
// header on every page: headline art, nav bar under it.
func (m model) topBlock() string {
	var b strings.Builder
	switch m.screen {
	case scrPost:
		p := m.posts[m.cursor]
		if p.header != nil && m.width >= maxCols(p.header) && m.height >= len(p.header)+8 {
			b.WriteString(m.th.dim.Render(strings.Join(p.header, "\n")) + "\n")
		} else {
			b.WriteString(m.th.title.Render(p.title) + "\n")
		}
		b.WriteString(m.th.brackets.Render("[home] [texts+images]") + " " + m.th.active.Render(p.name+".md") + "\n\n")
	case scrList:
		b.WriteString(m.cover())
		home := m.th.brackets.Render("[home]")
		if m.cursor < 0 { // menu focus, lynx-style hover
			home = m.th.cursor.Render("[home]")
		}
		b.WriteString(home + " " + m.th.active.Render("texts+images") + "\n\n")
	default:
		b.WriteString(m.cover())
		b.WriteString(m.th.active.Render("home") + " " + m.th.cursor.Render("[texts+images]") + "\n\n")
	}
	return b.String()
}

// pageStep is one pgup/pgdown jump: the viewport height under the chrome.
func (m model) pageStep() int {
	return max(1, m.height-strings.Count(m.topBlock(), "\n")-1)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		m.renderer = newRenderer(m.width, m.profile)
		switch m.screen {
		case scrHome:
			m.rendered = m.renderHome()
		case scrPost:
			m.rendered = m.renderPost(m.posts[m.cursor])
		}
	case tea.KeyMsg:
		scrollable := m.screen != scrList
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			switch m.screen {
			case scrPost:
				m.screen, m.scroll = scrList, 0
			default:
				return m, tea.Quit
			}
		case "up", "k":
			if scrollable {
				m.scroll = max(0, m.scroll-1)
			} else if m.cursor > -1 { // -1 = menu focus on [home]
				m.cursor--
			}
		case "down", "j":
			if scrollable {
				m.scroll++
			} else if m.cursor < len(m.posts)-1 {
				m.cursor++
			}
		case "pgup", "b":
			m.scroll = max(0, m.scroll-m.pageStep())
		case "pgdown", "f", " ":
			if scrollable {
				m.scroll += m.pageStep()
			}
		case "enter", "right", "l":
			switch m.screen {
			case scrHome:
				m.screen, m.scroll, m.cursor = scrList, 0, max(0, m.cursor)
			case scrList:
				if m.cursor < 0 {
					m.rendered = m.renderHome()
					m.screen, m.scroll = scrHome, 0
				} else if len(m.posts) > 0 {
					m.rendered = m.renderPost(m.posts[m.cursor])
					m.screen, m.scroll = scrPost, 0
				}
			}
		case "left", "h", "esc":
			switch m.screen {
			case scrPost:
				m.screen, m.scroll = scrList, 0
			case scrList:
				m.rendered = m.renderHome()
				m.screen, m.scroll = scrHome, 0
			}
		}
	}
	return m, nil
}

// pager renders the fixed top chrome, the scrollable m.rendered viewport
// and a statusline; statusFor gets the last visible line and the total.
func (m model) pager(top string, statusFor func(shown, total int) string) string {
	lines := strings.Split(m.rendered, "\n")
	visible := max(1, m.height-strings.Count(top, "\n")-1)
	start := min(m.scroll, max(0, len(lines)-1))
	end := min(len(lines), start+visible)
	return top + strings.Join(lines[start:end], "\n") +
		"\n" + m.th.status.Render(statusFor(end, len(lines)))
}

func (m model) View() string {
	top := m.topBlock()
	switch m.screen {
	case scrHome:
		return m.pager(top, func(shown, total int) string {
			return fmt.Sprintf(" euromancer · %d/%d · j/k scroll · enter texts+images · q quit ", shown, total)
		})
	case scrPost:
		p := m.posts[m.cursor]
		return m.pager(top, func(shown, total int) string {
			return fmt.Sprintf(" %s · %d/%d · j/k scroll · q back ", p.title, shown, total)
		})
	}
	// archive list — cDc style: date - [title]
	var b strings.Builder
	b.WriteString(top)
	for i, p := range m.posts {
		line := fmt.Sprintf("%s - [%s]", p.date, p.title)
		if i == m.cursor {
			b.WriteString(m.th.cursor.Render(line) + "\n")
		} else {
			b.WriteString(m.th.dim.Render(p.date+" - ") + m.th.brackets.Render("["+p.title+"]") + "\n")
		}
	}
	b.WriteString("\n" + m.th.status.Render(" j/k move · enter read · h home · q quit "))
	return b.String()
}

func min(a, b int) int { if a < b { return a }; return b }
func max(a, b int) int { if a > b { return a }; return b }

// ── wish server ─────────────────────────────────────────

func main() {
	host := envOr("HOST", "0.0.0.0")
	port := envOr("PORT", "23234")
	content := envOr("CONTENT_DIR", "../archive")
	keyPath := envOr("HOST_KEY", ".ssh/euromancer_ed25519")

	posts, err := loadPosts(content)
	if err != nil {
		log.Fatal("could not read content", "dir", content, "error", err)
	}
	homeSegments = proseSegments(homeMD, filepath.Join(content, "..", "assets", "images"))
	log.Info("content loaded", "posts", len(posts))

	s, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		wish.WithHostKeyPath(keyPath),
		wish.WithMiddleware(
			bm.Middleware(func(sess ssh.Session) (tea.Model, []tea.ProgramOption) {
				pty, _, _ := sess.Pty()
				w, h := pty.Window.Width, pty.Window.Height
				if w == 0 {
					w = 80
				}
				if h == 0 {
					h = 24
				}
				r := bm.MakeRenderer(sess)
				m := model{posts: posts, width: w, height: h, th: newTheme(r), profile: r.ColorProfile()}
				m.renderer = newRenderer(w, m.profile)
				m.rendered = m.renderHome()
				return m, []tea.ProgramOption{tea.WithAltScreen()}
			}),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Fatal("could not create server", "error", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	log.Info("starting SSH server", "host", host, "port", port)
	go func() {
		if err = s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Error("could not start server", "error", err)
			done <- nil
		}
	}()
	<-done
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Error("could not stop server", "error", err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
