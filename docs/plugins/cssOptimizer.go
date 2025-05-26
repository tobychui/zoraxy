package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func optimizeCss(htmlContent []byte) ([]byte, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(htmlContent)))
	if err != nil {
		return nil, err
	}

	originalHTMLContent := string(htmlContent)
	// Replace img elements

	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		//For each of the image element, replace the parent from p to div
		orginalParent, err := s.Parent().Html()
		if err != nil {
			fmt.Println("Error getting parent HTML:", err)
			return
		}

		src, exists := s.Attr("src")
		if !exists {
			fmt.Println("No src attribute found for img element")
			return
		}
		encodedSrc := (&url.URL{Path: src}).String()

		//Patch the bug in the parser that converts " />" to "/>"
		orginalParent = strings.ReplaceAll(orginalParent, "/>", " />")
		fmt.Println("<div class=\"ts-image is-rounded\"><img src=\"./" + encodedSrc + "\"></div>")
		//Replace the img with ts-image
		originalHTMLContent = strings.Replace(originalHTMLContent, orginalParent, "<div class=\"ts-image is-rounded\" style=\"max-width: 800px\">"+orginalParent+"</div>", 1)
	})

	// Add "ts-text" class to each p element
	doc.Find("p").Each(func(i int, s *goquery.Selection) {
		class, exists := s.Attr("class")
		var newClass string
		if exists {
			newClass = fmt.Sprintf("%s ts-text", class)
		} else {
			newClass = "ts-text"
		}

		originalParagraph, _ := s.Html()
		originalHTMLContent = strings.ReplaceAll(originalHTMLContent, originalParagraph, fmt.Sprintf("<p class=\"%s\">%s</p>", newClass, originalParagraph))
	})

	//Replace hr with ts-divider
	// Replace hr elements outside of code blocks
	doc.Find("hr").Each(func(i int, s *goquery.Selection) {
		parent := s.Parent()
		if parent.Is("code") {
			// Skip <hr> inside <code> blocks
			return
		}

		// Replace <hr> with <div class="ts-divider"></div>
		originalHTMLContent = strings.ReplaceAll(originalHTMLContent, "<hr>", "<div class=\"ts-divider has-top-spaced-large\"></div>")
	})

	// Add ts-table to all table elements
	doc.Find("table").Each(func(i int, s *goquery.Selection) {
		class, exists := s.Attr("class")
		var newClass string
		if exists {
			newClass = fmt.Sprintf("%s ts-table", class)
		} else {
			newClass = "ts-table"
		}

		originalTable, _ := s.Html()
		originalHTMLContent = strings.ReplaceAll(originalHTMLContent, originalTable, fmt.Sprintf("<table class=\"%s\">%s</table>", newClass, originalTable))
	})

	// Replace <ul> <ol> and <li>
	originalHTMLContent = strings.ReplaceAll(originalHTMLContent, "<ul>", "<div class=\"ts-list is-unordered\">")
	originalHTMLContent = strings.ReplaceAll(originalHTMLContent, "</ul>", "</div>")
	originalHTMLContent = strings.ReplaceAll(originalHTMLContent, "<ol>", "<div class=\"ts-list is-ordered\">")
	originalHTMLContent = strings.ReplaceAll(originalHTMLContent, "</ol>", "</div>")
	originalHTMLContent = strings.ReplaceAll(originalHTMLContent, "<li>", "<div class=\"item\">")
	originalHTMLContent = strings.ReplaceAll(originalHTMLContent, "</li>", "</div>")

	// Replace <strong> with <span class="ts-text is-heavy"></span>
	originalHTMLContent = strings.ReplaceAll(originalHTMLContent, "<strong>", "<span class=\"ts-text is-heavy\">")
	originalHTMLContent = strings.ReplaceAll(originalHTMLContent, "</strong>", "</span>")

	// Replace <code> without class with <span class="ts-text is-code">
	for {
		startIndex := strings.Index(originalHTMLContent, "<code>")
		if startIndex == -1 {
			break
		}

		endIndex := strings.Index(originalHTMLContent[startIndex+6:], "</code>")
		if endIndex == -1 {
			break
		}
		endIndex += startIndex + 6

		codeSegment := originalHTMLContent[startIndex : endIndex+7] // Include </code>
		fmt.Println(">>>>", codeSegment)
		if !strings.Contains(codeSegment, "class=") {
			replacement := strings.Replace(codeSegment, "<code>", "<span class=\"ts-text is-code\">", 1)
			replacement = strings.Replace(replacement, "</code>", "</span>", 1)
			originalHTMLContent = strings.Replace(originalHTMLContent, codeSegment, replacement, 1)
		} else {
			// Skip if <code> already has a class
			break
		}
	}

	//Replace blockquote to <div class="ts-quote">
	originalHTMLContent = strings.ReplaceAll(originalHTMLContent, "<blockquote>", "<div class=\"ts-quote\">")
	originalHTMLContent = strings.ReplaceAll(originalHTMLContent, "</blockquote>", "</div>")

	return []byte(originalHTMLContent), err
}
