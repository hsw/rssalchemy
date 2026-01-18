package pwextractor

import (
	"fmt"
	"github.com/egor3f/rssalchemy/internal/models"
	"github.com/ericchiang/css"
	"github.com/labstack/gommon/log"
	"golang.org/x/net/html"
	"strings"
)

type htmlParser struct {
	task       models.Task
	dateParser DateParser
	baseURL    *urlParts
}

func (p *htmlParser) parse(htmlStr string) (*models.TaskResult, error) {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	var result models.TaskResult
	result.Title = textFromSelector(doc, "title")

	icon, err := firstAttr(doc, "link[rel=apple-touch-icon]", "href")
	if err != nil {
		log.Warnf("page icon url: %v", err)
	} else if icon != "" {
		result.Icon = absURL(icon, p.baseURL)
	}

	postNodes, err := selectNodes(doc, p.task.SelectorPost)
	if err != nil {
		return nil, fmt.Errorf("post selector: %w", err)
	}
	if len(postNodes) == 0 {
		return nil, fmt.Errorf("no posts on page")
	}
	log.Debugf("Posts count=%d", len(postNodes))

	for _, post := range postNodes {
		item, err := p.extractPost(post)
		if err != nil {
			log.Errorf("extract post fields: %v", err)
			continue
		}
		if len(item.Title) == 0 || len(item.Link) == 0 || item.Created.IsZero() {
			log.Warnf("post has no required fields, skip")
			continue
		}
		result.Items = append(result.Items, item)
	}
	if len(result.Items) == 0 {
		return nil, fmt.Errorf("extract failed for all posts")
	}
	return &result, nil
}

func (p *htmlParser) extractPost(post *html.Node) (models.FeedItem, error) {
	var item models.FeedItem

	item.Title = textFromSelector(post, p.task.SelectorTitle)
	log.Debugf("---- POST: %s ----", item.Title)

	item.Link = absURL(attrFromSelector(post, p.task.SelectorLink, "href"), p.baseURL)

	if len(p.task.SelectorDescription) > 0 {
		item.Description = textFromSelector(post, p.task.SelectorDescription)
	}

	if len(p.task.SelectorAuthor) > 0 {
		item.AuthorName = textFromSelector(post, p.task.SelectorAuthor)
		item.AuthorLink = absURL(attrFromSelector(post, p.task.SelectorAuthor, "href"), p.baseURL)
	}

	if len(p.task.SelectorContent) > 0 {
		item.Content = extractContentFromSelector(post, p.task.SelectorContent, p.baseURL)
	}

	if len(p.task.SelectorEnclosure) > 0 {
		item.Enclosure = attrFromSelector(post, p.task.SelectorEnclosure, "src")
	}

	createdDateStr := ""
	switch p.task.CreatedExtractFrom {
	case models.ExtractFrom_InnerText:
		createdDateStr = textFromSelector(post, p.task.SelectorCreated)
	case models.ExtractFrom_Attribute:
		createdDateStr = attrFromSelector(post, p.task.SelectorCreated, p.task.CreatedAttributeName)
	default:
		return models.FeedItem{}, fmt.Errorf("invalid task.CreatedExtractFrom")
	}
	log.Debugf("date=%s", createdDateStr)
	createdDate, err := p.dateParser.ParseDate(createdDateStr)
	if err != nil {
		log.Errorf("dateparser: %v", err)
	} else {
		item.Created = createdDate
	}

	return item, nil
}

func selectNodes(root *html.Node, selector string) ([]*html.Node, error) {
	if strings.TrimSpace(selector) == "" {
		return nil, fmt.Errorf("selector is empty")
	}
	sel, err := css.Parse(selector)
	if err != nil {
		return nil, err
	}
	return sel.Select(root), nil
}

func firstNode(root *html.Node, selector string) (*html.Node, error) {
	nodes, err := selectNodes(root, selector)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}
	return nodes[0], nil
}

func textFromSelector(root *html.Node, selector string) string {
	node, err := firstNode(root, selector)
	if err != nil || node == nil {
		return ""
	}
	return nodeText(node)
}

func attrFromSelector(root *html.Node, selector string, attrName string) string {
	node, err := firstNode(root, selector)
	if err != nil || node == nil {
		return ""
	}
	return nodeAttr(node, attrName)
}

func firstAttr(root *html.Node, selector string, attrName string) (string, error) {
	node, err := firstNode(root, selector)
	if err != nil {
		return "", err
	}
	if node == nil {
		return "", nil
	}
	return nodeAttr(node, attrName), nil
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
			b.WriteString(" ")
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return strings.TrimSpace(b.String())
}

func nodeAttr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}
	return ""
}
