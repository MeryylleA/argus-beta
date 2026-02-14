package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// testStore creates a temporary SQLite store and returns it with a cleanup function.
func testStore(t *testing.T) (Store, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s, func() { s.Close() }
}

func TestNewStoreCreatesDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "subdir", "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("expected database file to be created")
	}
}

func TestProjectCRUD(t *testing.T) {
	s, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	// Create
	p := &Project{Name: "test-project", RootPath: "/tmp/test", Config: "{}"}
	if err := s.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID == "" {
		t.Fatal("expected project ID to be assigned")
	}

	// Get by ID
	got, err := s.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.Name != "test-project" {
		t.Errorf("Name = %q, want %q", got.Name, "test-project")
	}
	if got.RootPath != "/tmp/test" {
		t.Errorf("RootPath = %q, want %q", got.RootPath, "/tmp/test")
	}

	// Get by path
	got2, err := s.GetProjectByPath(ctx, "/tmp/test")
	if err != nil {
		t.Fatalf("GetProjectByPath: %v", err)
	}
	if got2.ID != p.ID {
		t.Errorf("GetProjectByPath returned different ID: %q vs %q", got2.ID, p.ID)
	}

	// List
	list, err := s.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("ListProjects returned %d projects, want 1", len(list))
	}
}

func TestProjectUniquePathConstraint(t *testing.T) {
	s, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	p1 := &Project{Name: "proj1", RootPath: "/tmp/unique", Config: "{}"}
	if err := s.CreateProject(ctx, p1); err != nil {
		t.Fatalf("CreateProject #1: %v", err)
	}

	p2 := &Project{Name: "proj2", RootPath: "/tmp/unique", Config: "{}"}
	if err := s.CreateProject(ctx, p2); err == nil {
		t.Fatal("expected error for duplicate root_path")
	}
}

func TestGetProjectNotFound(t *testing.T) {
	s, cleanup := testStore(t)
	defer cleanup()

	_, err := s.GetProject(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}
}

func TestSessionLifecycle(t *testing.T) {
	s, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	// Need a project first
	p := &Project{Name: "proj", RootPath: "/tmp/sess", Config: "{}"}
	if err := s.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	sess := &Session{
		ProjectID: p.ID,
		ModelA:    "claude-opus-4-6",
		Mode:      "single",
		Status:    "running",
	}
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("expected session ID to be assigned")
	}

	// Update status
	if err := s.UpdateSessionStatus(ctx, sess.ID, "completed"); err != nil {
		t.Fatalf("UpdateSessionStatus: %v", err)
	}

	// Update cost
	if err := s.UpdateSessionCost(ctx, sess.ID, 1.50); err != nil {
		t.Fatalf("UpdateSessionCost: %v", err)
	}
}

func TestFindingDedup(t *testing.T) {
	s, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	p := &Project{Name: "proj", RootPath: "/tmp/dedup", Config: "{}"}
	if err := s.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	sess := &Session{ProjectID: p.ID, ModelA: "gpt-5.2", Mode: "single", Status: "running"}
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	f := &Finding{
		SessionID:   sess.ID,
		ProjectID:   p.ID,
		Title:       "SQL Injection in login",
		Location:    "auth/login.go:42",
		Severity:    "high",
		Confidence:  "confirmed",
		Description: "Unsanitized input in SQL query",
		FoundBy:     "single",
	}
	if err := s.CreateFinding(ctx, f); err != nil {
		t.Fatalf("CreateFinding #1: %v", err)
	}

	// Duplicate (same project, location, title) should fail
	dup := &Finding{
		SessionID:   sess.ID,
		ProjectID:   p.ID,
		Title:       "SQL Injection in login",
		Location:    "auth/login.go:42",
		Severity:    "critical",
		Confidence:  "confirmed",
		Description: "Same issue, different session",
		FoundBy:     "single",
	}
	if err := s.CreateFinding(ctx, dup); err == nil {
		t.Fatal("expected error for duplicate finding (same project, location, title)")
	}

	// FindingExists should return true for the existing finding
	exists, err := s.FindingExists(ctx, p.ID, "auth/login.go:42", "SQL Injection in login")
	if err != nil {
		t.Fatalf("FindingExists: %v", err)
	}
	if !exists {
		t.Error("FindingExists should return true for existing finding")
	}

	// FindingExists should return false for a different title
	exists2, err := s.FindingExists(ctx, p.ID, "auth/login.go:42", "XSS in login")
	if err != nil {
		t.Fatalf("FindingExists: %v", err)
	}
	if exists2 {
		t.Error("FindingExists should return false for nonexistent finding")
	}

	// List should return 1
	findings, err := s.ListFindings(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListFindings: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("ListFindings returned %d findings, want 1", len(findings))
	}
}

func TestInvestigatedAreas(t *testing.T) {
	s, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	p := &Project{Name: "proj", RootPath: "/tmp/invest", Config: "{}"}
	if err := s.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	sess := &Session{ProjectID: p.ID, ModelA: "glm-5", Mode: "single", Status: "running"}
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	area := &InvestigatedArea{
		ProjectID: p.ID,
		SessionID: sess.ID,
		Path:      "auth/",
		Pattern:   "SQL injection patterns",
		Agent:     "single",
	}
	if err := s.MarkInvestigated(ctx, area); err != nil {
		t.Fatalf("MarkInvestigated: %v", err)
	}

	// INSERT OR IGNORE: duplicate should not error
	area2 := &InvestigatedArea{
		ProjectID: p.ID,
		SessionID: sess.ID,
		Path:      "auth/",
		Pattern:   "SQL injection patterns",
		Agent:     "single",
	}
	if err := s.MarkInvestigated(ctx, area2); err != nil {
		t.Fatalf("MarkInvestigated duplicate should not error: %v", err)
	}

	areas, err := s.GetInvestigatedAreas(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetInvestigatedAreas: %v", err)
	}
	if len(areas) != 1 {
		t.Errorf("GetInvestigatedAreas returned %d areas, want 1", len(areas))
	}
}

func TestChannelMessages(t *testing.T) {
	s, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	p := &Project{Name: "proj", RootPath: "/tmp/chan", Config: "{}"}
	if err := s.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	sess := &Session{ProjectID: p.ID, ModelA: "claude-opus-4-6", ModelB: "gpt-5.2", Mode: "collaborative", Status: "running"}
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Post from agent_a to agent_b
	msg := &ChannelMessage{
		SessionID: sess.ID,
		FromAgent: "agent_a",
		ToAgent:   "agent_b",
		MsgType:   "finding",
		Content:   "Found SQLi in login.go",
	}
	if err := s.PostMessage(ctx, msg); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	// Poll for agent_b — should see the message
	msgs, err := s.PollMessages(ctx, sess.ID, "agent_b")
	if err != nil {
		t.Fatalf("PollMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("PollMessages returned %d messages, want 1", len(msgs))
	}
	if msgs[0].Content != "Found SQLi in login.go" {
		t.Errorf("Content = %q, want %q", msgs[0].Content, "Found SQLi in login.go")
	}

	// Poll for agent_a — should see nothing
	msgs2, err := s.PollMessages(ctx, sess.ID, "agent_a")
	if err != nil {
		t.Fatalf("PollMessages for agent_a: %v", err)
	}
	if len(msgs2) != 0 {
		t.Errorf("PollMessages for agent_a returned %d messages, want 0", len(msgs2))
	}

	// Mark messages read
	if err := s.MarkMessagesRead(ctx, sess.ID, "agent_b"); err != nil {
		t.Fatalf("MarkMessagesRead: %v", err)
	}

	// Poll again — should be empty now
	msgs3, err := s.PollMessages(ctx, sess.ID, "agent_b")
	if err != nil {
		t.Fatalf("PollMessages after read: %v", err)
	}
	if len(msgs3) != 0 {
		t.Errorf("PollMessages after MarkMessagesRead returned %d, want 0", len(msgs3))
	}
}
