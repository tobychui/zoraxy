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
	/*
		// Replace h* elements
		doc.Find("h1").Each(func(i int, s *goquery.Selection) {
			originalHeader, err := s.Html()
			if err != nil {
				fmt.Println("Error getting header HTML:", err)
				return
			}

			// Patch the bug in the parser that converts " />" to "/>"
			originalHeader = strings.ReplaceAll(originalHeader, "/>", " />")
			wrappedHeader := fmt.Sprintf("<div class=\"ts-header is-big\">%s</div>", originalHeader)
			originalHTMLContent = strings.ReplaceAll(originalHTMLContent, s.Text(), wrappedHeader)
		})
	*/
	doc.Find("h2").Each(func(i int, s *goquery.Selection) {
		originalHeader, err := s.Html()
		if err != nil {
			fmt.Println("Error getting header HTML:", err)
			return
		}

		// Patch the bug in the parser that converts " />" to "/>"
		originalHeader = strings.ReplaceAll(originalHeader, "/>", " />")
		wrappedHeader := fmt.Sprintf("<div class=\"ts-header is-large\">%s</div>", originalHeader)
		originalHTMLContent = strings.ReplaceAll(originalHTMLContent, s.Text(), wrappedHeader)
	})

	doc.Find("h3").Each(func(i int, s *goquery.Selection) {
		originalHeader, err := s.Html()
		if err != nil {
			fmt.Println("Error getting header HTML:", err)
			return
		}

		// Patch the bug in the parser that converts " />" to "/>"
		originalHeader = strings.ReplaceAll(originalHeader, "/>", " />")
		wrappedHeader := fmt.Sprintf("<div class=\"ts-header\">%s</div>", originalHeader)
		originalHTMLContent = strings.ReplaceAll(originalHTMLContent, s.Text(), wrappedHeader)
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
		originalHTMLContent = strings.ReplaceAll(originalHTMLContent, "<hr>", "<div class=\"ts-divider\"></div>")
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

	//Replace blockquote to <div class="ts-quote">
	originalHTMLContent = strings.ReplaceAll(originalHTMLContent, "<blockquote>", "<div class=\"ts-quote\">")
	originalHTMLContent = strings.ReplaceAll(originalHTMLContent, "</blockquote>", "</div>")

	return []byte(originalHTMLContent), err
}
