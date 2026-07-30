package pages

import "strings"

// Default builds the page every event has before any host touches it.
//
// This is requirement (f) of the host-pages brief made concrete: an event that
// has never been configured still has a real page, built from the event record
// the organiser already filled in, with no theme to choose, no template to
// upload, no third-party fetch and no network call of any kind. A host who
// never opens the page editor is not penalised with a blank page.
//
// It is also the reference document for the format: whatever Default returns
// is, by construction, a document a host could have submitted themselves. There
// is no privileged block type that only Cackle may use.
func Default(ev Event) Document {
	d := Document{Version: Version}

	if paras := paragraphsOf(ev.Description); len(paras) > 0 {
		d.Blocks = append(d.Blocks, Block{Type: BlockText, Paragraphs: paras})
	}
	d.Blocks = append(d.Blocks,
		Block{Type: BlockDetails},
		Block{Type: BlockTickets},
	)
	return d
}

// paragraphsOf splits an event's free-text description into paragraphs the
// document format accepts, and drops anything it does not.
//
// The description column is ordinary user text that predates this package, so
// it can contain line breaks, over-long runs and — since nothing ever stopped
// it — markup or control characters. Rather than trust it, each candidate
// paragraph is put through the same plainText gate a host's own submission
// faces, and one that fails is skipped. The default page therefore cannot be a
// way to get content onto a page that the host-facing API would have refused.
//
// Over-long paragraphs are truncated on a rune boundary rather than dropped
// wholesale: losing the tail of a long description is a better failure than
// losing the whole thing, and truncation cannot introduce anything that was
// not already accepted.
func paragraphsOf(description string) []string {
	if strings.TrimSpace(description) == "" {
		return nil
	}
	// Normalise CRLF first so a Windows-authored description splits the same
	// way as a Unix-authored one.
	norm := strings.ReplaceAll(description, "\r\n", "\n")

	var out []string
	for _, chunk := range strings.Split(norm, "\n\n") {
		// Any surviving single newlines become spaces: the format expresses
		// line structure with separate paragraphs, never with characters
		// inside one.
		p := strings.Join(strings.Fields(strings.ReplaceAll(chunk, "\n", " ")), " ")
		if p == "" {
			continue
		}
		p = truncateRunes(p, MaxParagraphRunes)
		if err := plainText("description", p, MaxParagraphRunes); err != nil {
			continue
		}
		out = append(out, p)
		if len(out) == MaxParagraphs {
			break
		}
	}
	return out
}

func truncateRunes(s string, max int) string {
	n := 0
	for i := range s {
		if n == max {
			return s[:i]
		}
		n++
	}
	return s
}
