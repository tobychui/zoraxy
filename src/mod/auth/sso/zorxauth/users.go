package zorxauth

import (
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"imuslab.com/zoraxy/mod/utils"
)

type UserResponse struct {
	ID           string   `json:"id"`
	Username     string   `json:"username"`
	Email        string   `json:"email"`
	AllowedHosts []string `json:"allowedHosts"`
}

func normalizeUsername(username string) string {
	return strings.TrimSpace(username)
}

func normalizeAllowedHosts(rawHosts string) []string {
	hostsMap := map[string]bool{}
	hosts := []string{}
	for _, host := range strings.Split(rawHosts, ",") {
		host = strings.TrimSpace(strings.ToLower(host))
		if host == "" || hostsMap[host] {
			continue
		}
		hostsMap[host] = true
		hosts = append(hosts, host)
	}

	return hosts
}

func userToResponse(user *User) *UserResponse {
	return &UserResponse{
		ID:           user.ID,
		Username:     user.Username,
		Email:        user.Email,
		AllowedHosts: user.AllowedHosts,
	}
}

func hashPassword(raw string) string {
	h := sha512.New()
	h.Write([]byte(raw))
	return hex.EncodeToString(h.Sum(nil))
}

func (ar *AuthRouter) userDbKey(username string) string {
	return DB_USERS_KEY_PREFIX + strings.ToLower(normalizeUsername(username))
}

func (ar *AuthRouter) getUserByUsername(username string) (*User, error) {
	if ar.Database == nil {
		return nil, errors.New("database not available")
	}

	normalizedUsername := normalizeUsername(username)
	if normalizedUsername == "" {
		return nil, errors.New("username is empty")
	}

	user := &User{}
	if ar.Database.KeyExists(DB_USERS_TABLE, ar.userDbKey(normalizedUsername)) {
		if err := ar.Database.Read(DB_USERS_TABLE, ar.userDbKey(normalizedUsername), user); err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("user not found")
	}

	user.Username = normalizeUsername(user.Username)
	if user.Username == "" {
		user.Username = normalizedUsername
	}
	if user.ID == "" {
		user.ID = uuid.NewString()
	}

	return user, nil
}

func (ar *AuthRouter) listUsers() ([]*User, error) {
	if ar.Database == nil {
		return nil, errors.New("database not available")
	}

	entries, err := ar.Database.ListTable(DB_USERS_TABLE)
	if err != nil {
		return nil, err
	}

	users := []*User{}
	for _, keypairs := range entries {
		if len(keypairs) < 2 {
			continue
		}

		key := string(keypairs[0])
		if !strings.HasPrefix(key, DB_USERS_KEY_PREFIX) {
			continue
		}

		username := strings.TrimPrefix(key, DB_USERS_KEY_PREFIX)
		user, err := ar.getUserByUsername(username)
		if err != nil {
			continue
		}

		users = append(users, user)
	}

	sort.Slice(users, func(i, j int) bool {
		return strings.ToLower(users[i].Username) < strings.ToLower(users[j].Username)
	})

	return users, nil
}

func (ar *AuthRouter) saveUser(user *User, previousUsername string) error {
	if ar.Database == nil {
		return errors.New("database not available")
	}

	if user == nil {
		return errors.New("user is nil")
	}

	user.Username = normalizeUsername(user.Username)
	if user.Username == "" {
		return errors.New("username cannot be empty")
	}

	if strings.Contains(user.Username, "/") || strings.Contains(user.Username, "\\") {
		return errors.New("username contains invalid characters")
	}

	if user.ID == "" {
		user.ID = uuid.NewString()
	}

	if err := ar.Database.Write(DB_USERS_TABLE, ar.userDbKey(user.Username), user); err != nil {
		return err
	}

	if previousUsername != "" && !strings.EqualFold(previousUsername, user.Username) {
		ar.Database.Delete(DB_USERS_TABLE, ar.userDbKey(previousUsername))

		ar.sessionIdStore.Range(func(key, value interface{}) bool {
			name, ok := value.(string)
			if ok && strings.EqualFold(name, previousUsername) {
				ar.sessionIdStore.Store(key, user.Username)
			}
			return true
		})
	}

	return nil
}

func (ar *AuthRouter) HandleUsersList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	users, err := ar.listUsers()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	results := []*UserResponse{}
	for _, user := range users {
		results = append(results, userToResponse(user))
	}

	js, _ := json.Marshal(results)
	utils.SendJSONResponse(w, string(js))
}

func (ar *AuthRouter) HandleUserCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	username, err := utils.PostPara(r, "username")
	if err != nil {
		utils.SendErrorResponse(w, "missing username")
		return
	}

	password, err := utils.PostPara(r, "password")
	if err != nil {
		utils.SendErrorResponse(w, "missing password")
		return
	}

	username = normalizeUsername(username)
	if username == "" {
		utils.SendErrorResponse(w, "username cannot be empty")
		return
	}

	if _, err := ar.getUserByUsername(username); err == nil {
		utils.SendErrorResponse(w, "user already exists")
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	if email != "" {
		if _, err := mail.ParseAddress(email); err != nil {
			utils.SendErrorResponse(w, "invalid email format")
			return
		}
	}

	user := &User{
		ID:           uuid.NewString(),
		Username:     username,
		Email:        email,
		PasswordHash: hashPassword(password),
		AllowedHosts: normalizeAllowedHosts(r.FormValue("allowedHosts")),
	}

	if err := ar.saveUser(user, ""); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

func (ar *AuthRouter) HandleUserUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	username, err := utils.PostPara(r, "username")
	if err != nil {
		utils.SendErrorResponse(w, "missing username")
		return
	}

	user, err := ar.getUserByUsername(username)
	if err != nil {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	originalUsername := user.Username
	newUsername := strings.TrimSpace(r.FormValue("newUsername"))
	if newUsername != "" {
		if !strings.EqualFold(newUsername, originalUsername) {
			if _, err := ar.getUserByUsername(newUsername); err == nil {
				utils.SendErrorResponse(w, "new username already exists")
				return
			}
		}
		user.Username = newUsername
	}

	if _, ok := r.Form["email"]; ok {
		email := strings.TrimSpace(r.FormValue("email"))
		if email != "" {
			if _, err := mail.ParseAddress(email); err != nil {
				utils.SendErrorResponse(w, "invalid email format")
				return
			}
		}
		user.Email = email
	}

	if password := strings.TrimSpace(r.FormValue("password")); password != "" {
		user.PasswordHash = hashPassword(password)
	}

	if _, ok := r.Form["allowedHosts"]; ok {
		user.AllowedHosts = normalizeAllowedHosts(r.FormValue("allowedHosts"))
	}

	if err := ar.saveUser(user, originalUsername); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

func (ar *AuthRouter) HandleUserDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	username, err := utils.PostPara(r, "username")
	if err != nil {
		utils.SendErrorResponse(w, "missing username")
		return
	}

	user, err := ar.getUserByUsername(username)
	if err != nil {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	if err := ar.Database.Delete(DB_USERS_TABLE, ar.userDbKey(user.Username)); err != nil {
		utils.SendErrorResponse(w, "failed to delete user")
		return
	}

	ar.sessionIdStore.Range(func(key, value interface{}) bool {
		name, ok := value.(string)
		if ok && strings.EqualFold(name, user.Username) {
			ar.sessionIdStore.Delete(key)
		}
		return true
	})

	utils.SendOK(w)
}

func (ar *AuthRouter) GetUserFromRequest(w http.ResponseWriter, r *http.Request) (*User, error) {
	cookie, err := r.Cookie(ar.Options.CookieName)
	if err != nil {
		return nil, err
	}

	sessionObj, exists := ar.cookieIdStore.Load(cookie.Value)
	if !exists {
		return nil, errors.New("session not found")
	}

	browserSession, ok := sessionObj.(*BrowserSession)
	if !ok || browserSession == nil {
		return nil, errors.New("invalid session mapping")
	}

	// Check if session has expired
	if time.Now().After(browserSession.Expiry) {
		ar.cookieIdStore.Delete(cookie.Value)
		return nil, errors.New("session expired")
	}

	user, err := ar.getUserByUsername(browserSession.Username)
	if err != nil {
		return nil, errors.New("user record not found")
	}

	return user, nil
}

func (ar *AuthRouter) ValidateUserAccessToHost(username, host string) bool {
	username = normalizeUsername(username)
	host = strings.TrimSpace(strings.ToLower(host))

	if username == "" || host == "" {
		return false
	}

	user, err := ar.getUserByUsername(username)
	if err != nil {
		return false
	}

	if len(user.AllowedHosts) == 0 {
		return true
	}

	for _, allowed := range user.AllowedHosts {
		allowed = strings.TrimSpace(strings.ToLower(allowed))
		if allowed == "" {
			continue
		}

		if allowed == host {
			return true
		}

		if strings.HasPrefix(allowed, "*.") {
			suffix := strings.TrimPrefix(allowed, "*.")
			if strings.HasSuffix(host, "."+suffix) || host == suffix {
				return true
			}
		}
	}

	return false
}

func (ar *AuthRouter) ValidateUsername(username, password string) bool {
	username = normalizeUsername(username)
	if username == "" || password == "" {
		return false
	}

	user, err := ar.getUserByUsername(username)
	if err != nil {
		users, listErr := ar.listUsers()
		if listErr != nil {
			return false
		}

		var matchedUser *User
		for _, u := range users {
			if strings.EqualFold(strings.TrimSpace(u.Email), username) {
				matchedUser = u
				break
			}
		}

		if matchedUser == nil {
			return false
		}

		user = matchedUser
	}

	return user.PasswordHash != "" && user.PasswordHash == hashPassword(password)
}
