// Package rootfile edits kit blocks inside a target's root instructions file
// (CLAUDE.md, AGENTS.md, ...). The file belongs to the user: kits only ever
// touches the lines between its own markers, one block per kit.
//
// Markers are HTML comments so they stay invisible in rendered Markdown.
package rootfile

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

func BeginMarker(kit string) string { return "<!-- kits:begin " + kit + " -->" }
func EndMarker(kit string) string   { return "<!-- kits:end " + kit + " -->" }

var anyMarkerRe = regexp.MustCompile(`^<!-- kits:(begin|end) \S+ -->$`)

// Upsert replaces kit's block in doc with body, or appends a new block
// separated from existing content by one blank line.
func Upsert(doc []byte, kit string, body []byte) ([]byte, error) {
	for _, line := range strings.Split(string(body), "\n") {
		if anyMarkerRe.MatchString(strings.TrimSpace(line)) {
			return nil, fmt.Errorf("instructions for kit %q contain a kits marker line", kit)
		}
	}
	block := render(kit, body)
	start, end, found, err := find(doc, kit)
	if err != nil {
		return nil, err
	}
	if found {
		return concat(doc[:start], block, doc[end:]), nil
	}
	out := append([]byte(nil), doc...)
	if len(out) > 0 {
		if !bytes.HasSuffix(out, []byte("\n")) {
			out = append(out, '\n')
		}
		if !bytes.HasSuffix(out, []byte("\n\n")) {
			out = append(out, '\n')
		}
	}
	return append(out, block...), nil
}

// Remove deletes kit's block from doc. A doc without the block is returned
// unchanged.
func Remove(doc []byte, kit string) ([]byte, error) {
	start, end, found, err := find(doc, kit)
	if err != nil || !found {
		return doc, err
	}
	out := concat(doc[:start], nil, doc[end:])
	if end == len(doc) {
		// The block was last: drop the blank line Upsert put before it.
		out = bytes.TrimRight(out, "\n")
		if len(out) > 0 {
			out = append(out, '\n')
		}
	}
	return out, nil
}

func render(kit string, body []byte) []byte {
	var b bytes.Buffer
	b.WriteString(BeginMarker(kit) + "\n")
	b.Write(body)
	if len(body) > 0 && !bytes.HasSuffix(body, []byte("\n")) {
		b.WriteByte('\n')
	}
	b.WriteString(EndMarker(kit) + "\n")
	return b.Bytes()
}

// find locates kit's block as byte offsets [start, end), end including the
// end marker's newline. Malformed markers are errors, never guesses: editing
// around them could eat user content.
func find(doc []byte, kit string) (start, end int, found bool, err error) {
	begin, endm := BeginMarker(kit), EndMarker(kit)
	start = -1
	for off := 0; off < len(doc); {
		next := len(doc)
		if nl := bytes.IndexByte(doc[off:], '\n'); nl >= 0 {
			next = off + nl + 1
		}
		switch strings.TrimSpace(string(doc[off:next])) {
		case begin:
			if start >= 0 {
				return 0, 0, false, fmt.Errorf("more than one %s", begin)
			}
			start = off
		case endm:
			if start < 0 {
				return 0, 0, false, fmt.Errorf("%s without a preceding %s", endm, begin)
			}
			if found {
				return 0, 0, false, fmt.Errorf("more than one %s", endm)
			}
			end, found = next, true
		}
		off = next
	}
	if start >= 0 && !found {
		return 0, 0, false, fmt.Errorf("%s has no matching %s", begin, endm)
	}
	return start, end, found, nil
}

func concat(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}
