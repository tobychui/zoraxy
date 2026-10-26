package errorpage

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"imuslab.com/zoraxy/mod/info/logger"
)

/*
	Error Page Template Manager

	This module manages the error page templates stored in the
	templates folder of the web directory (e.g. ./www/templates)
	and decides which template to serve when Zoraxy generates
	an error response by itself.

	Template lookup order of an action
	1. Template assigned to the action in the template manager
	2. Legacy template of the action (e.g. whitelist.html)
	3. Status code template (e.g. 403.html)
	4. Built-in template
*/

const (
	AssignBuiltin    = "@builtin"      //Assignment value that forces an action to use the built-in template
	templateCacheTTL = 3 * time.Second //Pick up template changes made outside of the template manager
)

type Options struct {
	TemplateDir string         //Folder that stores the templates, e.g. ./www/templates
	ConfigFile  string         //JSON file that stores the action to template assignments
	Logger      *logger.Logger //Logger, can be nil
}

type Manager struct {
	options     *Options
	assignments map[Action]string //Action to template name, missing entry means default lookup
	assignMux   sync.RWMutex
	cache       map[string]*cachedTemplate
	cacheMux    sync.RWMutex
}

type cachedTemplate struct {
	content  []byte //nil if the template does not exist
	loadedAt time.Time
}

type managerConfig struct {
	Assignments map[Action]string
}

// ActionStatus is the assignment state of an action
type ActionStatus struct {
	*ActionInfo
	Assigned  string //Assigned template, empty for default lookup
	Effective string //Template that will be served, empty if the built-in template is used
}

// Create a new error page template manager
func NewManager(options *Options) *Manager {
	m := &Manager{
		options:     options,
		assignments: map[Action]string{},
		cache:       map[string]*cachedTemplate{},
	}

	if err := m.loadConfig(); err != nil {
		m.log("Unable to load error page template assignments", err)
	}
	return m
}

/*
	Serving
*/

// Write the error page of an action to w. Use the action default status code if statusCode <= 0
// A nil manager serves the built-in templates
func (m *Manager) ServeErrorPage(w http.ResponseWriter, action Action, statusCode int) {
	if statusCode <= 0 {
		statusCode = http.StatusInternalServerError
		if info := GetActionInfo(action); info != nil && info.StatusCode > 0 {
			statusCode = info.StatusCode
		}
	}

	content, _ := m.Resolve(action, statusCode)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	w.Write(RenderPlaceholders(content, statusCode))
}

// Resolve the template to serve for an action and status code
// Return the template content and its name, the name is empty if the built-in template is used
func (m *Manager) Resolve(action Action, statusCode int) ([]byte, string) {
	info := GetActionInfo(action)
	if m == nil {
		return getActionBuiltin(info), ""
	}

	assigned := m.GetAssignment(action)
	if assigned == AssignBuiltin {
		return getActionBuiltin(info), ""
	}

	candidates := []string{}
	if assigned != "" {
		candidates = append(candidates, assigned)
	}
	if info != nil && info.LegacyTemplate != "" {
		candidates = append(candidates, info.LegacyTemplate)
	}
	if IsErrorStatusCode(statusCode) {
		candidates = append(candidates, strconv.Itoa(statusCode)+".html")
	}

	for _, name := range candidates {
		if content := m.loadTemplate(name); content != nil {
			return content, name
		}
	}
	return getActionBuiltin(info), ""
}

// Replace the placeholders in a template with the response info
func RenderPlaceholders(content []byte, statusCode int) []byte {
	if !bytes.Contains(content, []byte("{{")) {
		return content
	}

	statusText := http.StatusText(statusCode)
	if statusText == "" {
		statusText = "Error"
	}
	replacer := strings.NewReplacer(
		"{{status_code}}", strconv.Itoa(statusCode),
		"{{status_text}}", statusText,
	)
	return []byte(replacer.Replace(string(content)))
}

// Load a template from the template folder with cache, return nil if it does not exist
func (m *Manager) loadTemplate(name string) []byte {
	m.cacheMux.RLock()
	cached, ok := m.cache[name]
	m.cacheMux.RUnlock()
	if ok && time.Since(cached.loadedAt) < templateCacheTTL {
		return cached.content
	}

	var content []byte
	if templatePath, err := m.templatePath(name); err == nil {
		if c, err := os.ReadFile(templatePath); err == nil {
			content = c
		}
	}

	m.cacheMux.Lock()
	m.cache[name] = &cachedTemplate{content: content, loadedAt: time.Now()}
	m.cacheMux.Unlock()
	return content
}

func (m *Manager) invalidateCache(names ...string) {
	m.cacheMux.Lock()
	for _, name := range names {
		delete(m.cache, name)
	}
	m.cacheMux.Unlock()
}

/*
	Action Assignments
*/

// Get the template assigned to an action, empty if using default lookup
func (m *Manager) GetAssignment(action Action) string {
	m.assignMux.RLock()
	defer m.assignMux.RUnlock()
	return m.assignments[action]
}

// Assign a template to an action. Use empty string for default lookup or AssignBuiltin for built-in template
func (m *Manager) SetAssignment(action Action, templateName string) error {
	if GetActionInfo(action) == nil {
		return errors.New("action not found")
	}

	if templateName != "" && templateName != AssignBuiltin {
		if _, err := m.ReadTemplate(templateName); err != nil {
			return err
		}
	}

	m.assignMux.Lock()
	if templateName == "" {
		delete(m.assignments, action)
	} else {
		m.assignments[action] = templateName
	}
	m.assignMux.Unlock()
	return m.saveConfig()
}

// List all actions with their assignment state
func (m *Manager) ListActions() []*ActionStatus {
	results := []*ActionStatus{}
	for _, info := range actions {
		_, effective := m.Resolve(info.ID, info.StatusCode)
		results = append(results, &ActionStatus{
			ActionInfo: info,
			Assigned:   m.GetAssignment(info.ID),
			Effective:  effective,
		})
	}
	return results
}

// Update the assignments after a template is renamed or removed (newName = "")
func (m *Manager) replaceAssignedTemplate(oldName string, newName string) error {
	changed := false
	m.assignMux.Lock()
	for action, assigned := range m.assignments {
		if assigned != oldName {
			continue
		}
		if newName == "" {
			delete(m.assignments, action)
		} else {
			m.assignments[action] = newName
		}
		changed = true
	}
	m.assignMux.Unlock()

	if !changed {
		return nil
	}
	return m.saveConfig()
}

func (m *Manager) loadConfig() error {
	if m.options.ConfigFile == "" {
		return nil
	}
	content, err := os.ReadFile(m.options.ConfigFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}

	conf := managerConfig{}
	if err := json.Unmarshal(content, &conf); err != nil {
		return err
	}

	m.assignMux.Lock()
	defer m.assignMux.Unlock()
	for action, templateName := range conf.Assignments {
		//Drop entries that are edited into something invalid
		if GetActionInfo(action) == nil {
			continue
		}
		if templateName != AssignBuiltin && ValidateTemplateName(templateName) != nil {
			continue
		}
		m.assignments[action] = templateName
	}
	return nil
}

func (m *Manager) saveConfig() error {
	if m.options.ConfigFile == "" {
		return nil
	}
	m.assignMux.RLock()
	js, err := json.MarshalIndent(managerConfig{Assignments: m.assignments}, "", "  ")
	m.assignMux.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(m.options.ConfigFile, js, 0775)
}

func (m *Manager) log(message string, err error) {
	if m.options.Logger != nil {
		m.options.Logger.PrintAndLog("error-page", message, err)
	}
}
