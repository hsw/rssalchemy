package pwextractor

import (
	"golang.org/x/net/html"
	"strings"
)

var allowedMarkupTags = map[string]bool{
	"b":      true,
	"i":      true,
	"strong": true,
}

func extractContentFromSelector(root *html.Node, selector string, baseURL *urlParts) string {
	node, err := firstNode(root, selector)
	if err != nil || node == nil {
		return ""
	}
	return extractContent(node, baseURL)
}

func extractContent(root *html.Node, baseURL *urlParts) string {
	var content strings.Builder
	var paragraph strings.Builder

	finishParagraph := func() {
		text := strings.TrimSpace(paragraph.String())
		if text == "" {
			paragraph.Reset()
			return
		}
		content.WriteString("<p>")
		content.WriteString(text)
		content.WriteString("</p>")
		paragraph.Reset()
	}

	addImage := func(src string) {
		src = absURL(src, baseURL)
		if src == "" {
			return
		}
		content.WriteString(`<img src="`)
		content.WriteString(src)
		content.WriteString(`"/>`)
	}

	var walk func(node *html.Node)
	walk = func(node *html.Node) {
		switch node.Type {
		case html.ElementNode:
			tag := strings.ToLower(node.Data)
			if tag == "img" {
				finishParagraph()
				addImage(nodeAttr(node, "src"))
				return
			}
			if allowedMarkupTags[tag] {
				paragraph.WriteString("<")
				paragraph.WriteString(tag)
				paragraph.WriteString(">")
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
			if allowedMarkupTags[tag] {
				paragraph.WriteString("</")
				paragraph.WriteString(tag)
				paragraph.WriteString(">")
			}
		case html.TextNode:
			if strings.TrimSpace(node.Data) != "" {
				paragraph.WriteString(node.Data)
				paragraph.WriteString(" ")
			}
		}
	}

	for child := root.FirstChild; child != nil; child = child.NextSibling {
		walk(child)
	}
	finishParagraph()
	return content.String()
}

