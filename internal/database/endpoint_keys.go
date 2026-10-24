package database

import (
	"time"

	"gorm.io/gorm"

	"lognorth/internal/events"
)

// routeEvents is how many of the newest events rekeyEndpoints reads for the
// routes the SDKs sent: a range of ids, so the read stays short on any size.
const routeEvents = 200_000

type appPath struct {
	APIKeyID uint
	Path     string
}

// rekeyEndpoints moves endpoint_stats and the endpoint alert states from
// URLs to endpoints (events.EndpointKey): /users/1 and /users/2 become
// /users/:id. A path takes the route its newest events sent, else it keeps
// its words and loses its ids. Rows that land on one endpoint and bucket
// merge: the totals and the errors add up, and the average duration is
// weighted by the totals.
func rekeyEndpoints(db *gorm.DB) error {
	var routes []struct {
		APIKeyID    uint
		Path, Route string
	}
	err := db.Raw(`SELECT DISTINCT api_key_id, CAST(json_extract(context, '$.path') AS TEXT) AS path,
			CAST(json_extract(context, '$.route') AS TEXT) AS route
		FROM events WHERE id > (SELECT COALESCE(MAX(id), 0) FROM events) - ?
			AND json_extract(context, '$.route') IS NOT NULL`, routeEvents).Scan(&routes).Error
	if err != nil {
		return err
	}
	routeOf := map[appPath]string{}
	for _, r := range routes {
		routeOf[appPath{r.APIKeyID, r.Path}] = r.Route
	}
	keyOf := func(p appPath) string {
		return events.EndpointKey(routeOf[p], p.Path)
	}

	if err := rekeyStats(db, keyOf); err != nil {
		return err
	}
	if err := mergeStates(db, "spike_states", keyOf); err != nil {
		return err
	}
	return mergeStates(db, "silence_states", keyOf)
}

// rekeyStats rewrites the rows of every endpoint that more than one path, or
// a path other than its own, lands on. The other rows stay as they are.
func rekeyStats(db *gorm.DB, keyOf func(appPath) string) error {
	var stored []appPath
	if err := db.Raw("SELECT DISTINCT api_key_id, path FROM endpoint_stats").Scan(&stored).Error; err != nil {
		return err
	}
	moved := map[appPath]bool{} // the endpoints that a path moves to
	for _, p := range stored {
		if key := keyOf(p); key != p.Path {
			moved[appPath{p.APIKeyID, key}] = true
		}
	}
	if len(moved) == 0 {
		return nil
	}

	type mapping struct {
		APIKeyID  uint
		Path, Key string
	}
	var rekey []mapping
	for _, p := range stored {
		if key := keyOf(p); moved[appPath{p.APIKeyID, key}] {
			rekey = append(rekey, mapping{p.APIKeyID, p.Path, key})
		}
	}

	statements := []string{
		"CREATE TEMP TABLE endpoint_rekey (api_key_id INTEGER, path TEXT, key TEXT, PRIMARY KEY (api_key_id, path))",
	}
	for _, sql := range statements {
		if err := db.Exec(sql).Error; err != nil {
			return err
		}
	}
	if err := db.Table("endpoint_rekey").CreateInBatches(rekey, 500).Error; err != nil {
		return err
	}
	for _, sql := range []string{
		`CREATE TEMP TABLE endpoint_merged AS
			SELECT r.api_key_id, r.key AS path, s.bucket, SUM(s.total) AS total, SUM(s.errors) AS errors,
				CASE WHEN SUM(s.total) > 0 THEN SUM(s.avg_duration * s.total) / SUM(s.total) ELSE 0 END AS avg_duration
			FROM endpoint_stats s JOIN endpoint_rekey r ON r.api_key_id = s.api_key_id AND r.path = s.path
			GROUP BY r.api_key_id, r.key, s.bucket`,
		`DELETE FROM endpoint_stats WHERE EXISTS (
			SELECT 1 FROM endpoint_rekey r WHERE r.api_key_id = endpoint_stats.api_key_id AND r.path = endpoint_stats.path)`,
		`INSERT INTO endpoint_stats (api_key_id, path, bucket, total, errors, avg_duration)
			SELECT api_key_id, path, bucket, total, errors, avg_duration FROM endpoint_merged`,
		"DROP TABLE endpoint_merged",
		"DROP TABLE endpoint_rekey",
	} {
		if err := db.Exec(sql).Error; err != nil {
			return err
		}
	}
	return nil
}

// alertState is a row of spike_states or silence_states: the alert state of
// an endpoint, unique per app and path.
type alertState struct {
	ID             uint
	APIKeyID       uint
	Path           string
	Severity       string
	CleanChecks    int
	ErrorCount     int        // spike_states only
	SpikeStartAt   *time.Time // spike_states only
	SilenceStartAt *time.Time // silence_states only
	LastAlertAt    *time.Time
}

// mergeStates moves the alert states of an endpoint's paths onto the
// endpoint, keeping the strongest: the highest severity, the latest alert
// (so the cooldown holds and a running alert does not email again), the
// earliest start, the errors added up, and the fewest clean checks.
func mergeStates(db *gorm.DB, table string, keyOf func(appPath) string) error {
	var rows []alertState
	if err := db.Table(table).Find(&rows).Error; err != nil {
		return err
	}
	groups := map[appPath][]alertState{}
	var order []appPath
	for _, r := range rows {
		key := appPath{r.APIKeyID, keyOf(appPath{r.APIKeyID, r.Path})}
		if groups[key] == nil {
			order = append(order, key)
		}
		groups[key] = append(groups[key], r)
	}

	for _, key := range order {
		group := groups[key]
		if len(group) == 1 && group[0].Path == key.Path {
			continue
		}
		// GORM stores an unset time as the zero time, never NULL: the alert
		// queries compare last_alert_at, and NULL compares as nothing.
		merged := map[string]any{"api_key_id": key.APIKeyID, "path": key.Path, "severity": "",
			"clean_checks": group[0].CleanChecks, "last_alert_at": time.Time{}}
		if table == "spike_states" {
			merged["spike_start_at"] = time.Time{}
		} else {
			merged["silence_start_at"] = time.Time{}
		}
		var ids []uint
		for _, r := range group {
			ids = append(ids, r.ID)
			merged["severity"] = strongerSeverity(merged["severity"].(string), r.Severity)
			merged["clean_checks"] = min(merged["clean_checks"].(int), r.CleanChecks)
			if table == "spike_states" {
				merged["error_count"] = asInt(merged["error_count"]) + r.ErrorCount
				merged["spike_start_at"] = earliest(merged["spike_start_at"], r.SpikeStartAt)
			} else {
				merged["silence_start_at"] = earliest(merged["silence_start_at"], r.SilenceStartAt)
			}
			merged["last_alert_at"] = latest(merged["last_alert_at"], r.LastAlertAt)
		}
		if err := db.Table(table).Where("id IN ?", ids).Delete(&alertState{}).Error; err != nil {
			return err
		}
		if err := db.Table(table).Create(merged).Error; err != nil {
			return err
		}
	}
	return nil
}

func strongerSeverity(a, b string) string {
	rank := map[string]int{"": 0, "warning": 1, "critical": 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func asInt(v any) int {
	n, _ := v.(int)
	return n
}

// earliest and latest keep the time that is set, the zero time counting as
// unset, as GORM stores it.
func earliest(current any, t *time.Time) any {
	if t == nil || t.IsZero() {
		return current
	}
	if c, ok := current.(time.Time); ok && !c.IsZero() && c.Before(*t) {
		return c
	}
	return *t
}

func latest(current any, t *time.Time) any {
	if t == nil || t.IsZero() {
		return current
	}
	if c, ok := current.(time.Time); ok && c.After(*t) {
		return c
	}
	return *t
}
