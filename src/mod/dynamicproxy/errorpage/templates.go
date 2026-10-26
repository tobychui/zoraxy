package errorpage

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

/*
	Template File Operations

	All file operations must go through templatePath() which
	validates the template name and makes sure the resolved
	path stays inside the template folder
*/

const MaxTemplateSize = 1 << 20 //1MB

var (
	ErrInvalidTemplateName = errors.New("invalid template name, only letters, numbers, dash, underscore and dot are allowed and it must end with .html")
	ErrTemplateNotFound    = errors.New("template not found")
	ErrTemplateExists      = errors.New("template with the same name already exists")
	ErrTemplateNotRegular  = errors.New("template is not a regular file")
	ErrTemplateTooLarge    = errors.New("template is too large")
)

var (
	templateNameRegex       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\-]{0,63}\.html$`)
	statusCodeTemplateRegex = regexp.MustCompile(`^([45][0-9]{2})\.html$`)
	windowsReservedNames    = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[0-9]|lpt[0-9])$`)
)

// TemplateInfo is the metadata of a template file
type TemplateInfo struct {
	Name       string
	Size       int64
	ModTime    int64
	StatusCode int      //Status code if this is a generic status code template (e.g. 404.html), otherwise 0
	UsedBy     []string //Name of the actions or system functions currently serving this template
}

// Check if the status code is an error status code that can have a template
func IsErrorStatusCode(statusCode int) bool {
	return statusCode >= 400 && statusCode <= 599
}

// Normalize the template name from user input, append .html if missing
func NormalizeTemplateName(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasSuffix(strings.ToLower(name), ".html") {
		return name[:len(name)-len(".html")] + ".html"
	}
	return name + ".html"
}

// Validate a template name. Template names must be a plain filename inside the template folder
func ValidateTemplateName(name string) error {
	if !templateNameRegex.MatchString(name) || strings.Contains(name, "..") {
		return ErrInvalidTemplateName
	}

	//Windows device names are reserved even with an extension
	basename := strings.SplitN(name, ".", 2)[0]
	if windowsReservedNames.MatchString(basename) {
		return ErrInvalidTemplateName
	}

	if filepath.Base(name) != name {
		return ErrInvalidTemplateName
	}
	return nil
}

// Get the status code of a status code template (e.g. 404.html), return 0 if not a status code template
func GetTemplateStatusCode(name string) int {
	matches := statusCodeTemplateRegex.FindStringSubmatch(name)
	if len(matches) != 2 {
		return 0
	}
	code, _ := strconv.Atoi(matches[1])
	return code
}

// Resolve the absolute path of a template and check it does not escape the template folder
func (m *Manager) templatePath(name string) (string, error) {
	if err := ValidateTemplateName(name); err != nil {
		return "", err
	}

	baseDir, err := filepath.Abs(m.options.TemplateDir)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(baseDir, name))
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(baseDir, target)
	if err != nil || rel != name || filepath.Dir(target) != baseDir {
		return "", ErrInvalidTemplateName
	}
	return target, nil
}

// Resolve the template path and make sure it is a regular file (not a symlink or folder)
func (m *Manager) existingTemplatePath(name string) (string, error) {
	templatePath, err := m.templatePath(name)
	if err != nil {
		return "", err
	}

	info, err := os.Lstat(templatePath)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrTemplateNotFound
	} else if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", ErrTemplateNotRegular
	}
	return templatePath, nil
}

// List all templates in the template folder
func (m *Manager) ListTemplates() ([]*TemplateInfo, error) {
	results := []*TemplateInfo{}
	entries, err := os.ReadDir(m.options.TemplateDir)
	if errors.Is(err, os.ErrNotExist) {
		return results, nil
	} else if err != nil {
		return nil, err
	}

	//Map of template name to the actions serving it
	usedBy := map[string][]string{}
	for _, status := range m.ListActions() {
		if status.Effective != "" {
			usedBy[status.Effective] = append(usedBy[status.Effective], status.Name)
		}
	}

	for _, entry := range entries {
		//Skip folders, symlinks and files that cannot be managed (e.g. .tmpl captcha templates)
		if !entry.Type().IsRegular() || ValidateTemplateName(entry.Name()) != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}

		templateInfo := &TemplateInfo{
			Name:       entry.Name(),
			Size:       info.Size(),
			ModTime:    info.ModTime().Unix(),
			StatusCode: GetTemplateStatusCode(entry.Name()),
			UsedBy:     usedBy[entry.Name()],
		}
		if systemUsage, ok := otherSystemTemplates[entry.Name()]; ok {
			templateInfo.UsedBy = append(templateInfo.UsedBy, systemUsage)
		}
		if templateInfo.UsedBy == nil {
			templateInfo.UsedBy = []string{}
		}
		results = append(results, templateInfo)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	return results, nil
}

// Read the content of a template
func (m *Manager) ReadTemplate(name string) ([]byte, error) {
	templatePath, err := m.existingTemplatePath(name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(templatePath)
}

// Write a template, set createOnly to prevent overwriting an existing template
func (m *Manager) WriteTemplate(name string, content []byte, createOnly bool) error {
	if len(content) > MaxTemplateSize {
		return ErrTemplateTooLarge
	}

	templatePath, err := m.templatePath(name)
	if err != nil {
		return err
	}

	info, err := os.Lstat(templatePath)
	if err == nil {
		if createOnly {
			return ErrTemplateExists
		}
		if !info.Mode().IsRegular() {
			return ErrTemplateNotRegular
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	templateDir := filepath.Dir(templatePath)
	if err := os.MkdirAll(templateDir, 0775); err != nil {
		return err
	}

	//Write to a temp file and rename, so the proxy never serves a half written template
	tmpFile, err := os.CreateTemp(templateDir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	_, err = tmpFile.Write(content)
	if closeErr := tmpFile.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tmpName, 0664)
	}
	if err == nil {
		err = os.Rename(tmpName, templatePath)
	}
	if err != nil {
		os.Remove(tmpName)
		return err
	}

	m.invalidateCache(name)
	return nil
}

// Delete a template, actions assigned to it fall back to the default lookup
func (m *Manager) DeleteTemplate(name string) error {
	templatePath, err := m.existingTemplatePath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(templatePath); err != nil {
		return err
	}

	m.invalidateCache(name)
	return m.replaceAssignedTemplate(name, "")
}

// Rename a template, actions assigned to it follow the new name
func (m *Manager) RenameTemplate(oldName string, newName string) error {
	oldPath, err := m.existingTemplatePath(oldName)
	if err != nil {
		return err
	}
	newPath, err := m.templatePath(newName)
	if err != nil {
		return err
	}

	if _, err := os.Lstat(newPath); err == nil {
		return ErrTemplateExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}

	m.invalidateCache(oldName, newName)
	return m.replaceAssignedTemplate(oldName, newName)
}
