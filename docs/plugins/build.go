package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/yosssi/gohtml"
)

type FileInfo struct {
	Filename string `json:"filename"`
	Title    string `json:"title"`
	Type     string `json:"type"`
}

func build() {
	rootDir := "./docs"
	outputFile := "./index.json"

	type Folder struct {
		Title string        `json:"title"`
		Path  string        `json:"path"`
		Type  string        `json:"type"`
		Files []interface{} `json:"files,omitempty"`
	}

	var buildTree func(path string, d fs.DirEntry) interface{}
	buildTree = func(path string, d fs.DirEntry) interface{} {
		relativePath, _ := filepath.Rel(rootDir, path)
		relativePath = filepath.ToSlash(relativePath)
		var title string
		if d.IsDir() {
			title = filepath.Base(relativePath)
		} else {
			title = strings.TrimSuffix(filepath.Base(relativePath), filepath.Ext(relativePath))
		}

		//Strip the leader numbers from the title, e.g. 1. Introduction -> Introduction
		if strings.Contains(title, ".") {
			parts := strings.SplitN(title, ".", 2)
			if len(parts) > 1 {
				title = strings.TrimSpace(parts[1])
			}
		}
		// Remove leading numbers and dots
		title = strings.TrimLeft(title, "0123456789. ")
		// Remove leading spaces
		title = strings.TrimLeft(title, " ")

		if d.IsDir() {
			folder := Folder{
				Title: title,
				Path:  relativePath,
				Type:  "folder",
			}

			entries, err := os.ReadDir(path)
			if err != nil {
				panic(err)
			}

			for _, entry := range entries {
				if entry.Name() == "img" || entry.Name() == "assets" {
					continue
				}
				if strings.Contains(filepath.ToSlash(filepath.Join(relativePath, entry.Name())), "/img/") || strings.Contains(filepath.ToSlash(filepath.Join(relativePath, entry.Name())), "/assets/") {
					continue
				}
				child := buildTree(filepath.Join(path, entry.Name()), entry)
				if child != nil {
					folder.Files = append(folder.Files, child)
				}
			}
			return folder
		} else {
			ext := filepath.Ext(relativePath)
			if ext != ".md" && ext != ".html" && ext != ".txt" {
				return nil
			}
			return FileInfo{
				Filename: relativePath,
				Title:    title,
				Type:     "file",
			}
		}
	}

	rootInfo, err := os.Stat(rootDir)
	if err != nil {
		panic(err)
	}
	rootFolder := buildTree(rootDir, fs.FileInfoToDirEntry(rootInfo))
	jsonData, err := json.MarshalIndent(rootFolder, "", "  ")
	if err != nil {
		panic(err)
	}

	/* For debug purposes, print the JSON structure */
	err = os.WriteFile(outputFile, jsonData, 0644)
	if err != nil {
		panic(err)
	}

	/* For each file in the folder structure, convert markdown to HTML */
	htmlOutputDir := "./html"
	os.RemoveAll(htmlOutputDir) // Clear previous HTML output
	err = os.MkdirAll(htmlOutputDir, os.ModePerm)
	if err != nil {
		panic(err)
	}

	var processFiles func(interface{})
	processFiles = func(node interface{}) {
		switch n := node.(type) {
		case FileInfo:
			if filepath.Ext(n.Filename) == ".md" {
				inputPath := filepath.Join(rootDir, n.Filename)
				outputPath := filepath.Join(htmlOutputDir, strings.TrimSuffix(n.Filename, ".md")+".html")

				// Ensure the output directory exists
				err := os.MkdirAll(filepath.Dir(outputPath), os.ModePerm)
				if err != nil {
					panic(err)
				}

				// Read the markdown file
				mdContent, err := os.ReadFile(inputPath)
				if err != nil {
					panic(err)
				}

				// Convert markdown to HTML
				docContent := mdToHTML(mdContent)
				docContent, err = optimizeCss(docContent)
				if err != nil {
					panic(err)
				}

				// Load the HTML template
				templateBytes, err := os.ReadFile("template/documents.html")
				if err != nil {
					panic(err)
				}

				// Generate the side menu HTML
				sideMenuHTML, err := generateSideMenu(string(jsonData), n.Title)
				if err != nil {
					panic(err)
				}

				templateBody := string(templateBytes)
				// Replace placeholders in the template
				htmlContent := strings.ReplaceAll(templateBody, "{{title}}", n.Title+" | Zoraxy Documentation")
				htmlContent = strings.ReplaceAll(htmlContent, "{{content}}", string(docContent))
				htmlContent = strings.ReplaceAll(htmlContent, "{{sideMenu}}", sideMenuHTML)
				htmlContent = strings.ReplaceAll(htmlContent, "{{root_url}}", *root_url)
				//Add more if needed

				//Beautify the HTML content
				htmlContent = gohtml.Format(htmlContent)

				// Write the HTML file
				err = os.WriteFile(outputPath, []byte(htmlContent), 0644)
				if err != nil {
					panic(err)
				}

				//Check if the .md file directory have an ./img or ./assets folder. If yes, copy the contents to the output directory
				imgDir := filepath.Join(rootDir, filepath.Dir(n.Filename), "img")
				assetsDir := filepath.Join(rootDir, filepath.Dir(n.Filename), "assets")
				if _, err := os.Stat(imgDir); !os.IsNotExist(err) {
					err = filepath.Walk(imgDir, func(srcPath string, info os.FileInfo, err error) error {
						if err != nil {
							return err
						}
						relPath, err := filepath.Rel(imgDir, srcPath)
						if err != nil {
							return err
						}
						destPath := filepath.Join(filepath.Dir(outputPath), "img", relPath)
						if info.IsDir() {
							return os.MkdirAll(destPath, os.ModePerm)
						}
						data, err := os.ReadFile(srcPath)
						if err != nil {
							return err
						}
						return os.WriteFile(destPath, data, 0644)
					})
					if err != nil {
						panic(err)
					}
				}
				if _, err := os.Stat(assetsDir); !os.IsNotExist(err) {
					err = filepath.Walk(assetsDir, func(srcPath string, info os.FileInfo, err error) error {
						if err != nil {
							return err
						}
						relPath, err := filepath.Rel(assetsDir, srcPath)
						if err != nil {
							return err
						}
						destPath := filepath.Join(filepath.Dir(outputPath), "assets", relPath)
						if info.IsDir() {
							return os.MkdirAll(destPath, os.ModePerm)
						}
						data, err := os.ReadFile(srcPath)
						if err != nil {
							return err
						}
						return os.WriteFile(destPath, data, 0644)
					})
					if err != nil {
						panic(err)
					}
				}

				fmt.Println("Generated HTML:", outputPath)
			}
		case Folder:
			for _, child := range n.Files {
				processFiles(child)
			}
		}
	}

	processFiles(rootFolder)
	copyOtherRes()
}

func copyOtherRes() {
	srcDir := "./assets"
	destDir := "./html/assets"

	err := filepath.Walk(srcDir, func(srcPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(srcDir, srcPath)
		if err != nil {
			return err
		}
		destPath := filepath.Join(destDir, relPath)
		if info.IsDir() {
			return os.MkdirAll(destPath, os.ModePerm)
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		return os.WriteFile(destPath, data, 0644)
	})
	if err != nil {
		panic(err)
	}

}

func mdToHTML(md []byte) []byte {
	// create markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	// create HTML renderer with extensions
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{
		Flags: htmlFlags,
	}
	renderer := html.NewRenderer(opts)

	return markdown.Render(doc, renderer)
}
