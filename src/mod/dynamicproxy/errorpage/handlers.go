package errorpage

import (
	"encoding/json"
	"net/http"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	HTTP API Handlers

	Write operations only accept POST so they are covered
	by the CSRF protection middleware
*/

func requirePost(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "405 - Method Not Allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// List all templates in the template folder
func (m *Manager) HandleListTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := m.ListTemplates()
	if err != nil {
		m.log("Unable to list error page templates", err)
		utils.SendErrorResponse(w, "unable to list templates")
		return
	}
	js, _ := json.Marshal(templates)
	utils.SendJSONResponse(w, string(js))
}

// Read the content of a template
func (m *Manager) HandleReadTemplate(w http.ResponseWriter, r *http.Request) {
	name, err := utils.GetPara(r, "name")
	if err != nil {
		utils.SendErrorResponse(w, "template name not defined")
		return
	}

	content, err := m.ReadTemplate(name)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	js, _ := json.Marshal(map[string]string{
		"name":    name,
		"content": string(content),
	})
	utils.SendJSONResponse(w, string(js))
}

// Create or update a template
func (m *Manager) HandleSaveTemplate(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}

	name, err := utils.PostPara(r, "name")
	if err != nil {
		utils.SendErrorResponse(w, "template name not defined")
		return
	}
	name = NormalizeTemplateName(name)

	//Content can be empty, read it from the form directly
	content := r.PostForm.Get("content")
	createOnly, _ := utils.PostBool(r, "create")

	err = m.WriteTemplate(name, []byte(content), createOnly)
	if err != nil {
		if err != ErrInvalidTemplateName && err != ErrTemplateExists && err != ErrTemplateTooLarge && err != ErrTemplateNotRegular {
			m.log("Unable to write error page template "+name, err)
		}
		utils.SendErrorResponse(w, err.Error())
		return
	}

	js, _ := json.Marshal(name)
	utils.SendJSONResponse(w, string(js))
}

// Delete a template
func (m *Manager) HandleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}

	name, err := utils.PostPara(r, "name")
	if err != nil {
		utils.SendErrorResponse(w, "template name not defined")
		return
	}

	if err := m.DeleteTemplate(name); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	utils.SendOK(w)
}

// Rename a template
func (m *Manager) HandleRenameTemplate(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}

	name, err := utils.PostPara(r, "name")
	if err != nil {
		utils.SendErrorResponse(w, "template name not defined")
		return
	}

	newName, err := utils.PostPara(r, "newName")
	if err != nil {
		utils.SendErrorResponse(w, "new template name not defined")
		return
	}
	newName = NormalizeTemplateName(newName)

	if err := m.RenameTemplate(name, newName); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	js, _ := json.Marshal(newName)
	utils.SendJSONResponse(w, string(js))
}

// List all actions and their assigned templates
func (m *Manager) HandleListActions(w http.ResponseWriter, r *http.Request) {
	js, _ := json.Marshal(m.ListActions())
	utils.SendJSONResponse(w, string(js))
}

// Assign a template to an action, leave template empty to restore the default lookup
func (m *Manager) HandleAssignTemplate(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}

	action, err := utils.PostPara(r, "action")
	if err != nil {
		utils.SendErrorResponse(w, "action not defined")
		return
	}

	templateName := r.PostForm.Get("template")
	if err := m.SetAssignment(Action(action), templateName); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	utils.SendOK(w)
}

// List the built-in templates, or get the content of one if id is given
func (m *Manager) HandleGetBuiltinTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := utils.GetPara(r, "id")
	if err != nil {
		js, _ := json.Marshal(builtinTemplates)
		utils.SendJSONResponse(w, string(js))
		return
	}

	for _, t := range builtinTemplates {
		if t.ID == id {
			js, _ := json.Marshal(map[string]string{
				"id":      t.ID,
				"content": string(t.content),
			})
			utils.SendJSONResponse(w, string(js))
			return
		}
	}
	utils.SendErrorResponse(w, "built-in template not found")
}
