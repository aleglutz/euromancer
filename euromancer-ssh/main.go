// euromancer-ssh — the same archive/NNNN/*.md, served over SSH as a TUI.
// Third output next to web (Eleventy) and Instagram (render.js).
//
//	go build && ./euromancer-ssh          # CONTENT_DIR=../archive PORT=23234
//	ssh -p 23234 localhost
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

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

// ── content ─────────────────────────────────────────────

type post struct {
	number, name, title, date string
	segments                  []segment
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

type model struct {
	posts         []post
	cursor        int
	viewing       bool
	rendered      string
	scroll        int
	width, height int
	renderer      *glamour.TermRenderer
}

func (m model) Init() tea.Cmd { return nil }

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
			glamour.WithWordWrap(min(msg.Width-2, 100)),
		)
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.viewing {
				m.viewing = false
				return m, nil
			}
			return m, tea.Quit
		case "up", "k":
			if m.viewing {
				m.scroll = max(0, m.scroll-1)
			} else if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.viewing {
				m.scroll++
			} else if m.cursor < len(m.posts)-1 {
				m.cursor++
			}
		case "pgup", "b":
			m.scroll = max(0, m.scroll-(m.height-4))
		case "pgdown", "f", " ":
			if m.viewing {
				m.scroll += m.height - 4
			}
		case "enter", "right", "l":
			if !m.viewing && len(m.posts) > 0 {
				p := m.posts[m.cursor]
				var b strings.Builder
				for _, seg := range p.segments {
					if seg.raw {
						b.WriteString(dimStyle.Render(seg.text))
						b.WriteString("\n")
						continue
					}
					if m.renderer != nil {
						if r, err := m.renderer.Render(seg.text); err == nil {
							b.WriteString(r)
							continue
						}
					}
					b.WriteString(seg.text)
				}
				m.rendered, m.viewing, m.scroll = b.String(), true, 0
			}
		case "left", "h", "esc":
			m.viewing = false
		}
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	if m.viewing {
		p := m.posts[m.cursor]
		b.WriteString(brackets.Render("[texts+images]") + " " + dimStyle.Render(p.name+".md") + "\n\n")
		lines := strings.Split(m.rendered, "\n")
		visible := max(1, m.height-4)
		start := min(m.scroll, max(0, len(lines)-1))
		end := min(len(lines), start+visible)
		b.WriteString(strings.Join(lines[start:end], "\n"))
		b.WriteString("\n" + statusStyle.Render(fmt.Sprintf(" %s · %d/%d · j/k scroll · q back ",
			p.title, min(end, len(lines)), len(lines))))
		return b.String()
	}
	// archive list — cDc style: date - [title]
	b.WriteString(titleStyle.Render("euromancer") + dimStyle.Render(" — texts+images over ssh") + "\n\n")
	for i, p := range m.posts {
		line := fmt.Sprintf("%s - [%s]", p.date, p.title)
		if i == m.cursor {
			b.WriteString(cursorStyle.Render(line) + "\n")
		} else {
			b.WriteString(dimStyle.Render(p.date+" - ") + brackets.Render("["+p.title+"]") + "\n")
		}
	}
	b.WriteString("\n" + statusStyle.Render(" j/k move · enter read · q quit "))
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
