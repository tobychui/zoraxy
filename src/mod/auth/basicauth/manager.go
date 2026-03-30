package basicauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"unicode"

	"imuslab.com/zoraxy/mod/auth"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

const (
	DefaultGroupID = "default"

	tableName = "basicauth"
	groupsKey = "groups"
	usersKey  = "users"
)

type Options struct {
	Database          *database.Database
	Logger            *logger.Logger
	GroupUsageChecker func(string) []string
}

type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type GroupSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
	UserCount int    `json:"user_count"`
	HostCount int    `json:"host_count"`
}

type User struct {
	Username     string   `json:"username"`
	PasswordHash string   `json:"password_hash"`
	GroupIDs     []string `json:"group_ids"`
}

type UserSummary struct {
	Username   string   `json:"username"`
	GroupIDs   []string `json:"group_ids"`
	GroupNames []string `json:"group_names"`
}

type Manager struct {
	options *Options

	mu     sync.RWMutex
	groups map[string]*Group
	users  map[string]*User
}

func NewManager(options *Options) (*Manager, error) {
	if options == nil || options.Database == nil {
		return nil, errors.New("basic auth manager database is required")
	}

	if err := options.Database.NewTable(tableName); err != nil {
		return nil, err
	}

	m := &Manager{
		options: options,
		groups:  map[string]*Group{},
		users:   map[string]*User{},
	}

	if err := m.Reload(); err != nil {
		return nil, err
	}

	return m, nil
}

func (m *Manager) Reload() error {
	loadedGroups := map[string]*Group{}
	loadedUsers := map[string]*User{}
	_ = m.options.Database.Read(tableName, groupsKey, &loadedGroups)
	_ = m.options.Database.Read(tableName, usersKey, &loadedUsers)

	normalizedGroups := map[string]*Group{}
	for _, group := range loadedGroups {
		normalizedGroup := m.normalizeGroup(group)
		normalizedGroups[normalizedGroup.ID] = normalizedGroup
	}
	if _, ok := normalizedGroups[DefaultGroupID]; !ok {
		normalizedGroups[DefaultGroupID] = &Group{
			ID:   DefaultGroupID,
			Name: "Default",
		}
	}

	normalizedUsers := map[string]*User{}
	for _, user := range loadedUsers {
		normalizedUser := m.normalizeUser(user, normalizedGroups)
		if normalizedUser == nil {
			continue
		}
		normalizedUsers[m.normalizeUsernameKey(normalizedUser.Username)] = normalizedUser
	}

	m.mu.Lock()
	m.groups = normalizedGroups
	m.users = normalizedUsers
	m.mu.Unlock()

	return m.persist()
}

func (m *Manager) ListGroups() []GroupSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userCounts := map[string]int{}
	for _, user := range m.users {
		for _, groupID := range user.GroupIDs {
			userCounts[groupID]++
		}
	}

	results := make([]GroupSummary, 0, len(m.groups))
	for _, group := range m.groups {
		hostCount := 0
		if m.options.GroupUsageChecker != nil {
			hostCount = len(m.options.GroupUsageChecker(group.ID))
		}
		results = append(results, GroupSummary{
			ID:        group.ID,
			Name:      group.Name,
			IsDefault: group.ID == DefaultGroupID,
			UserCount: userCounts[group.ID],
			HostCount: hostCount,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].IsDefault != results[j].IsDefault {
			return results[i].IsDefault
		}
		return strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
	})

	return results
}

func (m *Manager) ListUsers() []UserSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make([]UserSummary, 0, len(m.users))
	for _, user := range m.users {
		groupNames := make([]string, 0, len(user.GroupIDs))
		for _, groupID := range user.GroupIDs {
			if group, ok := m.groups[groupID]; ok {
				groupNames = append(groupNames, group.Name)
			} else {
				groupNames = append(groupNames, groupID)
			}
		}
		results = append(results, UserSummary{
			Username:   user.Username,
			GroupIDs:   append([]string{}, user.GroupIDs...),
			GroupNames: groupNames,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].Username) < strings.ToLower(results[j].Username)
	})

	return results
}

func (m *Manager) CreateGroup(name string, id string) (*Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("group name cannot be empty")
	}

	explicitID := strings.TrimSpace(id)
	baseID := m.sanitizeGroupID(explicitID)
	if baseID == "" {
		baseID = m.sanitizeGroupID(name)
	}
	if baseID == "" {
		baseID = "group"
	}
	if baseID == DefaultGroupID && strings.ToLower(explicitID) != DefaultGroupID {
		baseID = "group"
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if explicitID != "" {
		if _, exists := m.groups[baseID]; exists {
			return nil, fmt.Errorf("basic auth group %q already exists", baseID)
		}
	} else {
		baseID = m.nextAvailableGroupIDLocked(baseID)
	}

	group := m.normalizeGroup(&Group{
		ID:   baseID,
		Name: name,
	})
	m.groups[group.ID] = group

	if err := m.persistLocked(); err != nil {
		return nil, err
	}

	return m.copyGroup(group), nil
}

func (m *Manager) UpdateGroup(id string, name string) (*Group, error) {
	id = m.sanitizeGroupID(id)
	if id == "" {
		return nil, errors.New("group id not found")
	}
	if id == DefaultGroupID {
		return nil, errors.New("the default group cannot be renamed")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	group, ok := m.groups[id]
	if !ok {
		return nil, fmt.Errorf("basic auth group %q not found", id)
	}

	updated := m.normalizeGroup(&Group{
		ID:   group.ID,
		Name: name,
	})
	m.groups[id] = updated

	if err := m.persistLocked(); err != nil {
		return nil, err
	}

	return m.copyGroup(updated), nil
}

func (m *Manager) DeleteGroup(id string) error {
	id = m.sanitizeGroupID(id)
	if id == "" {
		return errors.New("group id not found")
	}
	if id == DefaultGroupID {
		return errors.New("the default group cannot be deleted")
	}

	if m.options.GroupUsageChecker != nil {
		inUseBy := m.options.GroupUsageChecker(id)
		if len(inUseBy) > 0 {
			return fmt.Errorf("basic auth group %q is still used by host rules: %s", id, strings.Join(inUseBy, ", "))
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.groups[id]; !ok {
		return fmt.Errorf("basic auth group %q not found", id)
	}

	delete(m.groups, id)
	for key, user := range m.users {
		updatedGroups := make([]string, 0, len(user.GroupIDs))
		for _, groupID := range user.GroupIDs {
			if groupID != id {
				updatedGroups = append(updatedGroups, groupID)
			}
		}
		if len(updatedGroups) == 0 {
			updatedGroups = []string{DefaultGroupID}
		}
		user.GroupIDs = updatedGroups
		m.users[key] = user
	}

	return m.persistLocked()
}

func (m *Manager) CreateUser(username string, password string, groupIDs []string) (*UserSummary, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username cannot be empty")
	}
	if password == "" {
		return nil, errors.New("password cannot be empty")
	}

	key := m.normalizeUsernameKey(username)

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.users[key]; exists {
		return nil, fmt.Errorf("basic auth user %q already exists", username)
	}

	user := m.normalizeUser(&User{
		Username:     username,
		PasswordHash: auth.Hash(password),
		GroupIDs:     groupIDs,
	}, m.groups)
	if user == nil {
		return nil, errors.New("invalid basic auth user")
	}

	m.users[key] = user
	if err := m.persistLocked(); err != nil {
		return nil, err
	}

	return m.userSummaryLocked(user), nil
}

func (m *Manager) UpdateUser(username string, password string, groupIDs []string) (*UserSummary, error) {
	key := m.normalizeUsernameKey(username)
	if key == "" {
		return nil, errors.New("username not found")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	user, ok := m.users[key]
	if !ok {
		return nil, fmt.Errorf("basic auth user %q not found", username)
	}

	passwordHash := user.PasswordHash
	if strings.TrimSpace(password) != "" {
		passwordHash = auth.Hash(password)
	}

	updated := m.normalizeUser(&User{
		Username:     user.Username,
		PasswordHash: passwordHash,
		GroupIDs:     groupIDs,
	}, m.groups)
	if updated == nil {
		return nil, errors.New("invalid basic auth user")
	}

	m.users[key] = updated
	if err := m.persistLocked(); err != nil {
		return nil, err
	}

	return m.userSummaryLocked(updated), nil
}

func (m *Manager) DeleteUser(username string) error {
	key := m.normalizeUsernameKey(username)
	if key == "" {
		return errors.New("username not found")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.users[key]; !ok {
		return fmt.Errorf("basic auth user %q not found", username)
	}

	delete(m.users, key)
	return m.persistLocked()
}

func (m *Manager) ValidateCredentials(username string, password string, allowedGroupIDs []string) (bool, []string) {
	key := m.normalizeUsernameKey(username)
	if key == "" {
		return false, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	user, ok := m.users[key]
	if !ok || user == nil {
		return false, nil
	}

	if auth.Hash(password) != user.PasswordHash {
		return false, nil
	}

	normalizedAllowed := m.normalizeExistingGroupIDs(allowedGroupIDs, m.groups)
	if len(normalizedAllowed) == 0 {
		return false, nil
	}

	matchedGroups := []string{}
	for _, allowedGroupID := range normalizedAllowed {
		for _, userGroupID := range user.GroupIDs {
			if allowedGroupID == userGroupID {
				matchedGroups = append(matchedGroups, allowedGroupID)
				break
			}
		}
	}

	return len(matchedGroups) > 0, matchedGroups
}

func (m *Manager) ImportLegacyCredential(username string, passwordHash string, groupID string) error {
	username = strings.TrimSpace(username)
	passwordHash = strings.TrimSpace(passwordHash)
	groupID = strings.TrimSpace(groupID)
	if username == "" || passwordHash == "" {
		return errors.New("legacy basic auth credential is incomplete")
	}
	if groupID == "" {
		groupID = DefaultGroupID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	normalizedGroupIDs := m.normalizeExistingGroupIDs([]string{groupID}, m.groups)
	if len(normalizedGroupIDs) == 0 {
		normalizedGroupIDs = []string{DefaultGroupID}
	}

	key := m.normalizeUsernameKey(username)
	if existing, ok := m.users[key]; ok && existing != nil {
		if existing.PasswordHash != passwordHash {
			return fmt.Errorf("legacy basic auth user %q has conflicting passwords and cannot be auto-migrated", username)
		}

		existing.GroupIDs = m.normalizeExistingGroupIDs(append(existing.GroupIDs, normalizedGroupIDs...), m.groups)
		m.users[key] = existing
		return m.persistLocked()
	}

	m.users[key] = m.normalizeUser(&User{
		Username:     username,
		PasswordHash: passwordHash,
		GroupIDs:     normalizedGroupIDs,
	}, m.groups)
	return m.persistLocked()
}

func (m *Manager) NormalizeGroupIDs(groupIDs []string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.normalizeExistingGroupIDs(groupIDs, m.groups)
}

func (m *Manager) ParseGroupIDs(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}, nil
	}

	groupIDs := []string{}
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &groupIDs); err != nil {
			return nil, errors.New("invalid basic auth group selection")
		}
	} else {
		groupIDs = strings.Split(raw, ",")
	}

	return m.NormalizeGroupIDs(groupIDs), nil
}

func (m *Manager) HandleGroupList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	js, _ := json.Marshal(m.ListGroups())
	utils.SendJSONResponse(w, string(js))
}

func (m *Manager) HandleGroupCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name, err := utils.PostPara(r, "name")
	if err != nil {
		utils.SendErrorResponse(w, "group name not found")
		return
	}
	customID, _ := utils.PostPara(r, "id")

	group, err := m.CreateGroup(name, customID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	js, _ := json.Marshal(group)
	utils.SendJSONResponse(w, string(js))
}

func (m *Manager) HandleGroupUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	groupID, err := utils.PostPara(r, "id")
	if err != nil {
		utils.SendErrorResponse(w, "group id not found")
		return
	}
	name, err := utils.PostPara(r, "name")
	if err != nil {
		utils.SendErrorResponse(w, "group name not found")
		return
	}

	group, err := m.UpdateGroup(groupID, name)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	js, _ := json.Marshal(group)
	utils.SendJSONResponse(w, string(js))
}

func (m *Manager) HandleGroupDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	groupID, err := utils.PostPara(r, "id")
	if err != nil {
		utils.SendErrorResponse(w, "group id not found")
		return
	}

	if err := m.DeleteGroup(groupID); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

func (m *Manager) HandleUserList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	js, _ := json.Marshal(m.ListUsers())
	utils.SendJSONResponse(w, string(js))
}

func (m *Manager) HandleUserCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, err := utils.PostPara(r, "username")
	if err != nil {
		utils.SendErrorResponse(w, "username not found")
		return
	}
	password, err := utils.PostPara(r, "password")
	if err != nil {
		utils.SendErrorResponse(w, "password not found")
		return
	}
	groupIDsRaw, _ := utils.PostPara(r, "groupIds")
	groupIDs, err := m.ParseGroupIDs(groupIDsRaw)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	user, err := m.CreateUser(username, password, groupIDs)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	js, _ := json.Marshal(user)
	utils.SendJSONResponse(w, string(js))
}

func (m *Manager) HandleUserUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, err := utils.PostPara(r, "username")
	if err != nil {
		utils.SendErrorResponse(w, "username not found")
		return
	}
	password, _ := utils.PostPara(r, "password")
	groupIDsRaw, _ := utils.PostPara(r, "groupIds")
	groupIDs, err := m.ParseGroupIDs(groupIDsRaw)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	user, err := m.UpdateUser(username, password, groupIDs)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	js, _ := json.Marshal(user)
	utils.SendJSONResponse(w, string(js))
}

func (m *Manager) HandleUserDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, err := utils.PostPara(r, "username")
	if err != nil {
		utils.SendErrorResponse(w, "username not found")
		return
	}

	if err := m.DeleteUser(username); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

func (m *Manager) persist() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.persistLockedCopy()
}

func (m *Manager) persistLocked() error {
	return m.persistLockedCopy()
}

func (m *Manager) persistLockedCopy() error {
	groupsCopy := map[string]*Group{}
	for id, group := range m.groups {
		groupsCopy[id] = m.copyGroup(group)
	}

	usersCopy := map[string]*User{}
	for key, user := range m.users {
		usersCopy[key] = m.copyUser(user)
	}

	if err := m.options.Database.Write(tableName, groupsKey, groupsCopy); err != nil {
		return err
	}
	if err := m.options.Database.Write(tableName, usersKey, usersCopy); err != nil {
		return err
	}

	return nil
}

func (m *Manager) userSummaryLocked(user *User) *UserSummary {
	if user == nil {
		return nil
	}

	groupIDs := append([]string{}, user.GroupIDs...)
	groupNames := make([]string, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		if group, ok := m.groups[groupID]; ok {
			groupNames = append(groupNames, group.Name)
		} else {
			groupNames = append(groupNames, groupID)
		}
	}

	return &UserSummary{
		Username:   user.Username,
		GroupIDs:   groupIDs,
		GroupNames: groupNames,
	}
}

func (m *Manager) normalizeGroup(group *Group) *Group {
	if group == nil {
		group = &Group{}
	}

	normalized := *group
	normalized.ID = m.sanitizeGroupID(normalized.ID)
	if normalized.ID == "" {
		normalized.ID = DefaultGroupID
	}
	normalized.Name = strings.TrimSpace(normalized.Name)
	if normalized.Name == "" {
		if normalized.ID == DefaultGroupID {
			normalized.Name = "Default"
		} else {
			normalized.Name = normalized.ID
		}
	}

	return &normalized
}

func (m *Manager) normalizeUser(user *User, groups map[string]*Group) *User {
	if user == nil {
		return nil
	}

	username := strings.TrimSpace(user.Username)
	if username == "" {
		return nil
	}

	passwordHash := strings.TrimSpace(user.PasswordHash)
	if passwordHash == "" {
		return nil
	}

	groupIDs := m.normalizeExistingGroupIDs(user.GroupIDs, groups)
	if len(groupIDs) == 0 {
		groupIDs = []string{DefaultGroupID}
	}

	return &User{
		Username:     username,
		PasswordHash: passwordHash,
		GroupIDs:     groupIDs,
	}
}

func (m *Manager) normalizeExistingGroupIDs(groupIDs []string, groups map[string]*Group) []string {
	results := []string{}
	seen := map[string]bool{}
	for _, groupID := range groupIDs {
		normalizedID := m.sanitizeGroupID(groupID)
		if normalizedID == "" || seen[normalizedID] {
			continue
		}
		if groups != nil {
			if _, ok := groups[normalizedID]; !ok {
				continue
			}
		}
		seen[normalizedID] = true
		results = append(results, normalizedID)
	}
	return results
}

func (m *Manager) sanitizeGroupID(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case unicode.IsLower(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}

	return strings.Trim(b.String(), "-_")
}

func (m *Manager) normalizeUsernameKey(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func (m *Manager) nextAvailableGroupIDLocked(base string) string {
	candidate := base
	index := 2
	for {
		if _, exists := m.groups[candidate]; !exists {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, index)
		index++
	}
}

func (m *Manager) copyGroup(group *Group) *Group {
	if group == nil {
		return nil
	}
	copied := *group
	return &copied
}

func (m *Manager) copyUser(user *User) *User {
	if user == nil {
		return nil
	}
	copied := *user
	copied.GroupIDs = append([]string{}, user.GroupIDs...)
	return &copied
}
