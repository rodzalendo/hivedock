package store

import "fmt"

// SetServiceHidden persists a user's explicit hide/unhide for a service
// (overrides the sidecar auto-hide heuristic). Upserts into service_prefs.
func (s *Store) SetServiceHidden(stack, service string, hidden bool) error {
	h := 0
	if hidden {
		h = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO service_prefs (stack, service, hidden, updated_at)
		VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(stack, service) DO UPDATE SET hidden = excluded.hidden, updated_at = datetime('now')
	`, stack, service, h)
	if err != nil {
		return fmt.Errorf("set service hidden: %w", err)
	}
	return nil
}

// SetServiceIcon persists a user's custom icon (a URL or a dashboard-icons
// slug) for a service; an empty string clears it. Upserts into service_prefs
// without disturbing the row's hidden flag.
func (s *Store) SetServiceIcon(stack, service, icon string) error {
	_, err := s.db.Exec(`
		INSERT INTO service_prefs (stack, service, icon, updated_at)
		VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(stack, service) DO UPDATE SET icon = excluded.icon, updated_at = datetime('now')
	`, stack, service, icon)
	if err != nil {
		return fmt.Errorf("set service icon: %w", err)
	}
	return nil
}

// ServiceIconOverrides loads all persisted custom icons, keyed
// stack -> service -> icon. Rows with an empty icon are omitted.
func (s *Store) ServiceIconOverrides() (map[string]map[string]string, error) {
	rows, err := s.db.Query(`SELECT stack, service, icon FROM service_prefs WHERE icon <> ''`)
	if err != nil {
		return nil, fmt.Errorf("query service_prefs icons: %w", err)
	}
	defer rows.Close()

	out := map[string]map[string]string{}
	for rows.Next() {
		var stack, service, icon string
		if err := rows.Scan(&stack, &service, &icon); err != nil {
			return nil, fmt.Errorf("scan service_prefs icon: %w", err)
		}
		if out[stack] == nil {
			out[stack] = map[string]string{}
		}
		out[stack][service] = icon
	}
	return out, rows.Err()
}

// SetServiceName persists a custom display name for a service's dashboard
// card; empty clears it. Upserts without disturbing the row's other prefs.
func (s *Store) SetServiceName(stack, service, name string) error {
	_, err := s.db.Exec(`
		INSERT INTO service_prefs (stack, service, display_name, updated_at)
		VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(stack, service) DO UPDATE SET display_name = excluded.display_name, updated_at = datetime('now')
	`, stack, service, name)
	if err != nil {
		return fmt.Errorf("set service name: %w", err)
	}
	return nil
}

// ServiceNameOverrides loads all persisted custom display names, keyed
// stack -> service -> name. Rows with an empty name are omitted.
func (s *Store) ServiceNameOverrides() (map[string]map[string]string, error) {
	rows, err := s.db.Query(`SELECT stack, service, display_name FROM service_prefs WHERE display_name <> ''`)
	if err != nil {
		return nil, fmt.Errorf("query service_prefs names: %w", err)
	}
	defer rows.Close()

	out := map[string]map[string]string{}
	for rows.Next() {
		var stack, service, name string
		if err := rows.Scan(&stack, &service, &name); err != nil {
			return nil, fmt.Errorf("scan service_prefs name: %w", err)
		}
		if out[stack] == nil {
			out[stack] = map[string]string{}
		}
		out[stack][service] = name
	}
	return out, rows.Err()
}

// RenameStackPrefs moves any persisted per-service prefs (hidden, custom icon)
// from an old stack name to a new one, so a renamed stack keeps its settings.
func (s *Store) RenameStackPrefs(oldName, newName string) error {
	_, err := s.db.Exec(`UPDATE service_prefs SET stack = ? WHERE stack = ?`, newName, oldName)
	if err != nil {
		return fmt.Errorf("rename stack prefs: %w", err)
	}
	return nil
}

// ServiceHiddenOverrides loads all persisted hide/unhide overrides, keyed
// stack -> service -> hidden.
func (s *Store) ServiceHiddenOverrides() (map[string]map[string]bool, error) {
	rows, err := s.db.Query(`SELECT stack, service, hidden FROM service_prefs`)
	if err != nil {
		return nil, fmt.Errorf("query service_prefs: %w", err)
	}
	defer rows.Close()

	out := map[string]map[string]bool{}
	for rows.Next() {
		var stack, service string
		var hidden int
		if err := rows.Scan(&stack, &service, &hidden); err != nil {
			return nil, fmt.Errorf("scan service_prefs: %w", err)
		}
		if out[stack] == nil {
			out[stack] = map[string]bool{}
		}
		out[stack][service] = hidden != 0
	}
	return out, rows.Err()
}
