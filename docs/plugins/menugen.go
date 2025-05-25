package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type MenuItem struct {
	Title    string     `json:"title"`
	Path     string     `json:"path"`
	Type     string     `json:"type"`
	Files    []MenuItem `json:"files,omitempty"`
	Filename string     `json:"filename,omitempty"`
}

func generateSideMenu(doctreeJson string, selectedTitle string) (string, error) {
	var root MenuItem
	if err := json.Unmarshal([]byte(doctreeJson), &root); err != nil {
		return "", err
	}

	var buffer bytes.Buffer
	buffer.WriteString(`<div class="ts-box">
	<div class="ts-menu is-end-icon">`)

	for _, item := range root.Files {
		writeMenuItem(&buffer, item, selectedTitle)
	}

	buffer.WriteString(`
	</div>
</div>`)

	return buffer.String(), nil
}

func writeMenuItem(buffer *bytes.Buffer, item MenuItem, selectedTitle string) {
	if item.Type == "file" {
		activeClass := ""
		if item.Title == selectedTitle {
			activeClass = " is-active"
		}

		//Generate the URL for the file
		filePath := item.Path
		if item.Filename != "" {
			filePath = fmt.Sprintf("%s/%s", item.Path, strings.ReplaceAll(item.Filename, ".md", ".html"))
		}
		urlPath := filepath.ToSlash(filepath.Clean(*root_url + filePath))
		buffer.WriteString(fmt.Sprintf(`
		<a class="item%s" href="%s">
			%s
		</a>`, activeClass, urlPath, item.Title))
	} else if item.Type == "folder" {
		buffer.WriteString(fmt.Sprintf(`
		<a class="item">
			%s
			<span class="ts-icon is-caret-down-icon"></span>
		</a>`, item.Title))

		if len(item.Files) > 0 {
			buffer.WriteString(`
		<div class="ts-menu is-dense is-small is-horizontally-padded">`)
			for _, subItem := range item.Files {
				writeMenuItem(buffer, subItem, selectedTitle)
			}
			buffer.WriteString(`
		</div>`)
		}
	}
}
