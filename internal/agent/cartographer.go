// Package agent — Cartographer module.
//
// Cartographer is a deterministic, zero-LLM static analysis module that
// fast-scans a target codebase through the Sandbox to produce an
// ARCHITECTURE.md-style summary. The output is pre-injected into the
// agent's Whiteboard (scratchpad) before the main LLM loop begins,
// giving the model instant context of the project topology.
package agent

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/argus-sec/argus/internal/logger"
)

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Cartographer performs deterministic static analysis of a workspace to
// produce a compact architecture summary. It operates entirely through the
// Sandbox so all path-traversal and symlink protections remain in effect.
type Cartographer struct {
	sandbox *Sandbox
}

// NewCartographer creates a Cartographer bound to the given Sandbox.
func NewCartographer(sandbox *Sandbox) *Cartographer {
	return &Cartographer{sandbox: sandbox}
}

// Survey walks the workspace and returns a Markdown-formatted architecture
// summary suitable for injection into the agent's Whiteboard.
// It never returns an error that should abort the agent — on failure it
// returns a best-effort partial result or a short notice.
func (c *Cartographer) Survey() string {
	logger.Info("Cartographer: beginning workspace survey …")

	snap := c.collectSnapshot()
	md := c.renderMarkdown(snap)

	logger.Success("Cartographer: survey complete (%d files, %d dirs, %d languages detected)",
		snap.TotalFiles, snap.TotalDirs, len(snap.Languages))
	return md
}

// ---------------------------------------------------------------------------
// Internal data model
// ---------------------------------------------------------------------------

// snapshot holds all the raw data collected during the walk.
type snapshot struct {
	TotalFiles int
	TotalDirs  int

	// Language → file count.
	Languages map[string]int

	// Detected manifest / config files (relative paths).
	Manifests []string

	// Detected frameworks / runtimes (deduplicated labels).
	Frameworks []string

	// Top-level directory names (first level only).
	TopDirs []string

	// Entry-point candidates (e.g. main.go, index.js, app.py …).
	EntryPoints []string

	// Security-relevant files (e.g. .env, Dockerfile, auth modules …).
	SecurityFiles []string

	// Dependency files content summaries (package.json deps, go.mod, etc.).
	DependencySummaries []string

	// Directory tree (compact, max 3 levels).
	Tree string
}

// ---------------------------------------------------------------------------
// Collection
// ---------------------------------------------------------------------------

func (c *Cartographer) collectSnapshot() snapshot {
	snap := snapshot{
		Languages: make(map[string]int),
	}

	root := c.sandbox.Root()

	// --- 1. Build compact directory tree (max 3 levels) ---
	snap.Tree = c.buildTree(root, "", 0, 3)

	// --- 2. Walk the entire workspace ---
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}

		name := d.Name()

		// Skip banned directories.
		if d.IsDir() {
			if bannedDirs[name] || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			snap.TotalDirs++
			return nil
		}

		snap.TotalFiles++

		rel, _ := filepath.Rel(root, path)

		// Language detection by extension.
		ext := strings.ToLower(filepath.Ext(name))
		if lang, ok := extToLang[ext]; ok {
			snap.Languages[lang]++
		}

		// Manifest / config detection.
		if isManifest(name) {
			snap.Manifests = append(snap.Manifests, rel)
		}

		// Entry-point detection.
		if isEntryPoint(name) {
			snap.EntryPoints = append(snap.EntryPoints, rel)
		}

		// Security-relevant file detection.
		if isSecurityRelevant(name, rel) {
			snap.SecurityFiles = append(snap.SecurityFiles, rel)
		}

		return nil
	})

	// --- 3. Top-level directories ---
	if entries, err := os.ReadDir(root); err == nil {
		for _, e := range entries {
			if e.IsDir() && !bannedDirs[e.Name()] && !strings.HasPrefix(e.Name(), ".") {
				snap.TopDirs = append(snap.TopDirs, e.Name()+"/")
			}
		}
	}

	// --- 4. Framework detection from manifests ---
	snap.Frameworks = c.detectFrameworks(root, snap.Manifests)

	// --- 5. Dependency summaries (lightweight) ---
	snap.DependencySummaries = c.extractDependencySummaries(root, snap.Manifests)

	return snap
}

// ---------------------------------------------------------------------------
// Tree builder
// ---------------------------------------------------------------------------

func (c *Cartographer) buildTree(dir, prefix string, depth, maxDepth int) string {
	if depth >= maxDepth {
		return ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	// Filter out banned / hidden dirs and banned extensions.
	var visible []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() && (bannedDirs[name] || strings.HasPrefix(name, ".")) {
			continue
		}
		if !e.IsDir() && bannedExts[strings.ToLower(filepath.Ext(name))] {
			continue
		}
		visible = append(visible, e)
	}

	var b strings.Builder
	for i, e := range visible {
		connector := "├── "
		childPrefix := prefix + "│   "
		if i == len(visible)-1 {
			connector = "└── "
			childPrefix = prefix + "    "
		}

		label := e.Name()
		if e.IsDir() {
			label += "/"
		}
		b.WriteString(prefix + connector + label + "\n")

		if e.IsDir() {
			sub := c.buildTree(filepath.Join(dir, e.Name()), childPrefix, depth+1, maxDepth)
			b.WriteString(sub)
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Framework detection
// ---------------------------------------------------------------------------

func (c *Cartographer) detectFrameworks(root string, manifests []string) []string {
	seen := map[string]bool{}

	for _, rel := range manifests {
		name := filepath.Base(rel)
		abs := filepath.Join(root, rel)

		switch name {
		case "package.json":
			c.scanJSONKeys(abs, []frameworkSignal{
				{"react", "React"},
				{"react-dom", "React"},
				{"next", "Next.js"},
				{"nuxt", "Nuxt"},
				{"vue", "Vue.js"},
				{"@angular/core", "Angular"},
				{"svelte", "Svelte"},
				{"express", "Express.js"},
				{"fastify", "Fastify"},
				{"koa", "Koa"},
				{"hono", "Hono"},
				{"tailwindcss", "Tailwind CSS"},
				{"prisma", "Prisma ORM"},
				{"sequelize", "Sequelize ORM"},
				{"mongoose", "Mongoose ODM"},
				{"typescript", "TypeScript"},
				{"vite", "Vite"},
				{"webpack", "Webpack"},
				{"electron", "Electron"},
			}, seen)

		case "go.mod":
			c.scanLineContains(abs, []frameworkSignal{
				{"gin-gonic/gin", "Gin (Go)"},
				{"gorilla/mux", "Gorilla Mux (Go)"},
				{"labstack/echo", "Echo (Go)"},
				{"go-chi/chi", "Chi (Go)"},
				{"gofiber/fiber", "Fiber (Go)"},
				{"gorm.io/gorm", "GORM (Go)"},
				{"ent/ent", "Ent ORM (Go)"},
				{"grpc", "gRPC"},
			}, seen)

		case "requirements.txt", "Pipfile", "pyproject.toml", "setup.py", "setup.cfg":
			c.scanLineContains(abs, []frameworkSignal{
				{"django", "Django"},
				{"flask", "Flask"},
				{"fastapi", "FastAPI"},
				{"starlette", "Starlette"},
				{"tornado", "Tornado"},
				{"sqlalchemy", "SQLAlchemy"},
				{"celery", "Celery"},
				{"pydantic", "Pydantic"},
				{"boto3", "AWS SDK (Python)"},
			}, seen)

		case "Gemfile":
			c.scanLineContains(abs, []frameworkSignal{
				{"rails", "Ruby on Rails"},
				{"sinatra", "Sinatra"},
				{"devise", "Devise (Auth)"},
			}, seen)

		case "Cargo.toml":
			c.scanLineContains(abs, []frameworkSignal{
				{"actix-web", "Actix Web (Rust)"},
				{"axum", "Axum (Rust)"},
				{"rocket", "Rocket (Rust)"},
				{"tokio", "Tokio (Rust)"},
				{"diesel", "Diesel ORM (Rust)"},
				{"sqlx", "SQLx (Rust)"},
			}, seen)

		case "pom.xml", "build.gradle", "build.gradle.kts":
			c.scanLineContains(abs, []frameworkSignal{
				{"spring-boot", "Spring Boot"},
				{"spring-security", "Spring Security"},
				{"quarkus", "Quarkus"},
				{"micronaut", "Micronaut"},
				{"hibernate", "Hibernate ORM"},
			}, seen)

		case "composer.json":
			c.scanJSONKeys(abs, []frameworkSignal{
				{"laravel/framework", "Laravel"},
				{"symfony/", "Symfony"},
				{"slim/slim", "Slim (PHP)"},
			}, seen)
		}
	}

	// Infrastructure signals from file existence.
	infraFiles := map[string]string{
		"Dockerfile":         "Docker",
		"docker-compose.yml": "Docker Compose",
		"docker-compose.yaml": "Docker Compose",
		"Makefile":           "Make",
		"Jenkinsfile":        "Jenkins CI",
		".github":            "GitHub Actions",
		".gitlab-ci.yml":     "GitLab CI",
		"terraform":          "Terraform",
		"serverless.yml":     "Serverless Framework",
		"k8s":                "Kubernetes",
		"kubernetes":         "Kubernetes",
		"helm":               "Helm",
		"nginx.conf":         "Nginx",
		"Procfile":           "Heroku",
		".env":               "Environment Variables (.env)",
	}
	for file, label := range infraFiles {
		check := filepath.Join(root, file)
		if _, err := os.Stat(check); err == nil {
			seen[label] = true
		}
	}

	var result []string
	for fw := range seen {
		result = append(result, fw)
	}
	sort.Strings(result)
	return result
}

type frameworkSignal struct {
	needle string
	label  string
}

// scanJSONKeys does a quick substring scan of a JSON file for dependency keys.
// It intentionally avoids full JSON parsing for speed and resilience.
func (c *Cartographer) scanJSONKeys(path string, signals []frameworkSignal, seen map[string]bool) {
	data, err := readFileCapped(path, 256*1024)
	if err != nil {
		return
	}
	content := string(data)
	for _, sig := range signals {
		if strings.Contains(content, sig.needle) {
			seen[sig.label] = true
		}
	}
}

// scanLineContains scans a file line-by-line for substring matches.
func (c *Cartographer) scanLineContains(path string, signals []frameworkSignal, seen map[string]bool) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		for _, sig := range signals {
			if strings.Contains(line, sig.needle) {
				seen[sig.label] = true
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Dependency summaries
// ---------------------------------------------------------------------------

func (c *Cartographer) extractDependencySummaries(root string, manifests []string) []string {
	var summaries []string

	for _, rel := range manifests {
		name := filepath.Base(rel)
		abs := filepath.Join(root, rel)

		switch name {
		case "go.mod":
			if mods := c.extractGoModDeps(abs); mods != "" {
				summaries = append(summaries, fmt.Sprintf("**%s** — %s", rel, mods))
			}
		case "package.json":
			if deps := c.extractPackageJSONDeps(abs); deps != "" {
				summaries = append(summaries, fmt.Sprintf("**%s** — %s", rel, deps))
			}
		case "requirements.txt":
			if deps := c.extractLineList(abs, 20); deps != "" {
				summaries = append(summaries, fmt.Sprintf("**%s** — %s", rel, deps))
			}
		case "Gemfile":
			if deps := c.extractGemfileDeps(abs); deps != "" {
				summaries = append(summaries, fmt.Sprintf("**%s** — %s", rel, deps))
			}
		}
	}

	return summaries
}

func (c *Cartographer) extractGoModDeps(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var deps []string
	inRequire := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "require (") || strings.HasPrefix(line, "require(") {
			inRequire = true
			continue
		}
		if inRequire {
			if line == ")" {
				inRequire = false
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 1 && !strings.HasPrefix(parts[0], "//") {
				deps = append(deps, parts[0])
			}
		}
		// Single-line require.
		if strings.HasPrefix(line, "require ") && !strings.Contains(line, "(") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				deps = append(deps, parts[1])
			}
		}
	}
	if len(deps) == 0 {
		return ""
	}
	if len(deps) > 20 {
		return fmt.Sprintf("%s … and %d more", strings.Join(deps[:20], ", "), len(deps)-20)
	}
	return strings.Join(deps, ", ")
}

func (c *Cartographer) extractPackageJSONDeps(path string) string {
	data, err := readFileCapped(path, 128*1024)
	if err != nil {
		return ""
	}
	content := string(data)

	// Quick-and-dirty: extract keys from "dependencies" and "devDependencies"
	// blocks without a full JSON parse.
	var deps []string
	for _, section := range []string{"dependencies", "devDependencies"} {
		idx := strings.Index(content, `"`+section+`"`)
		if idx == -1 {
			continue
		}
		// Find the opening brace.
		braceStart := strings.Index(content[idx:], "{")
		if braceStart == -1 {
			continue
		}
		start := idx + braceStart + 1
		// Find the closing brace.
		braceEnd := strings.Index(content[start:], "}")
		if braceEnd == -1 {
			continue
		}
		block := content[start : start+braceEnd]
		// Extract quoted keys.
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, `"`) {
				end := strings.Index(line[1:], `"`)
				if end > 0 {
					deps = append(deps, line[1:1+end])
				}
			}
		}
	}

	if len(deps) == 0 {
		return ""
	}
	if len(deps) > 25 {
		return fmt.Sprintf("%s … and %d more", strings.Join(deps[:25], ", "), len(deps)-25)
	}
	return strings.Join(deps, ", ")
}

func (c *Cartographer) extractLineList(path string, max int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var items []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip version specifiers.
		for _, sep := range []string{"==", ">=", "<=", "~=", "!="} {
			if idx := strings.Index(line, sep); idx > 0 {
				line = line[:idx]
			}
		}
		items = append(items, strings.TrimSpace(line))
		if len(items) >= max {
			break
		}
	}
	if len(items) == 0 {
		return ""
	}
	return strings.Join(items, ", ")
}

func (c *Cartographer) extractGemfileDeps(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var gems []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "gem ") || strings.HasPrefix(line, "gem(") {
			// gem 'name' or gem "name"
			for _, q := range []string{"'", `"`} {
				idx := strings.Index(line, q)
				if idx == -1 {
					continue
				}
				end := strings.Index(line[idx+1:], q)
				if end > 0 {
					gems = append(gems, line[idx+1:idx+1+end])
					break
				}
			}
		}
	}
	if len(gems) == 0 {
		return ""
	}
	if len(gems) > 20 {
		return fmt.Sprintf("%s … and %d more", strings.Join(gems[:20], ", "), len(gems)-20)
	}
	return strings.Join(gems, ", ")
}

// ---------------------------------------------------------------------------
// Markdown renderer
// ---------------------------------------------------------------------------

func (c *Cartographer) renderMarkdown(snap snapshot) string {
	var b strings.Builder

	b.WriteString("# 🗺️ Cartographer — Workspace Architecture Summary\n\n")

	// --- Stats ---
	b.WriteString(fmt.Sprintf("**Files:** %d | **Directories:** %d\n\n", snap.TotalFiles, snap.TotalDirs))

	// --- Languages ---
	if len(snap.Languages) > 0 {
		b.WriteString("## Languages\n")
		type langCount struct {
			lang  string
			count int
		}
		var sorted []langCount
		for l, c := range snap.Languages {
			sorted = append(sorted, langCount{l, c})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
		for _, lc := range sorted {
			b.WriteString(fmt.Sprintf("- **%s**: %d files\n", lc.lang, lc.count))
		}
		b.WriteString("\n")
	}

	// --- Frameworks & Tooling ---
	if len(snap.Frameworks) > 0 {
		b.WriteString("## Frameworks & Tooling\n")
		for _, fw := range snap.Frameworks {
			b.WriteString(fmt.Sprintf("- %s\n", fw))
		}
		b.WriteString("\n")
	}

	// --- Top-level structure ---
	if len(snap.TopDirs) > 0 {
		b.WriteString("## Top-Level Structure\n")
		for _, d := range snap.TopDirs {
			b.WriteString(fmt.Sprintf("- `%s`\n", d))
		}
		b.WriteString("\n")
	}

	// --- Directory tree ---
	if snap.Tree != "" {
		b.WriteString("## Directory Tree (depth 3)\n")
		b.WriteString("```\n")
		b.WriteString(snap.Tree)
		b.WriteString("```\n\n")
	}

	// --- Manifests ---
	if len(snap.Manifests) > 0 {
		b.WriteString("## Manifest / Config Files\n")
		for _, m := range snap.Manifests {
			b.WriteString(fmt.Sprintf("- `%s`\n", m))
		}
		b.WriteString("\n")
	}

	// --- Dependency summaries ---
	if len(snap.DependencySummaries) > 0 {
		b.WriteString("## Key Dependencies\n")
		for _, ds := range snap.DependencySummaries {
			b.WriteString(fmt.Sprintf("- %s\n", ds))
		}
		b.WriteString("\n")
	}

	// --- Entry points ---
	if len(snap.EntryPoints) > 0 {
		b.WriteString("## Entry Points\n")
		for _, ep := range snap.EntryPoints {
			b.WriteString(fmt.Sprintf("- `%s`\n", ep))
		}
		b.WriteString("\n")
	}

	// --- Security-relevant files ---
	if len(snap.SecurityFiles) > 0 {
		b.WriteString("## Security-Relevant Files (investigate first)\n")
		for _, sf := range snap.SecurityFiles {
			b.WriteString(fmt.Sprintf("- `%s`\n", sf))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Classification tables
// ---------------------------------------------------------------------------

// extToLang maps file extensions to human-readable language names.
var extToLang = map[string]string{
	".go":    "Go",
	".js":    "JavaScript",
	".jsx":   "JavaScript (JSX)",
	".ts":    "TypeScript",
	".tsx":   "TypeScript (TSX)",
	".py":    "Python",
	".rb":    "Ruby",
	".rs":    "Rust",
	".java":  "Java",
	".kt":    "Kotlin",
	".scala": "Scala",
	".cs":    "C#",
	".c":     "C",
	".cpp":   "C++",
	".h":     "C/C++ Header",
	".hpp":   "C++ Header",
	".php":   "PHP",
	".swift": "Swift",
	".m":     "Objective-C",
	".lua":   "Lua",
	".r":     "R",
	".pl":    "Perl",
	".sh":    "Shell",
	".bash":  "Shell",
	".zsh":   "Shell",
	".ps1":   "PowerShell",
	".sql":   "SQL",
	".html":  "HTML",
	".htm":   "HTML",
	".css":   "CSS",
	".scss":  "SCSS",
	".less":  "LESS",
	".svelte": "Svelte",
	".vue":   "Vue",
	".yaml":  "YAML",
	".yml":   "YAML",
	".json":  "JSON",
	".toml":  "TOML",
	".xml":   "XML",
	".md":    "Markdown",
	".proto": "Protocol Buffers",
	".graphql": "GraphQL",
	".gql":    "GraphQL",
	".tf":     "Terraform (HCL)",
	".hcl":    "HCL",
	".sol":    "Solidity",
	".zig":    "Zig",
	".ex":     "Elixir",
	".exs":    "Elixir",
	".erl":    "Erlang",
	".dart":   "Dart",
}

// manifestNames are filenames that indicate project manifests or configs.
var manifestNames = map[string]bool{
	"package.json":        true,
	"package-lock.json":   true,
	"yarn.lock":           true,
	"pnpm-lock.yaml":      true,
	"go.mod":              true,
	"go.sum":              true,
	"Cargo.toml":          true,
	"Cargo.lock":          true,
	"requirements.txt":    true,
	"Pipfile":             true,
	"Pipfile.lock":        true,
	"pyproject.toml":      true,
	"setup.py":            true,
	"setup.cfg":           true,
	"Gemfile":             true,
	"Gemfile.lock":        true,
	"composer.json":       true,
	"composer.lock":       true,
	"pom.xml":             true,
	"build.gradle":        true,
	"build.gradle.kts":    true,
	"settings.gradle":     true,
	"settings.gradle.kts": true,
	"Makefile":            true,
	"CMakeLists.txt":      true,
	"Dockerfile":          true,
	"docker-compose.yml":  true,
	"docker-compose.yaml": true,
	".dockerignore":       true,
	".env":                true,
	".env.example":        true,
	".env.local":          true,
	".env.production":     true,
	"tsconfig.json":       true,
	"vite.config.ts":      true,
	"vite.config.js":      true,
	"webpack.config.js":   true,
	"next.config.js":      true,
	"next.config.mjs":     true,
	"svelte.config.js":    true,
	"tailwind.config.js":  true,
	"tailwind.config.ts":  true,
	"nginx.conf":          true,
	"Procfile":            true,
	"serverless.yml":      true,
	"serverless.yaml":     true,
	"Jenkinsfile":         true,
	".gitlab-ci.yml":      true,
}

func isManifest(name string) bool {
	return manifestNames[name]
}

// entryPointNames are filenames commonly used as application entry points.
var entryPointNames = map[string]bool{
	"main.go":       true,
	"main.py":       true,
	"app.py":        true,
	"manage.py":     true,
	"wsgi.py":       true,
	"asgi.py":       true,
	"index.js":      true,
	"index.ts":      true,
	"server.js":     true,
	"server.ts":     true,
	"app.js":        true,
	"app.ts":        true,
	"main.rs":       true,
	"lib.rs":        true,
	"main.java":     true,
	"Application.java": true,
	"main.kt":       true,
	"main.dart":     true,
	"main.swift":    true,
	"index.php":     true,
	"config.ru":     true,
}

func isEntryPoint(name string) bool {
	return entryPointNames[name]
}

// isSecurityRelevant flags files that a security agent should inspect first.
func isSecurityRelevant(name, relPath string) bool {
	lower := strings.ToLower(name)
	lowerPath := strings.ToLower(relPath)

	// Exact names.
	secNames := map[string]bool{
		".env": true, ".env.local": true, ".env.production": true,
		".env.example": true, ".htaccess": true, ".htpasswd": true,
		"secrets.yml": true, "secrets.yaml": true, "credentials.yml": true,
		"credentials.yaml": true, "id_rsa": true, "id_rsa.pub": true,
		"id_ed25519": true, "id_ed25519.pub": true,
		"authorized_keys": true, "known_hosts": true,
		"shadow": true, "passwd": true,
	}
	if secNames[lower] {
		return true
	}

	// Path-segment keywords.
	secKeywords := []string{
		"auth", "login", "session", "token", "jwt",
		"oauth", "password", "credential", "secret",
		"crypto", "encrypt", "decrypt", "cert",
		"middleware", "permission", "rbac", "acl",
		"security", "sanitize", "validate", "csrf",
		"cors", "helmet", "rate-limit", "ratelimit",
	}
	for _, kw := range secKeywords {
		if strings.Contains(lowerPath, kw) {
			return true
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

// readFileCapped reads up to maxBytes bytes from a file.
func readFileCapped(path string, maxBytes int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size > maxBytes {
		size = maxBytes
	}
	buf := make([]byte, size)
	n, err := f.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}
