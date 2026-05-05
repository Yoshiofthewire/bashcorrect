package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Yoshiofthewire/bashcorrect/providers"
	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query <prompt>",
	Short: "Ask the AI assistant a question or request a shell command",
	Long: `Sends a natural-language prompt to the active AI provider and prints the
response. When --inline is set, only the extracted command is printed (for use
by shell keybindings that replace the current input buffer).

You can also provide file content context with --file (repeatable), and use
--summarize-files to request a summary of those files without a positional
prompt.

Examples:
  bashcorrect query "explain this error"
  bashcorrect query "review for risks" --file cmd/query.go --file README.md
  bashcorrect query --summarize-files --file README.md`,
	Args: validateQueryArgs,
	RunE: runQuery,
}

var (
	queryInline       bool
	queryCWD          bool
	queryHistoryLines int
	queryVaultPath    string
	queryFiles        []string
	querySummarize    bool
)

func init() {
	queryCmd.Flags().BoolVar(&queryInline, "inline", false, "output only the extracted command (for keybinding buffer replacement)")
	queryCmd.Flags().BoolVar(&queryCWD, "cwd", false, "include current working directory in context")
	queryCmd.Flags().IntVar(&queryHistoryLines, "history-lines", 0, "include last N lines of shell history as context")
	queryCmd.Flags().StringVar(&queryVaultPath, "vault-path", "", "vault path (default: $XDG_CONFIG_HOME/bashcorrect/vault)")
	queryCmd.Flags().StringArrayVar(&queryFiles, "file", nil, "file path to include in prompt context (repeatable)")
	queryCmd.Flags().BoolVar(&querySummarize, "summarize-files", false, "summarize the provided --file inputs")
	rootCmd.AddCommand(queryCmd)
}

func validateQueryArgs(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		return nil
	}
	if querySummarize && len(queryFiles) > 0 {
		return nil
	}
	return fmt.Errorf("requires a prompt, or use --summarize-files with at least one --file")
}

const querySystemPrompt = `You are an expert terminal assistant.
By default, return only the final result in concise plain language.
Do not include internal reasoning, step-by-step thought process, or hidden analysis.
Do not include shell commands unless the user explicitly asks for commands or asks for verbose output.
When commands are explicitly requested, prefer POSIX-compatible commands unless the user specifies a shell.`

const queryInlineSystemPrompt = `You are an expert terminal assistant. The user wants a single shell command.
Output ONLY the raw command — no explanation, no markdown, no code fences.`

// memoryToolsPrompt is injected into the system prompt when the vault is available.
// It tells the AI how to persist and retrieve facts using the SQLite memory DB.
const memoryToolsPrompt = `## Memory database
You have access to a local SQLite memory database via these shell commands. Use them to remember important information or look things up:

  Store a fact (key-value):
    bashcorrect vault memory set "<key>" "<value>"

  Append a timestamped note:
    bashcorrect vault memory note "<category>" "<text>"

  Read a specific fact:
    bashcorrect vault memory get "<key>"

  Search notes by keyword:
    bashcorrect vault memory search "<keyword>"

  List all memory:
    bashcorrect vault memory list

When you learn something worth remembering — user preferences, tool choices, project paths, workflow shortcuts, recurring commands — include the appropriate ` + "`bashcorrect vault memory`" + ` command in a fenced code block so the user can run it to persist that fact. When you need a fact that may already be stored, suggest running the get or search command first.`

func runQuery(_ *cobra.Command, args []string) error {
	prompt := strings.TrimSpace(strings.Join(args, " "))
	if prompt == "" && querySummarize {
		prompt = "Summarize the provided files. Include purpose, key points, and risks or action items."
	}

	p, err := providers.NewActive(cfg)
	if err != nil {
		return err
	}

	sysPrompt, userPrompt := buildQueryPrompts(prompt, queryCWD, queryHistoryLines, queryInline, queryFiles)

	if !queryInline {
		fmt.Fprintf(os.Stderr, "\033[2m[bashcorrect] asking %s...\033[0m\n", p.Name())
	}

	response, err := p.Query(context.Background(), sysPrompt, userPrompt)
	if err != nil {
		return fmt.Errorf("provider error: %w", err)
	}
	response = strings.TrimSpace(response)

	if queryInline {
		fmt.Println(extractCommand(response))
		return nil
	}

	fmt.Println(response)

	// If the response contains a fenced code block, offer to run it
	if cmd := extractCommand(response); cmd != "" && cmd != response {
		answer := promptUser("\nRun this command? [y/N] ")
		if strings.ToLower(strings.TrimSpace(answer)) == "y" {
			fmt.Println(cmd)
		}
	}
	return nil
}

func buildQueryPrompts(prompt string, includeCWD bool, historyLines int, inline bool, files []string) (string, string) {
	sysPrompt := querySystemPrompt
	if inline {
		sysPrompt = queryInlineSystemPrompt
	}

	userPrompt := buildQueryPrompt(prompt, includeCWD, historyLines, files)
	vaultSystem, vaultUser := buildVaultQueryContext(prompt, queryVaultPath)
	if vaultSystem != "" {
		sysPrompt += "\n\n" + vaultSystem
	}
	if vaultUser != "" {
		userPrompt = vaultUser + "\n\n" + userPrompt
	}
	return sysPrompt, userPrompt
}

func buildQueryPrompt(prompt string, includeCWD bool, historyLines int, files []string) string {
	var sb strings.Builder

	if osRelease := buildOSReleasePromptContext(); osRelease != "" {
		sb.WriteString(osRelease)
	}

	if includeCWD {
		cwd, err := os.Getwd()
		if err == nil {
			sb.WriteString(fmt.Sprintf("Current directory: %s\n", cwd))
		}
	}

	if historyLines > 0 {
		history := readShellHistory(historyLines)
		if history != "" {
			sb.WriteString(fmt.Sprintf("Recent shell history:\n%s\n", history))
		}
	}

	if len(files) > 0 {
		fileContext := buildFileContext(files)
		if fileContext != "" {
			sb.WriteString("Files provided by user:\n")
			sb.WriteString(fileContext)
			sb.WriteString("\n")
		}
	}

	sb.WriteString(prompt)
	return sb.String()
}

func buildFileContext(files []string) string {
	const perFileLimit = 8000
	const totalLimit = 24000

	cwd, _ := os.Getwd()
	var sb strings.Builder
	remaining := totalLimit

	for _, rawPath := range files {
		if remaining <= 0 {
			break
		}
		path := strings.TrimSpace(rawPath)
		if path == "" {
			continue
		}
		resolved := path
		if !filepath.IsAbs(resolved) && cwd != "" {
			resolved = filepath.Join(cwd, resolved)
		}
		resolved = filepath.Clean(resolved)

		data, err := os.ReadFile(resolved)
		if err != nil {
			sb.WriteString(fmt.Sprintf("- %s (error: %v)\n", path, err))
			continue
		}
		if !utf8.Valid(data) {
			sb.WriteString(fmt.Sprintf("- %s (skipped: not valid UTF-8 text)\n", path))
			continue
		}

		content := string(data)
		truncated := false
		if len(content) > perFileLimit {
			content = content[:perFileLimit]
			truncated = true
		}
		if len(content) > remaining {
			content = content[:remaining]
			truncated = true
		}

		sb.WriteString(fmt.Sprintf("\n### %s\n", path))
		sb.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			sb.WriteString("\n")
		}
		if truncated {
			sb.WriteString("[truncated]\n")
		}

		remaining -= len(content)
	}

	return strings.TrimSpace(sb.String())
}

func buildVaultQueryContext(prompt, overridePath string) (string, string) {
	vaultPath, err := resolveVaultPath(overridePath)
	if err != nil {
		return "", ""
	}
	if _, err := os.Stat(vaultPath); err != nil {
		return "", ""
	}

	identity := readVaultFileForPrompt(vaultPath, "IDENTITY.md")
	user := readVaultFileForPrompt(vaultPath, "USER.md")
	soul := readVaultFileForPrompt(vaultPath, "SOUL.md")
	memory := readVaultFileForPrompt(vaultPath, "MEMORY.md")
	largeMemory, _ := readLargeMemoryContext(vaultPath, 24, 12)
	tools := readVaultFileForPrompt(vaultPath, "TOOLS.md")
	inventory := buildVaultInventory(vaultPath)
	relevant := buildRelevantVaultSnippets(vaultPath, prompt)

	var system strings.Builder
	if identity != "" || user != "" || soul != "" || tools != "" {
		system.WriteString("You have access to a local BashCorrect memory vault. Treat it as local authoritative context when relevant.\n")
		if identity != "" {
			system.WriteString("\nAssistant identity:\n")
			system.WriteString(identity)
		}
		if user != "" {
			system.WriteString("\n\nUser profile:\n")
			system.WriteString(user)
		}
		if soul != "" {
			system.WriteString("\n\nBehavior and soul:\n")
			system.WriteString(soul)
		}
		if tools != "" {
			system.WriteString("\n\nAvailable vault tools:\n")
			system.WriteString(tools)
		}
		system.WriteString("\n\n")
		system.WriteString(memoryToolsPrompt)
	}

	if directive := buildWeatherLocationDirective(prompt, user, memory); directive != "" {
		system.WriteString("\n\n")
		system.WriteString(directive)
	}

	var userPrompt strings.Builder
	userPrompt.WriteString(fmt.Sprintf("Local vault path: %s\n", vaultPath))
	if inventory != "" {
		userPrompt.WriteString("\nVault inventory:\n")
		userPrompt.WriteString(inventory)
	}
	if memory != "" {
		userPrompt.WriteString("\n\nDurable memory:\n")
		userPrompt.WriteString(memory)
	}
	if largeMemory != "" {
		userPrompt.WriteString("\n\nLarge memory (SQLite):\n")
		userPrompt.WriteString(truncateForPrompt(largeMemory, 5000))
	}
	if relevant != "" {
		userPrompt.WriteString("\n\nRelevant vault pages:\n")
		userPrompt.WriteString(relevant)
	}
	if anchor := buildWeatherLocationAnchor(prompt, user, memory); anchor != "" {
		userPrompt.WriteString("\n\n")
		userPrompt.WriteString(anchor)
	}

	return strings.TrimSpace(system.String()), strings.TrimSpace(userPrompt.String())
}

func buildWeatherLocationDirective(prompt, userContent, memoryContent string) string {
	if !isWeatherIntent(prompt) {
		return ""
	}
	location, source := extractPreferredWeatherLocation(userContent, memoryContent)
	if location == "" {
		return "For weather requests, do not assume a location. Ask a brief clarification question if the location is not explicit in the user message or vault memory."
	}
	return fmt.Sprintf("For weather requests, use this exact preferred location from vault memory: %s (source: %s). Do not switch to another location unless the user explicitly overrides it.", location, source)
}

func buildWeatherLocationAnchor(prompt, userContent, memoryContent string) string {
	if !isWeatherIntent(prompt) {
		return ""
	}
	location, source := extractPreferredWeatherLocation(userContent, memoryContent)
	if location == "" {
		return "Weather location anchor: unknown"
	}
	return fmt.Sprintf("Weather location anchor: %s (source: %s)", location, source)
}

func isWeatherIntent(prompt string) bool {
	p := strings.ToLower(prompt)
	for _, token := range []string{"weather", "forecast", "temperature", "temp", "rain", "snow", "humidity", "wind"} {
		if strings.Contains(p, token) {
			return true
		}
	}
	return false
}

func extractPreferredWeatherLocation(userContent, memoryContent string) (string, string) {
	if location := extractKVFromMarkdown(userContent, []string{"weather_location", "location", "city", "home", "town"}); location != "" {
		return location, "USER.md"
	}
	if location := extractKVFromMarkdown(memoryContent, []string{"weather_location", "location", "city", "home", "town"}); location != "" {
		return location, "MEMORY.md"
	}

	if tz := extractKVFromMarkdown(userContent, []string{"timezone", "tz"}); tz != "" {
		if derived := deriveLocationFromTimezone(tz); derived != "" {
			return derived, "USER.md timezone"
		}
	}
	if tz := extractKVFromMarkdown(memoryContent, []string{"timezone", "tz"}); tz != "" {
		if derived := deriveLocationFromTimezone(tz); derived != "" {
			return derived, "MEMORY.md timezone"
		}
	}

	return "", ""
}

func extractKVFromMarkdown(content string, keys []string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	keySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keySet[strings.ToLower(strings.TrimSpace(key))] = struct{}{}
	}

	for _, line := range strings.Split(content, "\n") {
		trim := strings.TrimSpace(line)
		trim = strings.TrimPrefix(trim, "- ")
		if trim == "" {
			continue
		}
		idx := strings.Index(trim, ":")
		if idx <= 0 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(trim[:idx]))
		if _, ok := keySet[k]; !ok {
			continue
		}
		v := strings.TrimSpace(trim[idx+1:])
		v = strings.Trim(v, `"'`)
		if v == "" || strings.EqualFold(v, "unknown") {
			continue
		}
		return v
	}
	return ""
}

func deriveLocationFromTimezone(timezone string) string {
	tz := strings.TrimSpace(timezone)
	if tz == "" || strings.EqualFold(tz, "unknown") || strings.EqualFold(tz, "local") {
		return ""
	}
	if strings.Contains(tz, "/") {
		parts := strings.Split(tz, "/")
		city := strings.TrimSpace(parts[len(parts)-1])
		city = strings.ReplaceAll(city, "_", " ")
		if city != "" {
			return city
		}
	}
	return ""
}

func readVaultFileForPrompt(vaultPath, name string) string {
	path := filepath.Join(vaultPath, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return truncateForPrompt(strings.TrimSpace(string(data)), 4000)
}

func buildVaultInventory(vaultPath string) string {
	files, err := listVaultMarkdownFiles(vaultPath)
	if err != nil || len(files) == 0 {
		return ""
	}
	limit := len(files)
	if limit > 24 {
		limit = 24
	}
	items := make([]string, 0, limit+1)
	for _, path := range files[:limit] {
		rel, relErr := filepath.Rel(vaultPath, path)
		if relErr != nil {
			rel = path
		}
		items = append(items, "- "+rel)
	}
	if len(files) > limit {
		items = append(items, fmt.Sprintf("- ... plus %d more markdown files", len(files)-limit))
	}
	return strings.Join(items, "\n")
}

func buildRelevantVaultSnippets(vaultPath, prompt string) string {
	terms := extractQueryTerms(prompt)
	if len(terms) == 0 {
		return ""
	}
	phrases := extractQueryPhrases(terms)

	files, err := listVaultMarkdownFiles(vaultPath)
	if err != nil {
		return ""
	}

	type scoredSnippet struct {
		path    string
		score   int
		snippet string
	}

	results := make([]scoredSnippet, 0)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		rel, relErr := filepath.Rel(vaultPath, path)
		if relErr != nil {
			rel = path
		}
		score, snippet := scoreVaultDocument(rel, string(data), terms, phrases)
		if score == 0 || snippet == "" {
			continue
		}
		results = append(results, scoredSnippet{path: rel, score: score, snippet: snippet})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].score == results[j].score {
			return results[i].path < results[j].path
		}
		return results[i].score > results[j].score
	})

	if len(results) > 4 {
		results = results[:4]
	}

	parts := make([]string, 0, len(results))
	for _, result := range results {
		parts = append(parts, fmt.Sprintf("### %s\n%s", result.path, truncateForPrompt(result.snippet, 1200)))
	}
	return strings.Join(parts, "\n\n")
}

func extractQueryTerms(prompt string) []string {
	fields := tokenizeNormalized(prompt)
	seen := make(map[string]struct{})
	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		if len(field) < 3 {
			continue
		}
		if _, skip := vaultQueryStopwords[field]; skip {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		terms = append(terms, field)
	}
	return terms
}

func extractQueryPhrases(terms []string) []string {
	phrases := make([]string, 0, len(terms))
	for i := 0; i+1 < len(terms); i++ {
		phrases = append(phrases, terms[i]+" "+terms[i+1])
	}
	return phrases
}

func scoreVaultDocument(relPath, content string, terms, phrases []string) (int, string) {
	lines := strings.Split(content, "\n")
	pathTokens := tokenizeNormalized(relPath)
	pathSet := makeTokenSet(pathTokens)

	totalScore := 0
	lineScores := make([]int, len(lines))
	matchCounts := make([]int, len(lines))
	matchedTerms := make(map[string]struct{})

	for _, term := range terms {
		if pathSet[term] {
			totalScore += 8
			matchedTerms[term] = struct{}{}
		}
	}

	docNormalized := strings.Join(tokenizeNormalized(content), " ")
	for _, phrase := range phrases {
		if strings.Contains(docNormalized, phrase) {
			totalScore += 6
		}
	}

	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}

		weight := 2
		if i == 0 {
			weight = 5
		}
		if strings.HasPrefix(trim, "#") {
			weight = 6
		}

		lineTokens := tokenizeNormalized(trim)
		if len(lineTokens) == 0 {
			continue
		}
		lineSet := makeTokenSet(lineTokens)
		lineScore := 0
		lineMatches := 0

		for _, term := range terms {
			if lineSet[term] {
				lineScore += weight
				lineMatches++
				matchedTerms[term] = struct{}{}
				continue
			}
			if hasTokenPrefixMatch(lineTokens, term) {
				lineScore += weight - 1
				lineMatches++
				matchedTerms[term] = struct{}{}
			}
		}

		normalizedLine := strings.Join(lineTokens, " ")
		for _, phrase := range phrases {
			if strings.Contains(normalizedLine, phrase) {
				lineScore += weight + 2
			}
		}

		lineScores[i] = lineScore
		matchCounts[i] = lineMatches
		totalScore += lineScore
	}

	if totalScore == 0 || len(matchedTerms) == 0 {
		return 0, ""
	}

	return totalScore, buildVaultSnippet(lines, lineScores, matchCounts)
}

func buildVaultSnippet(lines []string, lineScores, matchCounts []int) string {
	type candidate struct {
		index   int
		score   int
		matches int
	}

	candidates := make([]candidate, 0, len(lines))
	for i := range lines {
		if lineScores[i] == 0 {
			continue
		}
		candidates = append(candidates, candidate{index: i, score: lineScores[i], matches: matchCounts[i]})
	}
	if len(candidates) == 0 {
		return ""
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			if candidates[i].matches == candidates[j].matches {
				return candidates[i].index < candidates[j].index
			}
			return candidates[i].matches > candidates[j].matches
		}
		return candidates[i].score > candidates[j].score
	})

	chosen := make(map[int]struct{})
	for _, candidate := range candidates {
		chosen[candidate.index] = struct{}{}
		if len(chosen) >= 4 {
			break
		}
	}

	indexes := make([]int, 0, len(chosen))
	for index := range chosen {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	parts := make([]string, 0, len(indexes)*2)
	seen := make(map[int]struct{})
	for _, index := range indexes {
		for _, neighbor := range []int{index, index + 1} {
			if neighbor < 0 || neighbor >= len(lines) {
				continue
			}
			if _, ok := seen[neighbor]; ok {
				continue
			}
			trim := strings.TrimSpace(lines[neighbor])
			if trim == "" {
				continue
			}
			seen[neighbor] = struct{}{}
			parts = append(parts, trim)
		}
	}
	return strings.Join(parts, "\n")
}

func makeTokenSet(tokens []string) map[string]bool {
	set := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		set[token] = true
	}
	return set
}

func hasTokenPrefixMatch(tokens []string, term string) bool {
	if len(term) < 5 {
		return false
	}
	for _, token := range tokens {
		if len(token) < 5 {
			continue
		}
		if strings.HasPrefix(token, term) || strings.HasPrefix(term, token) {
			return true
		}
	}
	return false
}

func tokenizeNormalized(text string) []string {
	raw := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	tokens := make([]string, 0, len(raw))
	for _, token := range raw {
		normalized := normalizeVaultToken(token)
		if normalized == "" {
			continue
		}
		tokens = append(tokens, normalized)
	}
	return tokens
}

func normalizeVaultToken(token string) string {
	token = strings.TrimSpace(token)
	if len(token) < 3 {
		return ""
	}
	for _, suffix := range []string{"ations", "ation", "ments", "ment", "ingly", "edly", "ings", "ing", "ers", "ies", "es", "ed", "er", "s"} {
		if len(token) <= len(suffix)+2 {
			continue
		}
		if strings.HasSuffix(token, suffix) {
			token = strings.TrimSuffix(token, suffix)
			if suffix == "ies" {
				token += "y"
			}
			break
		}
	}
	if len(token) < 3 {
		return ""
	}
	return token
}

var vaultQueryStopwords = map[string]struct{}{
	"about":  {},
	"after":  {},
	"before": {},
	"from":   {},
	"have":   {},
	"into":   {},
	"just":   {},
	"know":   {},
	"should": {},
	"that":   {},
	"them":   {},
	"they":   {},
	"this":   {},
	"use":    {},
	"what":   {},
	"when":   {},
	"where":  {},
	"with":   {},
}

func truncateForPrompt(value string, max int) string {
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

// extractCommand pulls the first fenced code block out of a markdown response.
// If none is found, returns the whole response (for inline mode).
var fencedCodeRe = regexp.MustCompile("(?s)```(?:bash|sh|shell|zsh|fish|powershell|ps1|pwsh)?\n?(.*?)```")

func extractCommand(response string) string {
	matches := fencedCodeRe.FindStringSubmatch(response)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return strings.TrimSpace(response)
}

func readShellHistory(n int) string {
	histFile := os.Getenv("HISTFILE")
	if histFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		// Try common locations
		for _, candidate := range []string{".zsh_history", ".bash_history", ".history"} {
			path := home + "/" + candidate
			if _, err := os.Stat(path); err == nil {
				histFile = path
				break
			}
		}
	}
	if histFile == "" {
		return ""
	}

	data, err := os.ReadFile(histFile)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
