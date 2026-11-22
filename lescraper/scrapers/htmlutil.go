package scrapers

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// Small helpers over golang.org/x/net/html for class-based lookups.

type htmlNode = html.Node

func parseHTML(body []byte) (*html.Node, error) {
	return html.Parse(bytes.NewReader(body))
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	if n.Type != html.ElementNode {
		return false
	}
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

// findAll returns descendants of n (not n itself) matching pred, in document
// order, without descending into matches.
func findAll(n *html.Node, pred func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if pred(c) {
			out = append(out, c)
			continue
		}
		out = append(out, findAll(c, pred)...)
	}
	return out
}

// find returns the first descendant matching pred, or nil.
func find(n *html.Node, pred func(*html.Node) bool) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if pred(c) {
			return c
		}
		if m := find(c, pred); m != nil {
			return m
		}
	}
	return nil
}

func byClass(class string) func(*html.Node) bool {
	return func(n *html.Node) bool { return hasClass(n, class) }
}

func byTag(tag string) func(*html.Node) bool {
	return func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == tag }
}

// text returns n's text content with whitespace collapsed. Zero-width
// spaces, which some CMSes leave in titles, are removed.
func text(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return cleanText(b.String())
}

// cleanText collapses whitespace and removes zero-width characters.
func cleanText(s string) string {
	s = strings.NewReplacer("\u200b", "", "\u200c", "", "\u200d", "", "\ufeff", "", "\u00a0", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// textOf returns the text of the first descendant with class, or "".
func textOf(n *html.Node, class string) string {
	return text(find(n, byClass(class)))
}
