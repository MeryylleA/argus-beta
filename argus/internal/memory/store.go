package memory

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// --- Domain types ---

type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	RootPath  string    `json:"root_path"`
	Config    string    `json:"config"` // JSON: scope, focus, bounty_program
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Session struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	ModelA       string    `json:"model_a"`
	ModelB       string    `json:"model_b,omitempty"`
	Mode         string    `json:"mode"`   // "single" | "collaborative"
	Status       string    `json:"status"` // "running" | "completed" | "failed" | "cancelled"
	StartedAt    time.Time `json:"started_at"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
	TotalCostUSD float64   `json:"total_cost_usd"`
}

type Finding struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	ProjectID   string    `json:"project_id"`
	Title       string    `json:"title"`
	Location    string    `json:"location"`
	Category    string    `json:"category,omitempty"`
	Severity    string    `json:"severity"`
	Confidence  string    `json:"confidence"`
	Description string    `json:"description"`
	DataFlow    string    `json:"data_flow,omitempty"`
	FoundBy     string    `json:"found_by"` // "agent_a" | "agent_b" | "single"
	CreatedAt   time.Time `json:"created_at"`
}

type InvestigatedArea struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	SessionID string    `json:"session_id"`
	Path      string    `json:"path"`
	Pattern   string    `json:"pattern"`
	Agent     string    `json:"agent"`
	CreatedAt time.Time `json:"created_at"`
}

type ChannelMessage struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	FromAgent string    `json:"from_agent"`
	ToAgent   string    `json:"to_agent"`
	MsgType   string    `json:"msg_type"` // "finding" | "question" | "context" | "duplicate"
	Content   string    `json:"content"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

// --- Store interface ---

type Store interface {
	// Projects
	CreateProject(ctx context.Context, p *Project) error
	GetProject(ctx context.Context, id string) (*Project, error)
	GetProjectByPath(ctx context.Context, rootPath string) (*Project, error)
	ListProjects(ctx context.Context) ([]*Project, error)

	// Sessions
	CreateSession(ctx context.Context, s *Session) error
	UpdateSessionStatus(ctx context.Context, id, status string) error
	UpdateSessionCost(ctx context.Context, id string, costUSD float64) error

	// Findings
	CreateFinding(ctx context.Context, f *Finding) error
	ListFindings(ctx context.Context, projectID string) ([]*Finding, error)
	FindingExists(ctx context.Context, projectID, location, title string) (bool, error)

	// Investigated areas
	MarkInvestigated(ctx context.Context, area *InvestigatedArea) error
	GetInvestigatedAreas(ctx context.Context, projectID string) ([]*InvestigatedArea, error)

	// Channel (collaborative mode)
	PostMessage(ctx context.Context, msg *ChannelMessage) error
	PollMessages(ctx context.Context, sessionID, toAgent string) ([]*ChannelMessage, error)
	MarkMessagesRead(ctx context.Context, sessionID, toAgent string) error

	Close() error
}

// --- SQLite implementation ---

type sqliteStore struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    root_path   TEXT NOT NULL UNIQUE,
    config      TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    model_a     TEXT NOT NULL,
    model_b     TEXT,
    mode        TEXT NOT NULL,
    status      TEXT NOT NULL,
    started_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at    DATETIME,
    total_cost_usd REAL DEFAULT 0.0
);

CREATE TABLE IF NOT EXISTS findings (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id),
    project_id  TEXT NOT NULL REFERENCES projects(id),
    title       TEXT NOT NULL,
    location    TEXT NOT NULL,
    category    TEXT,
    severity    TEXT,
    confidence  TEXT,
    description TEXT NOT NULL,
    data_flow   TEXT,
    found_by    TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project_id, location, title)
);

CREATE TABLE IF NOT EXISTS investigated_areas (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    session_id  TEXT NOT NULL REFERENCES sessions(id),
    path        TEXT NOT NULL,
    pattern     TEXT NOT NULL,
    agent       TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project_id, path, pattern)
);

CREATE TABLE IF NOT EXISTS channel_messages (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id),
    from_agent  TEXT NOT NULL,
    to_agent    TEXT NOT NULL,
    msg_type    TEXT NOT NULL,
    content     TEXT NOT NULL,
    read        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

// DefaultDBPath returns the default database path (~/.config/argus/argus.db).
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("memory: cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "argus", "argus.db"), nil
}

// NewStore opens (or creates) a SQLite database at the given path and initializes the schema.
func NewStore(dbPath string) (Store, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("memory: failed to create directory %s: %w", dir, err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("memory: failed to open database %s: %w", dbPath, err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("memory: failed to initialize schema: %w", err)
	}

	return &sqliteStore{db: db}, nil
}

func (s *sqliteStore) Close() error {
	return s.db.Close()
}

// --- Projects ---

func (s *sqliteStore) CreateProject(ctx context.Context, p *Project) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO projects (id, name, root_path, config, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.RootPath, p.Config, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("memory: create project: %w", err)
	}
	return nil
}

func (s *sqliteStore) GetProject(ctx context.Context, id string) (*Project, error) {
	p := &Project{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, root_path, config, created_at, updated_at FROM projects WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.RootPath, &p.Config, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("memory: project %q not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("memory: get project: %w", err)
	}
	return p, nil
}

func (s *sqliteStore) GetProjectByPath(ctx context.Context, rootPath string) (*Project, error) {
	p := &Project{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, root_path, config, created_at, updated_at FROM projects WHERE root_path = ?`, rootPath).
		Scan(&p.ID, &p.Name, &p.RootPath, &p.Config, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("memory: project with path %q not found", rootPath)
	}
	if err != nil {
		return nil, fmt.Errorf("memory: get project by path: %w", err)
	}
	return p, nil
}

func (s *sqliteStore) ListProjects(ctx context.Context) ([]*Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, root_path, config, created_at, updated_at FROM projects ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("memory: list projects: %w", err)
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		p := &Project{}
		if err := rows.Scan(&p.ID, &p.Name, &p.RootPath, &p.Config, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("memory: list projects scan: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// --- Sessions ---

func (s *sqliteStore) CreateSession(ctx context.Context, sess *Session) error {
	if sess.ID == "" {
		sess.ID = uuid.New().String()
	}
	sess.StartedAt = time.Now().UTC()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, project_id, model_a, model_b, mode, status, started_at, total_cost_usd)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.ProjectID, sess.ModelA, sess.ModelB, sess.Mode, sess.Status, sess.StartedAt, sess.TotalCostUSD)
	if err != nil {
		return fmt.Errorf("memory: create session: %w", err)
	}
	return nil
}

func (s *sqliteStore) UpdateSessionStatus(ctx context.Context, id, status string) error {
	var endedAt *time.Time
	if status == "completed" || status == "failed" || status == "cancelled" {
		now := time.Now().UTC()
		endedAt = &now
	}

	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET status = ?, ended_at = ? WHERE id = ?`,
		status, endedAt, id)
	if err != nil {
		return fmt.Errorf("memory: update session status: %w", err)
	}
	return nil
}

func (s *sqliteStore) UpdateSessionCost(ctx context.Context, id string, costUSD float64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET total_cost_usd = total_cost_usd + ? WHERE id = ?`,
		costUSD, id)
	if err != nil {
		return fmt.Errorf("memory: update session cost: %w", err)
	}
	return nil
}

// --- Findings ---

func (s *sqliteStore) CreateFinding(ctx context.Context, f *Finding) error {
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	f.CreatedAt = time.Now().UTC()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO findings (id, session_id, project_id, title, location, category, severity, confidence, description, data_flow, found_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.SessionID, f.ProjectID, f.Title, f.Location, f.Category, f.Severity, f.Confidence,
		f.Description, f.DataFlow, f.FoundBy, f.CreatedAt)
	if err != nil {
		return fmt.Errorf("memory: create finding: %w", err)
	}
	return nil
}

func (s *sqliteStore) ListFindings(ctx context.Context, projectID string) ([]*Finding, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, project_id, title, location, category, severity, confidence, description, data_flow, found_by, created_at
		 FROM findings WHERE project_id = ? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("memory: list findings: %w", err)
	}
	defer rows.Close()

	var findings []*Finding
	for rows.Next() {
		f := &Finding{}
		var category, dataFlow sql.NullString
		if err := rows.Scan(&f.ID, &f.SessionID, &f.ProjectID, &f.Title, &f.Location,
			&category, &f.Severity, &f.Confidence, &f.Description, &dataFlow, &f.FoundBy, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("memory: list findings scan: %w", err)
		}
		f.Category = category.String
		f.DataFlow = dataFlow.String
		findings = append(findings, f)
	}
	return findings, rows.Err()
}

func (s *sqliteStore) FindingExists(ctx context.Context, projectID, location, title string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM findings WHERE project_id = ? AND location = ? AND title = ?`,
		projectID, location, title).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("memory: finding exists check: %w", err)
	}
	return count > 0, nil
}

// --- Investigated Areas ---

func (s *sqliteStore) MarkInvestigated(ctx context.Context, area *InvestigatedArea) error {
	if area.ID == "" {
		area.ID = uuid.New().String()
	}
	area.CreatedAt = time.Now().UTC()

	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO investigated_areas (id, project_id, session_id, path, pattern, agent, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		area.ID, area.ProjectID, area.SessionID, area.Path, area.Pattern, area.Agent, area.CreatedAt)
	if err != nil {
		return fmt.Errorf("memory: mark investigated: %w", err)
	}
	return nil
}

func (s *sqliteStore) GetInvestigatedAreas(ctx context.Context, projectID string) ([]*InvestigatedArea, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, session_id, path, pattern, agent, created_at
		 FROM investigated_areas WHERE project_id = ? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("memory: get investigated areas: %w", err)
	}
	defer rows.Close()

	var areas []*InvestigatedArea
	for rows.Next() {
		a := &InvestigatedArea{}
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.SessionID, &a.Path, &a.Pattern, &a.Agent, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("memory: get investigated areas scan: %w", err)
		}
		areas = append(areas, a)
	}
	return areas, rows.Err()
}

// --- Channel Messages ---

func (s *sqliteStore) PostMessage(ctx context.Context, msg *ChannelMessage) error {
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	msg.CreatedAt = time.Now().UTC()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO channel_messages (id, session_id, from_agent, to_agent, msg_type, content, read, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.SessionID, msg.FromAgent, msg.ToAgent, msg.MsgType, msg.Content, false, msg.CreatedAt)
	if err != nil {
		return fmt.Errorf("memory: post message: %w", err)
	}
	return nil
}

func (s *sqliteStore) PollMessages(ctx context.Context, sessionID, toAgent string) ([]*ChannelMessage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, from_agent, to_agent, msg_type, content, read, created_at
		 FROM channel_messages WHERE session_id = ? AND to_agent = ? AND read = FALSE
		 ORDER BY created_at ASC`, sessionID, toAgent)
	if err != nil {
		return nil, fmt.Errorf("memory: poll messages: %w", err)
	}
	defer rows.Close()

	var messages []*ChannelMessage
	for rows.Next() {
		m := &ChannelMessage{}
		if err := rows.Scan(&m.ID, &m.SessionID, &m.FromAgent, &m.ToAgent, &m.MsgType, &m.Content, &m.Read, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("memory: poll messages scan: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (s *sqliteStore) MarkMessagesRead(ctx context.Context, sessionID, toAgent string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE channel_messages SET read = TRUE WHERE session_id = ? AND to_agent = ? AND read = FALSE`,
		sessionID, toAgent)
	if err != nil {
		return fmt.Errorf("memory: mark messages read: %w", err)
	}
	return nil
}
