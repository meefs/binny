package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// role tags a transcript line so it renders with a consistent prefix + color.
type role int

const (
	roleSys role = iota
	roleUser
	roleBinny
	roleTool
	roleErr
	roleOK
	roleWarn
)

// line is one rendered entry in the scrollback transcript.
type line struct {
	role role
	text string
}

func sysLine(s string) line   { return line{roleSys, s} }
func userLine(s string) line  { return line{roleUser, s} }
func binnyLine(s string) line { return line{roleBinny, s} }
func toolLine(s string) line  { return line{roleTool, s} }
func errLine(s string) line   { return line{roleErr, s} }
func okLine(s string) line    { return line{roleOK, s} }
func warnLine(s string) line  { return line{roleWarn, s} }

// render formats a line, wrapping to width. Prefix + color come from the role.
func (l line) render(width int) string {
	var prefix string
	var style lipgloss.Style
	switch l.role {
	case roleUser:
		prefix, style = "you › ", styleUser
	case roleBinny:
		prefix, style = "binny › ", styleBinny
	case roleTool:
		prefix, style = "  ⚙ ", styleTool
	case roleErr:
		prefix, style = "  ✗ ", styleErr
	case roleOK:
		prefix, style = "  ✓ ", styleOK
	case roleWarn:
		prefix, style = "  ! ", styleWarn
	default:
		prefix, style = "", styleDim
	}
	// Wrap the body to the width LEFT AFTER the prefix/indent, so prefix+body
	// (and the indented continuation rows) fit inside `width` instead of
	// overflowing and getting clipped on the right.
	pw := lipgloss.Width(prefix)
	body := l.text
	if wrapW := width - pw; wrapW > 8 {
		body = lipgloss.NewStyle().Width(wrapW).Render(l.text)
	}
	// Prefix only the first wrapped row; continuation rows align under it.
	rows := strings.Split(body, "\n")
	pad := strings.Repeat(" ", pw)
	for i, r := range rows {
		if i == 0 {
			rows[i] = style.Render(prefix) + r
		} else {
			rows[i] = pad + r
		}
	}
	return strings.Join(rows, "\n")
}
