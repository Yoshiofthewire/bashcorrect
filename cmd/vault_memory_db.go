package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

const memoryDBRelativePath = ".bashcorrect-vault/cache/memory.db"

func memoryDBPath(vaultPath string) string {
	return filepath.Join(vaultPath, memoryDBRelativePath)
}

func ensureMemoryDB(vaultPath string) error {
	db, err := sql.Open("sqlite", memoryDBPath(vaultPath))
	if err != nil {
		return fmt.Errorf("opening memory db: %w", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS memory_kv (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS memory_notes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			category TEXT NOT NULL,
			content TEXT NOT NULL,
			source TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_notes_category_created ON memory_notes(category, created_at DESC);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("initializing memory db: %w", err)
		}
	}
	return nil
}

func upsertMemoryKV(vaultPath, key, value string) error {
	if err := ensureMemoryDB(vaultPath); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", memoryDBPath(vaultPath))
	if err != nil {
		return fmt.Errorf("opening memory db: %w", err)
	}
	defer db.Close()

	stamp := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO memory_kv (key, value, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, key, value, stamp)
	if err != nil {
		return fmt.Errorf("upserting memory kv: %w", err)
	}
	return nil
}

func appendMemoryNote(vaultPath, category, content, source string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	if err := ensureMemoryDB(vaultPath); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", memoryDBPath(vaultPath))
	if err != nil {
		return fmt.Errorf("opening memory db: %w", err)
	}
	defer db.Close()

	stamp := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO memory_notes (category, content, source, created_at) VALUES (?, ?, ?, ?)`, category, content, source, stamp)
	if err != nil {
		return fmt.Errorf("inserting memory note: %w", err)
	}
	return nil
}

func readLargeMemoryContext(vaultPath string, maxKV, maxNotes int) (string, error) {
	if err := ensureMemoryDB(vaultPath); err != nil {
		return "", err
	}
	db, err := sql.Open("sqlite", memoryDBPath(vaultPath))
	if err != nil {
		return "", fmt.Errorf("opening memory db: %w", err)
	}
	defer db.Close()

	var b strings.Builder
	b.WriteString("KV entries:\n")
	rows, err := db.Query(`SELECT key, value FROM memory_kv ORDER BY key ASC LIMIT ?`, maxKV)
	if err != nil {
		return "", fmt.Errorf("querying memory kv: %w", err)
	}
	defer rows.Close()

	hasKV := false
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return "", fmt.Errorf("scanning memory kv: %w", err)
		}
		hasKV = true
		b.WriteString("- ")
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(value)
		b.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("reading memory kv rows: %w", err)
	}
	if !hasKV {
		b.WriteString("- (none)\n")
	}

	b.WriteString("\nRecent notes:\n")
	noteRows, err := db.Query(`SELECT category, content, source, created_at FROM memory_notes ORDER BY id DESC LIMIT ?`, maxNotes)
	if err != nil {
		return "", fmt.Errorf("querying memory notes: %w", err)
	}
	defer noteRows.Close()

	hasNotes := false
	for noteRows.Next() {
		var category, content, source, createdAt string
		if err := noteRows.Scan(&category, &content, &source, &createdAt); err != nil {
			return "", fmt.Errorf("scanning memory note: %w", err)
		}
		hasNotes = true
		b.WriteString("- [")
		b.WriteString(category)
		b.WriteString("] ")
		b.WriteString(content)
		if source != "" {
			b.WriteString(" (source: ")
			b.WriteString(source)
			b.WriteString(")")
		}
		if createdAt != "" {
			b.WriteString(" @")
			b.WriteString(createdAt)
		}
		b.WriteString("\n")
	}
	if err := noteRows.Err(); err != nil {
		return "", fmt.Errorf("reading memory note rows: %w", err)
	}
	if !hasNotes {
		b.WriteString("- (none)\n")
	}

	return strings.TrimSpace(b.String()), nil
}

// searchMemoryNotes returns notes whose content contains the query string (case-insensitive).
func searchMemoryNotes(vaultPath, query string, limit int) ([]struct{ Category, Content, Source, CreatedAt string }, error) {
	if err := ensureMemoryDB(vaultPath); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", memoryDBPath(vaultPath))
	if err != nil {
		return nil, fmt.Errorf("opening memory db: %w", err)
	}
	defer db.Close()

	pattern := "%" + strings.ReplaceAll(query, "%", "\\%") + "%"
	rows, err := db.Query(`SELECT category, content, source, created_at FROM memory_notes WHERE content LIKE ? ORDER BY id DESC LIMIT ?`, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("searching memory notes: %w", err)
	}
	defer rows.Close()

	var results []struct{ Category, Content, Source, CreatedAt string }
	for rows.Next() {
		var r struct{ Category, Content, Source, CreatedAt string }
		if err := rows.Scan(&r.Category, &r.Content, &r.Source, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning memory note: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// getMemoryKV returns the value for a single key, or ("", false, nil) if missing.
func getMemoryKV(vaultPath, key string) (string, bool, error) {
	if err := ensureMemoryDB(vaultPath); err != nil {
		return "", false, err
	}
	db, err := sql.Open("sqlite", memoryDBPath(vaultPath))
	if err != nil {
		return "", false, fmt.Errorf("opening memory db: %w", err)
	}
	defer db.Close()

	var value string
	err = db.QueryRow(`SELECT value FROM memory_kv WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reading memory kv: %w", err)
	}
	return value, true, nil
}

// ── vault memory CLI commands ──────────────────────────────────────────────

var vaultMemoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage the SQLite memory database",
	Long: `Read and write structured memory in the local SQLite database.
These commands can be used by the user or included in AI responses so
that important facts, preferences, and notes are persisted between sessions.`,
}

var vaultMemorySetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Store or update a key-value fact",
	Args:  cobra.ExactArgs(2),
	RunE:  runVaultMemorySet,
}

var vaultMemoryNoteCmd = &cobra.Command{
	Use:   "note <category> <text>",
	Short: "Append a timestamped note under a category",
	Args:  cobra.ExactArgs(2),
	RunE:  runVaultMemoryNote,
}

var vaultMemoryGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Print the value stored for a key",
	Args:  cobra.ExactArgs(1),
	RunE:  runVaultMemoryGet,
}

var vaultMemorySearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search notes whose content contains the query",
	Args:  cobra.ExactArgs(1),
	RunE:  runVaultMemorySearch,
}

var vaultMemoryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all key-value facts and recent notes",
	RunE:  runVaultMemoryList,
}

func runVaultMemorySet(_ *cobra.Command, args []string) error {
	vaultPath, err := resolveVaultPath(vaultPathFlag)
	if err != nil {
		return err
	}
	if err := upsertMemoryKV(vaultPath, args[0], args[1]); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "memory: set %q\n", args[0])
	return nil
}

func runVaultMemoryNote(_ *cobra.Command, args []string) error {
	vaultPath, err := resolveVaultPath(vaultPathFlag)
	if err != nil {
		return err
	}
	if err := appendMemoryNote(vaultPath, args[0], args[1], "user"); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "memory: appended note in category %q\n", args[0])
	return nil
}

func runVaultMemoryGet(_ *cobra.Command, args []string) error {
	vaultPath, err := resolveVaultPath(vaultPathFlag)
	if err != nil {
		return err
	}
	value, found, err := getMemoryKV(vaultPath, args[0])
	if err != nil {
		return err
	}
	if !found {
		fmt.Fprintf(os.Stderr, "memory: key %q not found\n", args[0])
		return nil
	}
	fmt.Println(value)
	return nil
}

func runVaultMemorySearch(_ *cobra.Command, args []string) error {
	vaultPath, err := resolveVaultPath(vaultPathFlag)
	if err != nil {
		return err
	}
	results, err := searchMemoryNotes(vaultPath, args[0], 20)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Println("(no results)")
		return nil
	}
	for _, r := range results {
		line := fmt.Sprintf("[%s] %s", r.Category, r.Content)
		if r.Source != "" {
			line += fmt.Sprintf(" (source: %s)", r.Source)
		}
		if r.CreatedAt != "" {
			line += fmt.Sprintf(" @%s", r.CreatedAt)
		}
		fmt.Println(line)
	}
	return nil
}

func runVaultMemoryList(_ *cobra.Command, _ []string) error {
	vaultPath, err := resolveVaultPath(vaultPathFlag)
	if err != nil {
		return err
	}
	result, err := readLargeMemoryContext(vaultPath, 100, 50)
	if err != nil {
		return err
	}
	fmt.Println(result)
	return nil
}
