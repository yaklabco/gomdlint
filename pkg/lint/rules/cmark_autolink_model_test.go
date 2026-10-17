package rules

// This file models the autolink extension of cmark-gfm, which GitHub renders
// Markdown with (github/cmark-gfm, extensions/autolink.c), for the MD034
// sweep test: a fix that wraps a literal in angle brackets must leave the
// links GitHub shows unchanged. The functions follow the C functions of the
// same names; where the C code works on size_t, the model keeps the same
// bounds with int.

import (
	"bytes"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/pkg/lint/mdscan"
)

// cmarkLink is a link the model finds: its span in the inline content it
// scanned and its destination.
type cmarkLink struct {
	start, end int
	url        string
}

// cmarkText is text that cmark-gfm keeps in a text node, decoded, with the
// source offsets each byte came from.
type cmarkText struct {
	text   []byte
	starts []int // where the source of text[i] starts
	ends   []int // where the source of text[i] ends
}

func (c *cmarkText) add(from, to int, decoded []byte) {
	for _, ch := range decoded {
		c.text = append(c.text, ch)
		c.starts = append(c.starts, from)
		c.ends = append(c.ends, to)
	}
}

// unput takes back the bytes whose source starts at or after from, as
// cmark_node_unput does for the scheme that url_match rewinds over.
func (c *cmarkText) unput(from int) {
	keep := len(c.text)
	for keep > 0 && c.starts[keep-1] >= from {
		keep--
	}
	c.text, c.starts, c.ends = c.text[:keep], c.starts[:keep], c.ends[:keep]
}

// cmarkAutolinks returns the links that cmark-gfm's autolink extension makes
// in one line of inline content. The inline parser calls url_match at each
// ":" and www_match at each "w" it reaches in text, but not inside code
// spans, angle brackets (autolinks and raw HTML), backslash escapes, or
// character references, and not while a "[" is open. postprocess_text then
// finds emails in the decoded text outside those constructs and links.
// Emphasis does not split the text here: unpaired delimiters stay text, and
// the sweep attaches no fix where paired ones would split a literal. Tables
// are not modelled either: MD034 attaches no fix in a run of lines that could
// hold one, and the sweep checks that on its own.
func cmarkAutolinks(content []byte) []cmarkLink {
	var links []cmarkLink
	var texts []cmarkText
	current := cmarkText{}
	flush := func() {
		if len(current.text) > 0 {
			texts = append(texts, current)
		}
		current = cmarkText{}
	}
	textStart, brackets := 0, 0
	for pos := 0; pos < len(content); {
		switch ch := content[pos]; {
		case ch == '`' || ch == '<':
			if end := cmarkConstructEnd(content, pos); end > pos {
				flush()
				pos, textStart = end, end
				continue
			}
			if ch == '`' {
				// A run of backticks that opens no code span is text as a whole.
				for pos < len(content) && content[pos] == '`' {
					current.add(pos, pos+1, content[pos:pos+1])
					pos++
				}
				continue
			}
		case ch == '\\' && pos+1 < len(content) && mdscan.IsASCIIPunct(content[pos+1]):
			current.add(pos, pos+2, content[pos+1:pos+2])
			pos += 2
			continue
		case ch == '&':
			if end, decoded := cmarkEntity(content, pos); end > pos {
				current.add(pos, end, decoded)
				pos = end
				continue
			}
		case ch == '[':
			brackets++
		case ch == ']' && brackets > 0:
			brackets--
		case ch == ':' && brackets == 0:
			if link, ok := cmarkURLMatch(content, pos, textStart); ok {
				current.unput(link.start)
				flush()
				links = append(links, link)
				pos, textStart = link.end, link.end
				continue
			}
		case ch == 'w' && brackets == 0:
			if link, ok := cmarkWWWMatch(content, pos); ok {
				flush()
				links = append(links, link)
				pos, textStart = link.end, link.end
				continue
			}
		}
		current.add(pos, pos+1, content[pos:pos+1])
		pos++
	}
	flush()
	for _, text := range texts {
		links = append(links, cmarkEmails(text)...)
	}
	sort.Slice(links, func(i, j int) bool { return links[i].start < links[j].start })
	return links
}

// cmarkAutolinkPattern and cmarkTagPattern match a CommonMark autolink and a
// simple HTML tag, which the inline parser takes whole at "<".
var (
	cmarkAutolinkPattern = regexp.MustCompile(`^<(?:[A-Za-z][A-Za-z0-9+.-]{1,31}:[^<>\x00-\x20]*|` +
		`[A-Za-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?` +
		`(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*)>`)
	cmarkTagPattern = regexp.MustCompile(`^</?[A-Za-z][A-Za-z0-9-]*(?:\s[^<>]*)?/?>`)
)

// cmarkConstructEnd returns the end of the code span, autolink, or HTML tag
// that starts at content[pos], or pos when none does.
func cmarkConstructEnd(content []byte, pos int) int {
	if content[pos] == '<' {
		if loc := cmarkAutolinkPattern.FindIndex(content[pos:]); loc != nil {
			return pos + loc[1]
		}
		if loc := cmarkTagPattern.FindIndex(content[pos:]); loc != nil {
			return pos + loc[1]
		}
		return pos
	}
	run := pos
	for run < len(content) && content[run] == '`' {
		run++
	}
	size := run - pos
	for idx := run; idx < len(content); {
		if content[idx] != '`' {
			idx++
			continue
		}
		closing := idx
		for closing < len(content) && content[closing] == '`' {
			closing++
		}
		if closing-idx == size {
			return closing
		}
		idx = closing
	}
	return pos
}

// cmarkEntity decodes the character reference at content[pos], returning
// its end and its text, or pos when none starts there. Only the named
// references the sweep uses are known.
func cmarkEntity(content []byte, pos int) (int, []byte) {
	end := bytes.IndexByte(content[pos:], ';')
	if end < 2 {
		return pos, nil
	}
	name := string(content[pos+1 : pos+end])
	end += pos + 1
	var code int64
	var err error
	switch {
	case strings.HasPrefix(name, "#x") || strings.HasPrefix(name, "#X"):
		code, err = strconv.ParseInt(name[2:], 16, 32)
	case strings.HasPrefix(name, "#"):
		code, err = strconv.ParseInt(name[1:], 10, 32)
	default:
		named := map[string]rune{"amp": '&', "lt": '<', "gt": '>', "quot": '"', "apos": '\'', "nbsp": 0xa0, "copy": 0xa9}
		r, known := named[name]
		if !known {
			return pos, nil
		}
		return end, []byte(string(r))
	}
	if err != nil {
		return pos, nil
	}
	if code == 0 || code > unicode.MaxRune {
		code = utf8.RuneError
	}
	return end, []byte(string(rune(code)))
}

// cmarkIsSpace, cmarkIsAlpha, and cmarkIsAlnum follow cmark's ASCII
// character classes.
func cmarkIsSpace(ch byte) bool { return ch == ' ' || (ch >= '\t' && ch <= '\r') }
func cmarkIsAlpha(ch byte) bool { return isASCIILetter(ch) }
func cmarkIsAlnum(ch byte) bool { return isASCIILetter(ch) || isASCIIDigit(ch) }

// cmarkUTF8IsSpace and cmarkUTF8IsPunct follow cmark_utf8proc_is_space and
// cmark_utf8proc_is_punctuation.
func cmarkUTF8IsSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\f', '\r', ' ', 0xa0, 0x1680, 0x202f, 0x205f, 0x3000:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

func cmarkUTF8IsPunct(r rune) bool {
	if r < utf8.RuneSelf {
		return mdscan.IsASCIIPunct(byte(r))
	}
	return unicode.IsPunct(r)
}

// cmarkIsValidHostchar follows is_valid_hostchar: the character that starts
// at data[0] decodes and is neither space nor punctuation. A continuation
// byte does not decode.
func cmarkIsValidHostchar(data []byte) bool {
	r, size := utf8.DecodeRune(data)
	if r == utf8.RuneError && size <= 1 {
		return false
	}
	return !cmarkUTF8IsSpace(r) && !cmarkUTF8IsPunct(r)
}

// cmarkAutolinkIsSafe follows sd_autolink_issafe.
func cmarkAutolinkIsSafe(link []byte) bool {
	for _, scheme := range []string{"http://", "https://", "ftp://"} {
		if len(link) > len(scheme) && strings.EqualFold(string(link[:len(scheme)]), scheme) &&
			cmarkIsValidHostchar(link[len(scheme):]) {
			return true
		}
	}
	return false
}

// cmarkAutolinkDelim follows autolink_delim: it ends a link at "<" and strips
// trailing punctuation, a ")" only while more close than open, and an entity
// reference before a final ";".
func cmarkAutolinkDelim(data []byte, linkEnd int) int {
	opening, closing := 0, 0
	for idx := range linkEnd {
		if data[idx] == '<' {
			linkEnd = idx
			break
		}
		switch data[idx] {
		case '(':
			opening++
		case ')':
			closing++
		}
	}
	for linkEnd > 0 {
		switch data[linkEnd-1] {
		case ')':
			if closing <= opening {
				return linkEnd
			}
			closing--
			linkEnd--
		case '?', '!', '.', ',', ':', '*', '_', '~', '\'', '"':
			linkEnd--
		case ';':
			newEnd := linkEnd - 2
			for newEnd > 0 && cmarkIsAlpha(data[newEnd]) {
				newEnd--
			}
			if newEnd < linkEnd-2 && data[newEnd] == '&' {
				linkEnd = newEnd
			} else {
				linkEnd--
			}
		default:
			return linkEnd
		}
	}
	return linkEnd
}

// cmarkCheckDomain follows check_domain. It never looks at data[0] or at the
// last byte, and it refuses a domain with an underscore in one of its last
// two segments unless the domain has more than ten.
func cmarkCheckDomain(data []byte, allowShort bool) int {
	size := len(data)
	np, uscore1, uscore2 := 0, 0, 0
	idx := 1
	for ; idx < size-1; idx++ {
		if data[idx] == '\\' && idx < size-2 {
			idx++
		}
		switch {
		case data[idx] == '_':
			uscore2++
		case data[idx] == '.':
			uscore1 = uscore2
			uscore2 = 0
			np++
		case !cmarkIsValidHostchar(data[idx:]) && data[idx] != '-':
			return cmarkDomainResult(idx, np, uscore1, uscore2, allowShort)
		}
	}
	return cmarkDomainResult(idx, np, uscore1, uscore2, allowShort)
}

func cmarkDomainResult(end, np, uscore1, uscore2 int, allowShort bool) int {
	if (uscore1 > 0 || uscore2 > 0) && np <= 10 {
		return 0
	}
	if allowShort || np > 0 {
		return end
	}
	return 0
}

// cmarkURLMatch follows url_match at the ":" at content[colon]: it rewinds
// over the letters of the scheme, which must be http, https, or ftp, checks
// the domain, runs to ASCII whitespace or "<", and strips trailing
// punctuation. textStart bounds the rewind to the text before the ":".
func cmarkURLMatch(content []byte, colon, textStart int) (cmarkLink, bool) {
	data := content[colon:]
	size := len(data)
	if size < 4 || data[1] != '/' || data[2] != '/' {
		return cmarkLink{}, false
	}
	rewind := 0
	for colon-rewind > textStart && cmarkIsAlpha(content[colon-rewind-1]) {
		rewind++
	}
	if !cmarkAutolinkIsSafe(content[colon-rewind:]) {
		return cmarkLink{}, false
	}
	linkEnd := len("://")
	domainLen := cmarkCheckDomain(data[linkEnd:], true)
	if domainLen == 0 {
		return cmarkLink{}, false
	}
	linkEnd += domainLen
	for linkEnd < size && !cmarkIsSpace(data[linkEnd]) && data[linkEnd] != '<' {
		linkEnd++
	}
	if linkEnd = cmarkAutolinkDelim(data, linkEnd); linkEnd == 0 {
		return cmarkLink{}, false
	}
	start, end := colon-rewind, colon+linkEnd
	return cmarkLink{start: start, end: end, url: string(content[start:end])}, true
}

// cmarkWWWMatch follows www_match at the "w" at content[pos]. It links only
// after "*", "_", "~", "(", ASCII whitespace, or the start of the content.
func cmarkWWWMatch(content []byte, pos int) (cmarkLink, bool) {
	if pos > 0 && strings.IndexByte("*_~(", content[pos-1]) < 0 && !cmarkIsSpace(content[pos-1]) {
		return cmarkLink{}, false
	}
	data := content[pos:]
	size := len(data)
	if size < 4 || !bytes.HasPrefix(data, []byte("www.")) {
		return cmarkLink{}, false
	}
	linkEnd := cmarkCheckDomain(data, false)
	if linkEnd == 0 {
		return cmarkLink{}, false
	}
	for linkEnd < size && !cmarkIsSpace(data[linkEnd]) && data[linkEnd] != '<' {
		linkEnd++
	}
	if linkEnd = cmarkAutolinkDelim(data, linkEnd); linkEnd == 0 {
		return cmarkLink{}, false
	}
	return cmarkLink{start: pos, end: pos + linkEnd, url: "http://" + string(data[:linkEnd])}, true
}

// cmarkValidateProtocol follows validate_protocol: protocol ends right before
// the local part, at data[at-rewind], and no letter or digit comes before it.
func cmarkValidateProtocol(protocol string, data []byte, at, rewind, maxRewind int) bool {
	size := len(protocol)
	if size > maxRewind-rewind {
		return false
	}
	if string(data[at-rewind-size:at-rewind]) != protocol {
		return false
	}
	if size == maxRewind-rewind {
		return true
	}
	return !cmarkIsAlnum(data[at-rewind-size-1])
}

// cmarkEmails follows postprocess_text on one decoded text node. It returns
// the email links it makes, with their spans in the source.
func cmarkEmails(node cmarkText) []cmarkLink {
	data := node.text
	var links []cmarkLink
	start, offset, remaining := 0, 0, len(data)
	for offset < remaining {
		at := bytes.IndexByte(data[start+offset:start+remaining], '@')
		if at < 0 {
			break
		}
		maxRewind := at
		autoMailto, isXMPP := true, false
		// As in the C code, np survives the jump back to foundAt.
		np := 0
		var link, linkEnd, rewind int
	foundAt:
		//nolint:intrange // run to the end, the loop leaves rewind at maxRewind, as the C code does
		for rewind = 0; rewind < maxRewind; rewind++ {
			ch := data[start+offset+maxRewind-rewind-1]
			if cmarkIsAlnum(ch) || strings.IndexByte(".+-_", ch) >= 0 {
				continue
			}
			if ch == ':' {
				if cmarkValidateProtocol("mailto:", data, start+offset+maxRewind, rewind, maxRewind) {
					autoMailto = false
					continue
				}
				if cmarkValidateProtocol("xmpp:", data, start+offset+maxRewind, rewind, maxRewind) {
					autoMailto, isXMPP = false, true
					continue
				}
			}
			break
		}
		if rewind == 0 {
			offset += maxRewind + 1
			continue
		}
		link = start + offset + maxRewind
	domain:
		for linkEnd = 1; linkEnd < remaining-offset-maxRewind; linkEnd++ {
			ch := data[link+linkEnd]
			switch {
			case cmarkIsAlnum(ch):
			case ch == '@':
				// Another "@": try again with the text before it as the local part.
				offset += maxRewind + 1
				maxRewind = linkEnd - 1
				goto foundAt
			case ch == '.' && linkEnd < remaining-offset-maxRewind-1 && cmarkIsAlnum(data[link+linkEnd+1]):
				np++
			case ch == '/' && isXMPP:
			case ch != '-' && ch != '_':
				break domain
			}
		}
		if last := data[link+linkEnd-1]; linkEnd < 2 || np == 0 || (!cmarkIsAlpha(last) && last != '.') {
			offset += maxRewind + linkEnd
			continue
		}
		if linkEnd = cmarkAutolinkDelim(data[link:], linkEnd); linkEnd == 0 {
			offset += maxRewind + 1
			continue
		}
		url := string(data[link-rewind : link+linkEnd])
		if autoMailto {
			url = "mailto:" + url
		}
		links = append(links, cmarkLink{start: node.starts[link-rewind], end: node.ends[link+linkEnd-1], url: url})
		start += offset + maxRewind + linkEnd
		remaining -= offset + maxRewind + linkEnd
		offset = 0
	}
	return links
}

// TestCmarkAutolinks checks the model against the autolink examples of the
// GFM spec that the current cmark-gfm code still gives, and against how
// GitHub reads the literals whose fixes MD034 withholds.
func TestCmarkAutolinks(t *testing.T) {
	tests := []struct {
		content string
		want    []cmarkLink
	}{
		{"foo@bar.baz", []cmarkLink{{0, 11, "mailto:foo@bar.baz"}}},
		{"hello@mail+xyz.example isn't valid, but hello+xyz@mail.example is.",
			[]cmarkLink{{40, 62, "mailto:hello+xyz@mail.example"}}},
		{"a.b-c_d@a.b.", []cmarkLink{{0, 11, "mailto:a.b-c_d@a.b"}}},
		{"a.b-c_d@a.b-", nil},
		{"a.b-c_d@a.b_", nil},
		{"mailto:foo@bar.baz", []cmarkLink{{0, 18, "mailto:foo@bar.baz"}}},
		{"www.commonmark.org/he<lp", []cmarkLink{{0, 21, "http://www.commonmark.org/he"}}},
		{"Visit www.commonmark.org.", []cmarkLink{{6, 24, "http://www.commonmark.org"}}},
		{"www.google.com/search?q=Markup+(business)))", []cmarkLink{{0, 41, "http://www.google.com/search?q=Markup+(business)"}}},
		{"(www.google.com/search?q=Markup+(business)", []cmarkLink{{1, 42, "http://www.google.com/search?q=Markup+(business)"}}},
		{"www.google.com/search?q=commonmark&hl;", []cmarkLink{{0, 34, "http://www.google.com/search?q=commonmark"}}},
		{"http://commonmark.org", []cmarkLink{{0, 21, "http://commonmark.org"}}},
		{"(Visit https://encrypted.google.com/search?q=Markup+(business))",
			[]cmarkLink{{7, 62, "https://encrypted.google.com/search?q=Markup+(business)"}}},
		{"See user@example.com&#97; now", []cmarkLink{{4, 25, "mailto:user@example.coma"}}},
		{"See https://example.com/a\u00a0b here", []cmarkLink{{4, 28, "https://example.com/a\u00a0b"}}},
		{"xmpp&#58;user@example.com", []cmarkLink{{0, 25, "xmpp:user@example.com"}}},
		// check_domain counts an underscore after the domain, unless it is the
		// last byte of the content, which it never looks at.
		{"_https://example.com_ now", nil},
		{"_https://example.com_", []cmarkLink{{1, 20, "https://example.com"}}},
		{"www.foo@example.com", []cmarkLink{{0, 19, "http://www.foo@example.com"}}},
		{"https://example.com/a]", []cmarkLink{{0, 22, "https://example.com/a]"}}},
		{"see <https://example.com> now", nil},
		{"`https://example.com`", nil},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, cmarkAutolinks([]byte(tt.content)), "%q", tt.content)
	}
}
