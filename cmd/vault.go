package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Yoshiofthewire/bashcorrect/config"
	"github.com/spf13/cobra"
)

var (
	vaultPathFlag    string
	vaultForce       bool
	vaultNoBootstrap bool
	vaultTitle       string
	vaultID          string
	vaultType        string
)

var vaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Manage a local memory vault",
	Long: `Creates and manages a deterministic local memory vault inspired by wiki-style
knowledge systems. The vault stores structured pages (entities, concepts,
syntheses, sources, reports) plus cache artifacts for tooling.`,
}

var vaultInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the memory vault layout",
	RunE:  runVaultInit,
}

var vaultStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show memory vault status",
	RunE:  runVaultStatus,
}

var vaultBootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Run vault bootstrap tasks",
	RunE:  runVaultBootstrapCmd,
}

var vaultNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new vault page",
	Long:  "Create a page in entities, concepts, syntheses, sources, or reports.",
	RunE:  runVaultNew,
}

var vaultGetCmd = &cobra.Command{
	Use:   "get <id-or-path>",
	Short: "Print a vault page by id or path",
	Args:  cobra.ExactArgs(1),
	RunE:  runVaultGet,
}

var vaultSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search markdown pages inside the vault",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runVaultSearch,
}

var vaultTypes = map[string]string{
	"entity":    "entities",
	"concept":   "concepts",
	"synthesis": "syntheses",
	"source":    "sources",
	"report":    "reports",
}

func init() {
	vaultCmd.PersistentFlags().StringVar(&vaultPathFlag, "path", "", "vault path (default: $XDG_CONFIG_HOME/bashcorrect/vault)")

	vaultInitCmd.Flags().BoolVar(&vaultForce, "force", false, "overwrite top-level template files")
	vaultInitCmd.Flags().BoolVar(&vaultNoBootstrap, "no-bootstrap", false, "skip running BOOTSTRAP.md automation after init")

	vaultNewCmd.Flags().StringVar(&vaultTitle, "title", "", "page title (required)")
	vaultNewCmd.Flags().StringVar(&vaultID, "id", "", "stable page id (default: <type>.<slug>)")
	vaultNewCmd.Flags().StringVar(&vaultType, "type", "entity", "page type: entity|concept|synthesis|source|report")
	_ = vaultNewCmd.MarkFlagRequired("title")

	vaultMemoryCmd.AddCommand(vaultMemorySetCmd)
	vaultMemoryCmd.AddCommand(vaultMemoryNoteCmd)
	vaultMemoryCmd.AddCommand(vaultMemoryGetCmd)
	vaultMemoryCmd.AddCommand(vaultMemorySearchCmd)
	vaultMemoryCmd.AddCommand(vaultMemoryListCmd)

	vaultCmd.AddCommand(vaultInitCmd)
	vaultCmd.AddCommand(vaultStatusCmd)
	vaultCmd.AddCommand(vaultBootstrapCmd)
	vaultCmd.AddCommand(vaultNewCmd)
	vaultCmd.AddCommand(vaultGetCmd)
	vaultCmd.AddCommand(vaultSearchCmd)
	vaultCmd.AddCommand(vaultMemoryCmd)
	rootCmd.AddCommand(vaultCmd)
}

func runVaultInit(_ *cobra.Command, _ []string) error {
	vaultPath, err := resolveVaultPath(vaultPathFlag)
	if err != nil {
		return err
	}
	return initVaultAtPath(vaultPath, vaultForce, !vaultNoBootstrap)
}

func initVaultAtPath(vaultPath string, force bool, bootstrap bool) error {
	if err := os.MkdirAll(vaultPath, 0o700); err != nil {
		return fmt.Errorf("creating vault: %w", err)
	}

	dirs := []string{
		"entities",
		"concepts",
		"syntheses",
		"sources",
		"reports",
		"_attachments",
		"_views",
		".bashcorrect-vault/cache",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(vaultPath, dir), 0o700); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	if err := ensureMemoryDB(vaultPath); err != nil {
		return err
	}

	files := map[string]string{
		"AGENTS.md":    vaultAgentsTemplate(),
		"WIKI.md":      vaultWikiTemplate(),
		"index.md":     vaultIndexTemplate(),
		"inbox.md":     "# Inbox\n\nCapture raw notes here before promoting into structured pages.\n",
		"SOUL.md":      vaultSoulTemplate(),
		"TOOLS.md":     vaultToolsTemplate(),
		"BOOTSTRAP.md": vaultBootstrapTemplate(),
		".bashcorrect-vault/cache/agent-digest.json": "[]\n",
		".bashcorrect-vault/cache/claims.jsonl":      "",
	}

	for relPath, content := range files {
		target := filepath.Join(vaultPath, relPath)
		if err := writeTemplateFile(target, content, force); err != nil {
			return err
		}
	}

	bootstrapRan := false
	if bootstrap {
		if err := runVaultBootstrap(vaultPath); err != nil {
			return err
		}
		bootstrapRan = true
	}

	fmt.Printf("Vault initialized at %s\n", vaultPath)
	if bootstrapRan {
		fmt.Println("Bootstrap completed")
	} else {
		fmt.Println("Bootstrap skipped")
	}
	return nil
}
func runVaultStatus(_ *cobra.Command, _ []string) error {
	vaultPath, err := resolveVaultPath(vaultPathFlag)
	if err != nil {
		return err
	}

	if _, err := os.Stat(vaultPath); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Vault path: %s\nStatus: not initialized\n", vaultPath)
		return nil
	}

	counts, total, err := countVaultPages(vaultPath)
	if err != nil {
		return err
	}

	fmt.Printf("Vault path: %s\n", vaultPath)
	fmt.Println("Status: initialized")
	fmt.Printf("Total pages: %d\n", total)
	sections := []string{"entities", "concepts", "syntheses", "sources", "reports"}
	for _, s := range sections {
		fmt.Printf("- %s: %d\n", s, counts[s])
	}

	digest := filepath.Join(vaultPath, ".bashcorrect-vault", "cache", "agent-digest.json")
	if _, err := os.Stat(digest); err == nil {
		fmt.Println("Digest: present")
	} else {
		fmt.Println("Digest: missing")
	}

	return nil
}

func runVaultNew(_ *cobra.Command, _ []string) error {
	vaultPath, err := resolveVaultPath(vaultPathFlag)
	if err != nil {
		return err
	}

	folder, ok := vaultTypes[strings.ToLower(strings.TrimSpace(vaultType))]
	if !ok {
		return fmt.Errorf("invalid --type %q (use: entity, concept, synthesis, source, report)", vaultType)
	}

	title := strings.TrimSpace(vaultTitle)
	if title == "" {
		return fmt.Errorf("--title is required")
	}

	slug := slugify(title)
	if slug == "" {
		return fmt.Errorf("title produced an empty slug")
	}

	pageID := strings.TrimSpace(vaultID)
	if pageID == "" {
		pageID = strings.ToLower(strings.TrimSpace(vaultType)) + "." + slug
	}

	filePath := filepath.Join(vaultPath, folder, slug+".md")
	if _, err := os.Stat(filePath); err == nil {
		return fmt.Errorf("page already exists: %s", filePath)
	}

	content := buildVaultPage(strings.ToLower(strings.TrimSpace(vaultType)), pageID, title, time.Now().UTC())
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("writing page: %w", err)
	}

	fmt.Println(filePath)
	return nil
}

func runVaultGet(_ *cobra.Command, args []string) error {
	vaultPath, err := resolveVaultPath(vaultPathFlag)
	if err != nil {
		return err
	}

	target, err := resolveVaultTarget(vaultPath, args[0])
	if err != nil {
		return err
	}

	data, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("reading page: %w", err)
	}
	fmt.Print(string(data))
	return nil
}

func runVaultSearch(_ *cobra.Command, args []string) error {
	vaultPath, err := resolveVaultPath(vaultPathFlag)
	if err != nil {
		return err
	}

	query := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
	if query == "" {
		return fmt.Errorf("query cannot be empty")
	}

	files, err := listVaultMarkdownFiles(vaultPath)
	if err != nil {
		return err
	}

	matches := 0
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if strings.Contains(strings.ToLower(line), query) {
				rel, relErr := filepath.Rel(vaultPath, path)
				if relErr != nil {
					rel = path
				}
				fmt.Printf("%s:%d: %s\n", rel, lineNo, strings.TrimSpace(line))
				matches++
				break
			}
		}
		_ = f.Close()
	}

	if matches == 0 {
		fmt.Println("No matches.")
	}

	return nil
}

func resolveVaultPath(flagPath string) (string, error) {
	if strings.TrimSpace(flagPath) != "" {
		return filepath.Clean(flagPath), nil
	}
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "vault"), nil
}

func writeTemplateFile(path, content string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func countVaultPages(vaultPath string) (map[string]int, int, error) {
	counts := map[string]int{
		"entities":  0,
		"concepts":  0,
		"syntheses": 0,
		"sources":   0,
		"reports":   0,
	}
	total := 0

	for section := range counts {
		sectionPath := filepath.Join(vaultPath, section)
		files, err := os.ReadDir(sectionPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, 0, fmt.Errorf("reading %s: %w", sectionPath, err)
		}
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(strings.ToLower(file.Name()), ".md") {
				counts[section]++
				total++
			}
		}
	}

	return counts, total, nil
}

func listVaultMarkdownFiles(vaultPath string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(vaultPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".bashcorrect-vault" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking vault: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

func resolveVaultTarget(vaultPath, in string) (string, error) {
	candidate := strings.TrimSpace(in)
	if candidate == "" {
		return "", fmt.Errorf("target cannot be empty")
	}

	if filepath.IsAbs(candidate) {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	joined := filepath.Join(vaultPath, candidate)
	if _, err := os.Stat(joined); err == nil {
		return joined, nil
	}

	if !strings.HasSuffix(strings.ToLower(joined), ".md") {
		withMD := joined + ".md"
		if _, err := os.Stat(withMD); err == nil {
			return withMD, nil
		}
	}

	files, err := listVaultMarkdownFiles(vaultPath)
	if err != nil {
		return "", err
	}
	needle := "id: " + candidate
	for _, path := range files {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		if strings.Contains(string(data), needle) {
			return path, nil
		}
	}

	return "", fmt.Errorf("no page found for %q", in)
}

func buildVaultPage(pageType, id, title string, now time.Time) string {
	updatedAt := now.Format(time.RFC3339)
	return fmt.Sprintf(`---
pageType: %s
id: %s
title: %s
updatedAt: %s
confidence: 0.5
status: draft
claims: []
openQuestions: []
---

# %s

## Summary

TBD.

## Evidence

- source: TBD
- notes: TBD

`, pageType, id, title, updatedAt, title)
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}

	var b strings.Builder
	prevDash := false
	for _, r := range s {
		isLetter := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		if isLetter || isDigit {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteRune('-')
			prevDash = true
		}
	}

	out := strings.Trim(b.String(), "-")
	return out
}

func vaultAgentsTemplate() string {
	return `# AGENTS

This vault stores durable memory pages for BashCorrect workflows.
The bot is always allowed to write to vault memory files.

Guidelines:
- Keep factual claims tied to evidence.
- Prefer structured updates over ad-hoc rewrites.
- Review low-confidence claims regularly.
`
}

func vaultWikiTemplate() string {
	return `# WIKI

This vault follows a deterministic layout inspired by memory-wiki:

- entities/: durable people/systems/projects
- concepts/: patterns and policies
- syntheses/: maintained summaries and rollups
- sources/: raw imported references
- reports/: generated dashboards or audits

Use bashcorrect vault new to create structured pages.
`
}

func vaultIndexTemplate() string {
	return `# Memory Vault Index

- [Inbox](inbox.md)
- [Entities](entities)
- [Concepts](concepts)
- [Syntheses](syntheses)
- [Sources](sources)
- [Reports](reports)
`
}
