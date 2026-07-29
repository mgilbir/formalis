package formalis

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Format detection, and why it does not build a tree.
//
// The twelve Is* predicates answer "which format is this", which every one of
// them decides from the root element name plus a handful of things immediately
// under it — the presence of a distinguishing child, the text of a
// CustomizationID or ProfileID, or (ZATCA alone) an AdditionalDocumentReference
// carrying a particular ID.
//
// They used to get that by calling parseCII and reading two fields off the
// result, which meant materialising every element in the document to look at a
// name. That made them the cheapest way to reach the memory amplification
// maxNodes now bounds: a 60 MB document cost about 2.4 GB to answer a question
// about its first tag.
//
// docShape is what the predicates actually consult, and scanShape fills it in a
// single pass that retains only the open elements — bounded by maxDepth — and
// the few strings below. Nothing accumulates per element, so the cost is set by
// the nesting rather than by the size, and detection no longer needs a node
// budget at all: there is no tree to bound.
//
// Note what does *not* work: parsing into a tree but capping it at depth 2. In
// the document that motivates this, the millions of siblings *are* the root's
// direct children, so a depth-capped tree retains all of them. The scan has to
// inspect and discard, which is what it does.
//
// The scan reads to end of input rather than stopping once it has what it
// needs. Detection has to be able to say "I could not read this" — that is what
// the error return of every predicate is for — and a document whose first tag
// is fine but whose remainder is truncated or malformed is exactly such a case.
// Stopping early would report it as readable, which is the collapse the error
// return exists to prevent.

// docShape is the part of a document the Is* predicates examine.
type docShape struct {
	// root is the local name of the root element.
	root string
	// The distinguishing children the predicates test for by presence.
	hasNaglowek    bool
	hasBiller      bool
	hasSellerParty bool
	// The text of the first direct child of the root with each name, matching
	// what root.str(name) returned from the tree.
	customizationID string
	profileID       string
	gotCustomID     bool
	gotProfileID    bool
	// icvDocRef records an AdditionalDocumentReference, at any depth, whose
	// first direct ID child reads "ICV" — what zatcaDocRef looked for.
	icvDocRef bool
}

// The elements whose text the scan keeps. Anything else is discarded as it is
// read.
const (
	captureNone = iota
	captureCustomizationID
	captureProfileID
	captureDocRefID
)

// scanFrame is one open element. It holds the name, because a child's meaning
// depends on its parent, and a text accumulator only for the elements listed
// above — so a document of millions of siblings allocates nothing per sibling.
type scanFrame struct {
	name string
	// capture says which docShape field this element's text feeds, or
	// captureNone.
	capture int
	buf     []byte
	// gotID marks an AdditionalDocumentReference whose first ID child has
	// already been read, so a second one is ignored the way child() ignored it.
	gotID bool
}

// scanShape reads xmlData once and reports what the Is* predicates need to know
// about it, or an error if it could not be read.
//
// It mirrors parseCII's acceptance exactly — the same decoder, the same charset
// reader, the same depth cap, the same treatment of an empty document — so the
// predicates changed only in what they cost, not in which documents they
// accept. An element's text is the character data directly inside it, excluding
// its descendants', which is what ciiNode.text held.
func scanShape(xmlData []byte) (*docShape, error) {
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	dec.CharsetReader = xmlCharsetReader

	var d docShape
	var stack []scanFrame
	seenRoot := false

	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(stack) >= maxDepth {
				return nil, fmt.Errorf("the XML nests deeper than %d elements", maxDepth)
			}
			name := t.Name.Local
			f := scanFrame{name: name}

			switch {
			case len(stack) == 0:
				// Go's decoder accepts more than one top-level element, and
				// parseCII overwrites its root each time it sees one, so the
				// tree the validators check is built from the *last*. Detection
				// has to land on the same document or a predicate would route
				// to a validator that then reports a different root — so
				// everything gathered from an earlier top-level element is
				// discarded here, exactly as the tree discarded it.
				d = docShape{root: name}
				seenRoot = true
			case len(stack) == 1:
				// A direct child of the root.
				switch name {
				case "Naglowek":
					d.hasNaglowek = true
				case "Biller":
					d.hasBiller = true
				case "SellerParty":
					d.hasSellerParty = true
				case "CustomizationID":
					if !d.gotCustomID {
						f.capture = captureCustomizationID
					}
				case "ProfileID":
					if !d.gotProfileID {
						f.capture = captureProfileID
					}
				}
			}
			// zatcaDocRef searched the whole tree, so this is not restricted to
			// a particular depth.
			if name == "ID" && len(stack) > 0 {
				if p := &stack[len(stack)-1]; p.name == "AdditionalDocumentReference" && !p.gotID {
					p.gotID = true
					f.capture = captureDocRefID
				}
			}
			stack = append(stack, f)

		case xml.EndElement:
			if len(stack) > 0 {
				d.close(&stack[len(stack)-1])
				stack = stack[:len(stack)-1]
			}

		case xml.CharData:
			if len(stack) > 0 {
				if f := &stack[len(stack)-1]; f.capture != captureNone {
					f.buf = append(f.buf, t...)
				}
			}
		}
	}
	// Elements left open at end of input never saw their EndElement. parseCII
	// materialised their text too.
	for i := range stack {
		d.close(&stack[i])
	}
	if !seenRoot {
		return nil, fmt.Errorf("no root element")
	}
	return &d, nil
}

// close stores a finished element's captured text.
func (d *docShape) close(f *scanFrame) {
	if f.capture == captureNone {
		return
	}
	text := string(f.buf)
	f.buf = nil
	switch f.capture {
	case captureCustomizationID:
		d.customizationID = text
		d.gotCustomID = true
	case captureProfileID:
		d.profileID = text
		d.gotProfileID = true
	case captureDocRefID:
		if strings.TrimSpace(text) == "ICV" {
			d.icvDocRef = true
		}
	}
	f.capture = captureNone
}

// str reports the trimmed text of the named direct child of the root, matching
// what root.str(name) returned.
func (d *docShape) str(name string) string {
	switch name {
	case "CustomizationID":
		return strings.TrimSpace(d.customizationID)
	case "ProfileID":
		return strings.TrimSpace(d.profileID)
	}
	return ""
}

// detectShape reads xmlData far enough to identify what kind of document it is.
//
// It exists so the Is* predicates share one answer to "could I read this at
// all". They report three outcomes between them — yes, definitively not, and
// could not tell — and the third is what the error carries: XML that is not
// well-formed, an encoding this package does not implement, or a document that
// nests past the cap. Folding any of those into a plain false would tell a
// caller that a truncated Facturae invoice is not a Facturae invoice, which is
// not the same statement and misroutes anything dispatching on the answer.
func detectShape(xmlData []byte) (*docShape, error) {
	d, err := scanShape(xmlData)
	if err != nil {
		return nil, fmt.Errorf("the XML could not be read: %w", err)
	}
	return d, nil
}
