package config

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schemaVersion = 1

var schemaDDL = []string{
	`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`,

	`CREATE TABLE IF NOT EXISTS credentials (
		id      INTEGER PRIMARY KEY AUTOINCREMENT,
		service TEXT NOT NULL,
		name    TEXT NOT NULL,
		key     TEXT NOT NULL,
		value   TEXT NOT NULL,
		UNIQUE(service, name, key)
	)`,

	`CREATE TABLE IF NOT EXISTS servers (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		name            TEXT NOT NULL UNIQUE,
		ip              TEXT NOT NULL,
		hetzner_project TEXT NOT NULL,
		hetzner_id      INTEGER NOT NULL
	)`,

	`CREATE TABLE IF NOT EXISTS peon_keys (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		server_ip   TEXT NOT NULL UNIQUE,
		private_key TEXT NOT NULL,
		key_path    TEXT NOT NULL
	)`,

	`CREATE TABLE IF NOT EXISTS projects (
		id     INTEGER PRIMARY KEY AUTOINCREMENT,
		name   TEXT NOT NULL UNIQUE,
		repo   TEXT NOT NULL,
		server TEXT NOT NULL
	)`,

	`CREATE TABLE IF NOT EXISTS environments (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id   INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		env_name     TEXT NOT NULL,
		domain       TEXT NOT NULL,
		dns_provider TEXT NOT NULL,
		branch       TEXT NOT NULL,
		deploy_path  TEXT NOT NULL,
		deploy_user  TEXT NOT NULL,
		port         INTEGER NOT NULL,
		UNIQUE(project_id, env_name)
	)`,
}

// SQLiteStore implements Store backed by a SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// DBPath returns the default database path.
func DBPath() string {
	return filepath.Join(os.Getenv("HOME"), ".config", "arnor", "arnor.db")
}

// NewSQLiteStore opens (or creates) a SQLite database at dbPath and ensures
// the schema is up to date. File permissions are set to 0600.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating config dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Limit to a single connection. This is fine for a CLI tool and
	// avoids the :memory: issue where each connection gets its own DB.
	db.SetMaxOpenConns(1)

	// Enable WAL mode and foreign keys.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	// Create schema — execute each statement individually.
	for _, stmt := range schemaDDL {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("creating schema: %w", err)
		}
	}

	// Set version if empty.
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count); err != nil {
		db.Close()
		return nil, fmt.Errorf("checking schema version: %w", err)
	}
	if count == 0 {
		if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (?)", schemaVersion); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting schema version: %w", err)
		}
	}

	// Restrict file permissions (best-effort on the file).
	_ = os.Chmod(dbPath, 0o600)

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// --- Credentials ---

func (s *SQLiteStore) GetCredential(service, name, key string) (string, error) {
	var value string
	err := s.db.QueryRow(
		"SELECT value FROM credentials WHERE service = ? AND name = ? AND key = ?",
		service, name, key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("credential not found: %s/%s/%s", service, name, key)
	}
	if err != nil {
		return "", fmt.Errorf("querying credential: %w", err)
	}
	return value, nil
}

func (s *SQLiteStore) SetCredential(service, name, key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO credentials (service, name, key, value) VALUES (?, ?, ?, ?)
		 ON CONFLICT(service, name, key) DO UPDATE SET value = excluded.value`,
		service, name, key, value,
	)
	if err != nil {
		return fmt.Errorf("setting credential: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListCredentials(service string) ([]Credential, error) {
	rows, err := s.db.Query(
		"SELECT service, name, key, value FROM credentials WHERE service = ? ORDER BY name, key",
		service,
	)
	if err != nil {
		return nil, fmt.Errorf("listing credentials: %w", err)
	}
	defer rows.Close()

	var creds []Credential
	for rows.Next() {
		var c Credential
		if err := rows.Scan(&c.Service, &c.Name, &c.Key, &c.Value); err != nil {
			return nil, fmt.Errorf("scanning credential: %w", err)
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

func (s *SQLiteStore) DeleteCredential(service, name string) error {
	_, err := s.db.Exec(
		"DELETE FROM credentials WHERE service = ? AND name = ?",
		service, name,
	)
	if err != nil {
		return fmt.Errorf("deleting credential: %w", err)
	}
	return nil
}

// --- Peon Keys ---

func (s *SQLiteStore) GetPeonKey(serverIP string) (string, error) {
	var privateKey string
	err := s.db.QueryRow(
		"SELECT private_key FROM peon_keys WHERE server_ip = ?",
		serverIP,
	).Scan(&privateKey)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("peon key not found for %s", serverIP)
	}
	if err != nil {
		return "", fmt.Errorf("querying peon key: %w", err)
	}
	return privateKey, nil
}

func (s *SQLiteStore) SetPeonKey(serverIP, privateKey, keyPath string) error {
	_, err := s.db.Exec(
		`INSERT INTO peon_keys (server_ip, private_key, key_path) VALUES (?, ?, ?)
		 ON CONFLICT(server_ip) DO UPDATE SET private_key = excluded.private_key, key_path = excluded.key_path`,
		serverIP, privateKey, keyPath,
	)
	if err != nil {
		return fmt.Errorf("setting peon key: %w", err)
	}
	return nil
}

// --- Hetzner Projects ---

func (s *SQLiteStore) ListHetznerProjects() ([]HetznerProject, error) {
	rows, err := s.db.Query(
		"SELECT DISTINCT name FROM credentials WHERE service = 'hetzner' AND key = 'api_token' ORDER BY name",
	)
	if err != nil {
		return nil, fmt.Errorf("listing hetzner projects: %w", err)
	}
	defer rows.Close()

	var projects []HetznerProject
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, fmt.Errorf("scanning hetzner project: %w", err)
		}
		projects = append(projects, HetznerProject{Alias: alias})
	}
	return projects, rows.Err()
}

// --- Config (LoadConfig / SaveConfig) ---

func (s *SQLiteStore) LoadConfig() (*Config, error) {
	cfg := &Config{}

	// Load Hetzner projects from credentials.
	cfg.HetznerProjects, _ = s.ListHetznerProjects()

	// Load servers.
	serverRows, err := s.db.Query("SELECT name, ip, hetzner_project, hetzner_id FROM servers ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("loading servers: %w", err)
	}
	for serverRows.Next() {
		var srv Server
		if err := serverRows.Scan(&srv.Name, &srv.IP, &srv.HetznerProject, &srv.HetznerID); err != nil {
			serverRows.Close()
			return nil, fmt.Errorf("scanning server: %w", err)
		}
		cfg.Servers = append(cfg.Servers, srv)
	}
	if err := serverRows.Err(); err != nil {
		serverRows.Close()
		return nil, fmt.Errorf("iterating servers: %w", err)
	}
	serverRows.Close()

	// Load projects and environments with a LEFT JOIN to avoid nested queries
	// (needed because we use MaxOpenConns=1).
	rows, err := s.db.Query(`
		SELECT p.name, p.repo, p.server,
		       e.env_name, e.domain, e.dns_provider, e.branch, e.deploy_path, e.deploy_user, e.port
		FROM projects p
		LEFT JOIN environments e ON e.project_id = p.id
		ORDER BY p.name, e.env_name
	`)
	if err != nil {
		return nil, fmt.Errorf("loading projects: %w", err)
	}
	defer rows.Close()

	projectMap := make(map[string]*Project)
	var projectOrder []string

	for rows.Next() {
		var pName, pRepo, pServer string
		var envName, domain, dnsProvider, branch, deployPath, deployUser sql.NullString
		var port sql.NullInt64

		if err := rows.Scan(&pName, &pRepo, &pServer, &envName, &domain, &dnsProvider, &branch, &deployPath, &deployUser, &port); err != nil {
			return nil, fmt.Errorf("scanning project row: %w", err)
		}

		p, ok := projectMap[pName]
		if !ok {
			p = &Project{
				Name:         pName,
				Repo:         pRepo,
				Server:       pServer,
				Environments: make(map[string]Environment),
			}
			projectMap[pName] = p
			projectOrder = append(projectOrder, pName)
		}

		if envName.Valid {
			p.Environments[envName.String] = Environment{
				Domain:      domain.String,
				DNSProvider: dnsProvider.String,
				Branch:      branch.String,
				DeployPath:  deployPath.String,
				DeployUser:  deployUser.String,
				Port:        int(port.Int64),
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating projects: %w", err)
	}

	for _, name := range projectOrder {
		cfg.Projects = append(cfg.Projects, *projectMap[name])
	}

	return cfg, nil
}

func (s *SQLiteStore) SaveConfig(cfg *Config) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Upsert servers.
	for _, srv := range cfg.Servers {
		_, err := tx.Exec(
			`INSERT INTO servers (name, ip, hetzner_project, hetzner_id) VALUES (?, ?, ?, ?)
			 ON CONFLICT(name) DO UPDATE SET ip = excluded.ip, hetzner_project = excluded.hetzner_project, hetzner_id = excluded.hetzner_id`,
			srv.Name, srv.IP, srv.HetznerProject, srv.HetznerID,
		)
		if err != nil {
			return fmt.Errorf("upserting server %s: %w", srv.Name, err)
		}
	}

	// Upsert projects and environments.
	for _, p := range cfg.Projects {
		_, err := tx.Exec(
			`INSERT INTO projects (name, repo, server) VALUES (?, ?, ?)
			 ON CONFLICT(name) DO UPDATE SET repo = excluded.repo, server = excluded.server`,
			p.Name, p.Repo, p.Server,
		)
		if err != nil {
			return fmt.Errorf("upserting project %s: %w", p.Name, err)
		}

		var projectID int
		if err := tx.QueryRow("SELECT id FROM projects WHERE name = ?", p.Name).Scan(&projectID); err != nil {
			return fmt.Errorf("getting project ID for %s: %w", p.Name, err)
		}

		for envName, env := range p.Environments {
			_, err := tx.Exec(
				`INSERT INTO environments (project_id, env_name, domain, dns_provider, branch, deploy_path, deploy_user, port)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				 ON CONFLICT(project_id, env_name) DO UPDATE SET
				   domain = excluded.domain,
				   dns_provider = excluded.dns_provider,
				   branch = excluded.branch,
				   deploy_path = excluded.deploy_path,
				   deploy_user = excluded.deploy_user,
				   port = excluded.port`,
				projectID, envName, env.Domain, env.DNSProvider, env.Branch, env.DeployPath, env.DeployUser, env.Port,
			)
			if err != nil {
				return fmt.Errorf("upserting environment %s/%s: %w", p.Name, envName, err)
			}
		}
	}

	return tx.Commit()
}
