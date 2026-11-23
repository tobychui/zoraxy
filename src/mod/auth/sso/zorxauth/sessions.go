package zorxauth

/*
	sessions.go

	Session ownership helpers. Every session (gateway, browser and the one-time
	validation code in between) is bound to the User.ID of the user that signed
	in, not to the username. This keeps a session working across a rename and
	makes sure a session of a deleted user can never authenticate a new user
	that reuses the same username.

	Session persistence. Browser and gateway sessions are written through to the
	session tables: every add, update and removal goes through the helpers below,
	which change the in-memory store and the database together. A logged out
	session therefore never comes back after a restart, and live sessions survive
	even when Zoraxy is killed before Close() runs.
*/

import (
	"errors"
	"sync"
)

// resolveSessionUser returns the current user record a session belongs to.
// Sessions without a user ID were created before sessions were bound to IDs
// and are rejected, so their owner signs in again once.
func (ar *AuthRouter) resolveSessionUser(username, userID string) (*User, error) {
	if userID == "" {
		return nil, errors.New("session is not bound to a user ID")
	}

	// Fast path: a single read by username, valid only when the ID still matches
	if user, err := ar.getUserByUsername(username); err == nil && user.ID == userID {
		return user, nil
	}

	// The user was renamed (or deleted), look the record up by ID instead
	user, err := ar.getUserByID(userID)
	if err != nil {
		return nil, errors.New("session user no longer exists")
	}
	return user, nil
}

// deleteBrowserSessionsOfUser removes every per-site browser session of a user
func (ar *AuthRouter) deleteBrowserSessionsOfUser(userID string) {
	if userID == "" {
		return
	}
	ar.cookieIdStore.Range(func(key, value interface{}) bool {
		if s, ok := value.(*BrowserSession); ok && s.UserID == userID {
			if id, ok := key.(string); ok {
				ar.removeBrowserSession(id)
			}
		}
		return true
	})
}

// deleteAllSessionsOfUser removes every session of a user: browser sessions,
// gateway sessions and pending validation codes
func (ar *AuthRouter) deleteAllSessionsOfUser(userID string) {
	if userID == "" {
		return
	}
	ar.deleteBrowserSessionsOfUser(userID)
	ar.gatewaySessionStore.Range(func(key, value interface{}) bool {
		if s, ok := value.(*GatewaySession); ok && s.UserID == userID {
			if id, ok := key.(string); ok {
				ar.removeGatewaySession(id)
			}
		}
		return true
	})
	ar.sessionIdStore.Range(func(key, value interface{}) bool {
		if s, ok := value.(*ValidationCodeSession); ok && s.UserID == userID {
			ar.sessionIdStore.Delete(key)
		}
		return true
	})
}

/* Write-through session store helpers */

// storeBrowserSession adds or overwrites a browser session
func (ar *AuthRouter) storeBrowserSession(id string, s *BrowserSession) {
	ar.putSession(&ar.cookieIdStore, DB_BROWSER_SESSIONS_TABLE, DB_BROWSER_SESSION_KEY_PREFIX, id, s)
}

// replaceBrowserSession swaps in updated only if old is still the stored session,
// so it can never revive a session that was removed in the meantime
func (ar *AuthRouter) replaceBrowserSession(id string, old, updated *BrowserSession) {
	ar.swapSession(&ar.cookieIdStore, DB_BROWSER_SESSIONS_TABLE, DB_BROWSER_SESSION_KEY_PREFIX, id, old, updated)
}

// removeBrowserSession removes a browser session
func (ar *AuthRouter) removeBrowserSession(id string) {
	ar.dropSession(&ar.cookieIdStore, DB_BROWSER_SESSIONS_TABLE, DB_BROWSER_SESSION_KEY_PREFIX, id)
}

// storeGatewaySession adds or overwrites a gateway session
func (ar *AuthRouter) storeGatewaySession(id string, s *GatewaySession) {
	ar.putSession(&ar.gatewaySessionStore, DB_GATEWAY_SESSIONS_TABLE, DB_GATEWAY_SESSION_KEY_PREFIX, id, s)
}

// replaceGatewaySession swaps in updated only if old is still the stored session,
// so it can never revive a session that was removed in the meantime
func (ar *AuthRouter) replaceGatewaySession(id string, old, updated *GatewaySession) {
	ar.swapSession(&ar.gatewaySessionStore, DB_GATEWAY_SESSIONS_TABLE, DB_GATEWAY_SESSION_KEY_PREFIX, id, old, updated)
}

// removeGatewaySession removes a gateway session
func (ar *AuthRouter) removeGatewaySession(id string) {
	ar.dropSession(&ar.gatewaySessionStore, DB_GATEWAY_SESSIONS_TABLE, DB_GATEWAY_SESSION_KEY_PREFIX, id)
}

// putSession stores a session in memory and in its table
func (ar *AuthRouter) putSession(store *sync.Map, table, prefix, id string, session interface{}) {
	ar.sessionPersistMutex.Lock()
	defer ar.sessionPersistMutex.Unlock()
	store.Store(id, session)
	ar.writeSessionRow(table, prefix+id, session)
}

// swapSession replaces a session in memory and in its table if old is still current
func (ar *AuthRouter) swapSession(store *sync.Map, table, prefix, id string, old, updated interface{}) {
	ar.sessionPersistMutex.Lock()
	defer ar.sessionPersistMutex.Unlock()
	if store.CompareAndSwap(id, old, updated) {
		ar.writeSessionRow(table, prefix+id, updated)
	}
}

// dropSession removes a session from memory and from its table
func (ar *AuthRouter) dropSession(store *sync.Map, table, prefix, id string) {
	ar.sessionPersistMutex.Lock()
	defer ar.sessionPersistMutex.Unlock()
	store.Delete(id)
	if ar.Database == nil || !ar.Database.KeyExists(table, prefix+id) {
		return
	}
	if err := ar.Database.Delete(table, prefix+id); err != nil && ar.Logger != nil {
		ar.Logger.PrintAndLog("zorxauth", "Failed to delete persisted session: "+err.Error(), err)
	}
}

// writeSessionRow persists one session. Memory stays authoritative while running,
// so a failed write is logged and Close() retries it on shutdown.
func (ar *AuthRouter) writeSessionRow(table, key string, session interface{}) {
	if ar.Database == nil {
		return
	}
	if err := ar.Database.Write(table, key, session); err != nil && ar.Logger != nil {
		ar.Logger.PrintAndLog("zorxauth", "Failed to persist session: "+err.Error(), err)
	}
}

// clearSessionTable deletes every row of a session table
func (ar *AuthRouter) clearSessionTable(table string) {
	entries, err := ar.Database.ListTable(table)
	if err != nil {
		return
	}
	for _, entry := range entries {
		ar.Database.Delete(table, string(entry[0]))
	}
}
