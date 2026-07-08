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
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	bm "github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
)

// ── samizdat palette (from css/styles.css, body.euromancer) ──
var (
	ink    = lipgloss.Color("#8b949e")
	paper  = lipgloss.Color("#0d1117")
	accent = lipgloss.Color("#2f6f9c") // RWR Kleine Galerie Wurzen blue

	titleStyle  = lipgloss.NewStyle().Foreground(accent).Bold(true)
	dimStyle    = lipgloss.NewStyle().Foreground(ink)
	cursorStyle = lipgloss.NewStyle().Foreground(paper).Background(accent) // inverse video
	statusStyle = lipgloss.NewStyle().Foreground(paper).Background(ink)    // tmux statusline
	brackets    = lipgloss.NewStyle().Foreground(accent)
)

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

// segment is a chunk of post body: raw ascii (typewriting, passed straight
// to the terminal) or prose (rendered through glamour).
type segment struct {
	raw  bool
	text string
}

var (
	fmRe   = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n?`)
	kvRe   = regexp.MustCompile(`(?m)^(\w[\w-]*):\s*(.*)$`)
	slide  = regexp.MustCompile(`<!--\s*slide:\w+\s*-->`)
	twDiv  = regexp.MustCompile(`(?s)<div class="typewriting"[^>]*>\s*<pre>\n?(.*?)</pre>\s*(?:<img[^>]*>\s*)*</div>`)
	imgTag = regexp.MustCompile(`<img[^>]*>`)
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
		segments = append(segments, segment{raw: true, text: strings.Trim(src[loc[2]:loc[3]], "\n")})
		last = loc[1]
	}
	if last < len(src) {
		segments = append(segments, segment{text: imgTag.ReplaceAllString(src[last:], "`[image — see the web edition]`")})
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

// renderHome composes the home screen body: the cover art (when it fits)
// above the prose from home.md — the art scrolls with the content, like
// the web header.
func (m model) renderHome() string {
	var b strings.Builder
	if m.width >= headerCols {
		b.WriteString(dimStyle.Render(strings.Join(headerLines, "\n")) + "\n")
	}
	b.WriteString(m.renderMD(homeMD))
	return b.String()
}

// renderPost composes the post body: tdfiglet headline (when present and
// it fits) above the segments.
func (m model) renderPost(p post) string {
	var b strings.Builder
	if p.header != nil && m.width >= maxCols(p.header) {
		b.WriteString(dimStyle.Render(strings.Join(p.header, "\n")) + "\n\n")
	}
	for _, seg := range p.segments {
		if seg.raw {
			b.WriteString(dimStyle.Render(seg.text))
			b.WriteString("\n")
			continue
		}
		b.WriteString(m.renderMD(seg.text))
	}
	return b.String()
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
		m.renderer, _ = glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(min(m.width-2, 100)),
		)
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
			} else if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if scrollable {
				m.scroll++
			} else if m.cursor < len(m.posts)-1 {
				m.cursor++
			}
		case "pgup", "b":
			m.scroll = max(0, m.scroll-(m.height-4))
		case "pgdown", "f", " ":
			if scrollable {
				m.scroll += m.height - 4
			}
		case "enter", "right", "l":
			switch m.screen {
			case scrHome:
				m.screen, m.scroll = scrList, 0
			case scrList:
				if len(m.posts) > 0 {
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

// pager renders a fixed nav line, the scrollable m.rendered viewport and
// a statusline; statusFor gets the last visible line and the line total.
func (m model) pager(nav string, statusFor func(shown, total int) string) string {
	var b strings.Builder
	b.WriteString(nav + "\n\n")
	lines := strings.Split(m.rendered, "\n")
	visible := max(1, m.height-4)
	start := min(m.scroll, max(0, len(lines)-1))
	end := min(len(lines), start+visible)
	b.WriteString(strings.Join(lines[start:end], "\n"))
	b.WriteString("\n" + statusStyle.Render(statusFor(end, len(lines))))
	return b.String()
}

func (m model) View() string {
	switch m.screen {
	case scrHome:
		nav := cursorStyle.Render("home") + " " + brackets.Render("[texts+images]")
		return m.pager(nav, func(shown, total int) string {
			return fmt.Sprintf(" euromancer · %d/%d · j/k scroll · enter texts+images · q quit ", shown, total)
		})
	case scrPost:
		p := m.posts[m.cursor]
		nav := brackets.Render("[home] [texts+images]") + " " + cursorStyle.Render(p.name+".md")
		return m.pager(nav, func(shown, total int) string {
			return fmt.Sprintf(" %s · %d/%d · j/k scroll · q back ", p.title, shown, total)
		})
	}
	// archive list — cDc style: date - [title]
	var b strings.Builder
	if m.width >= headerCols && m.height >= len(headerLines)+len(m.posts)+6 {
		b.WriteString(dimStyle.Render(strings.Join(headerLines, "\n")) + "\n")
	}
	b.WriteString(brackets.Render("[home]") + " " + cursorStyle.Render("texts+images") + "\n\n")
	for i, p := range m.posts {
		line := fmt.Sprintf("%s - [%s]", p.date, p.title)
		if i == m.cursor {
			b.WriteString(cursorStyle.Render(line) + "\n")
		} else {
			b.WriteString(dimStyle.Render(p.date+" - ") + brackets.Render("["+p.title+"]") + "\n")
		}
	}
	b.WriteString("\n" + statusStyle.Render(" j/k move · enter read · h home · q quit "))
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
				m := model{posts: posts, width: w, height: h}
				m.renderer, _ = glamour.NewTermRenderer(
					glamour.WithStandardStyle("dark"),
					glamour.WithWordWrap(min(w-2, 100)),
				)
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
