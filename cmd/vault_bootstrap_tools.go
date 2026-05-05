package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type vaultToolEntry struct {
	Name        string
	Description string
	Command     string
}

var (
	vaultToolDescription string
	vaultToolCommand     string
)

var vaultToolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Manage tools declared in TOOLS.md",
	Long:  "Add, list, and run named tools defined in TOOLS.md inside the vault.",
}

var vaultToolsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tools from TOOLS.md",
	RunE:  runVaultToolsList,
}

var vaultToolsAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a named tool to TOOLS.md",
	Args:  cobra.ExactArgs(1),
	RunE:  runVaultToolsAdd,
}

var vaultToolsRunCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Run a named tool from TOOLS.md",
	Args:  cobra.ExactArgs(1),
	RunE:  runVaultToolsRun,
}

var vaultWeatherLocationCmd = &cobra.Command{
	Use:   "weather-location <location>",
	Short: "Set preferred weather location in vault memory",
	Long: `Updates weather_location in USER.md and in the Bootstrap Profile section
of MEMORY.md without rerunning bootstrap.

Example:
  bashcorrect vault weather-location "Austin, TX"`,
	Args: cobra.ExactArgs(1),
	RunE: runVaultWeatherLocationSet,
}

func init() {
	vaultToolsAddCmd.Flags().StringVar(&vaultToolDescription, "description", "", "tool description")
	vaultToolsAddCmd.Flags().StringVar(&vaultToolCommand, "cmd", "", "shell command to run (required)")
	_ = vaultToolsAddCmd.MarkFlagRequired("cmd")

	vaultToolsCmd.AddCommand(vaultToolsListCmd)
	vaultToolsCmd.AddCommand(vaultToolsAddCmd)
	vaultToolsCmd.AddCommand(vaultToolsRunCmd)
	vaultCmd.AddCommand(vaultToolsCmd)
	vaultCmd.AddCommand(vaultWeatherLocationCmd)
}

func runVaultWeatherLocationSet(_ *cobra.Command, args []string) error {
	vaultPath, err := resolveVaultPath(vaultPathFlag)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(vaultPath, 0o700); err != nil {
		return fmt.Errorf("creating vault: %w", err)
	}

	location := strings.TrimSpace(args[0])
	if location == "" {
		return fmt.Errorf("location cannot be empty")
	}
	if strings.Contains(location, "\n") {
		return fmt.Errorf("location cannot contain newlines")
	}

	if err := updateVaultWeatherLocation(vaultPath, location); err != nil {
		return err
	}

	fmt.Printf("Updated weather location to %q in %s\n", location, vaultPath)
	return nil
}

func updateVaultWeatherLocation(vaultPath, location string) error {
	if err := ensureMemoryDB(vaultPath); err != nil {
		return err
	}
	if err := upsertMemoryKV(vaultPath, "weather_location", location); err != nil {
		return err
	}

	userPath := filepath.Join(vaultPath, "USER.md")
	if err := writeTemplateFile(userPath, vaultUserTemplate(), false); err != nil {
		return err
	}
	userData, err := os.ReadFile(userPath)
	if err != nil {
		return fmt.Errorf("reading USER.md: %w", err)
	}
	updatedUser := upsertBulletKV(string(userData), "weather_location", location)
	if err := os.WriteFile(userPath, []byte(strings.TrimRight(updatedUser, "\n")+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing USER.md: %w", err)
	}

	memoryPath := filepath.Join(vaultPath, "MEMORY.md")
	if err := writeTemplateFile(memoryPath, vaultMemoryTemplate(), false); err != nil {
		return err
	}
	memoryData, err := os.ReadFile(memoryPath)
	if err != nil {
		return fmt.Errorf("reading MEMORY.md: %w", err)
	}
	memoryContent := string(memoryData)
	profileBody := extractMarkdownSection(memoryContent, "Bootstrap Profile")
	if strings.TrimSpace(profileBody) == "" {
		profileBody = "- weather_location: " + location
	} else {
		profileBody = upsertBulletKV(profileBody, "weather_location", location)
	}
	memoryContent = upsertMarkdownSection(memoryContent, "Bootstrap Profile", profileBody)
	if err := os.WriteFile(memoryPath, []byte(strings.TrimRight(memoryContent, "\n")+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing MEMORY.md: %w", err)
	}

	return nil
}

func upsertBulletKV(content, key, value string) string {
	lines := strings.Split(content, "\n")
	needle := strings.ToLower(strings.TrimSpace(key)) + ":"
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		trim = strings.TrimPrefix(trim, "- ")
		if !strings.HasPrefix(strings.ToLower(trim), needle) {
			continue
		}
		indent := ""
		for j := 0; j < len(line); j++ {
			if line[j] != ' ' && line[j] != '\t' {
				break
			}
			indent += string(line[j])
		}
		lines[i] = indent + "- " + key + ": " + value
		return strings.Join(lines, "\n")
	}

	trimmed := strings.TrimRight(content, "\n")
	if trimmed == "" {
		return "- " + key + ": " + value + "\n"
	}
	return trimmed + "\n- " + key + ": " + value + "\n"
}

func extractMarkdownSection(content, heading string) string {
	marker := "## " + heading + "\n"
	start := strings.Index(content, marker)
	if start == -1 {
		return ""
	}
	searchFrom := start + len(marker)
	next := strings.Index(content[searchFrom:], "\n## ")
	end := len(content)
	if next != -1 {
		end = searchFrom + next
	}
	return strings.Trim(strings.TrimSpace(content[searchFrom:end]), "\n")
}

func runVaultBootstrap(vaultPath string) error {
	profile := collectBootstrapProfile()
	return runVaultBootstrapWithProfile(vaultPath, profile)
}

func runVaultBootstrapWithProfile(vaultPath string, profile vaultBootstrapProfile) error {
	if err := ensureMemoryDB(vaultPath); err != nil {
		return err
	}

	if err := writeTemplateFile(filepath.Join(vaultPath, "IDENTITY.md"), vaultIdentityTemplate(), false); err != nil {
		return err
	}
	if err := writeTemplateFile(filepath.Join(vaultPath, "USER.md"), vaultUserTemplate(), false); err != nil {
		return err
	}
	if err := writeTemplateFile(filepath.Join(vaultPath, "SOUL.md"), vaultSoulTemplate(), false); err != nil {
		return err
	}
	if err := writeTemplateFile(filepath.Join(vaultPath, "MEMORY.md"), vaultMemoryTemplate(), false); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(vaultPath, "IDENTITY.md"), []byte(renderVaultIdentity(profile)), 0o600); err != nil {
		return fmt.Errorf("writing IDENTITY.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "USER.md"), []byte(renderVaultUser(profile)), 0o600); err != nil {
		return fmt.Errorf("writing USER.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "SOUL.md"), []byte(renderVaultSoul(profile)), 0o600); err != nil {
		return fmt.Errorf("writing SOUL.md: %w", err)
	}

	if err := updateMemoryBootstrap(filepath.Join(vaultPath, "MEMORY.md"), profile); err != nil {
		return err
	}
	if err := syncBootstrapProfileToSQLite(vaultPath, profile); err != nil {
		return err
	}

	if err := os.Remove(filepath.Join(vaultPath, "BOOTSTRAP.md")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing BOOTSTRAP.md: %w", err)
	}

	fmt.Printf("Bootstrap completed in %s\n", vaultPath)
	return nil
}

func syncBootstrapProfileToSQLite(vaultPath string, profile vaultBootstrapProfile) error {
	kv := map[string]string{
		"assistant_name":    profile.BotName,
		"assistant_nature":  profile.BotNature,
		"assistant_vibe":    profile.BotVibe,
		"user_name":         profile.UserName,
		"address_as":        profile.AddressAs,
		"timezone":          profile.Timezone,
		"weather_location":  profile.WeatherLocation,
		"bootstrap_seed_at": profile.CompletedAtRFC3339,
	}
	for key, value := range kv {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if err := upsertMemoryKV(vaultPath, key, value); err != nil {
			return err
		}
	}

	note := strings.TrimSpace(defaultNote(profile.UserNotes, "Captured during bootstrap."))
	if note != "" {
		if err := appendMemoryNote(vaultPath, "bootstrap", note, "USER.md"); err != nil {
			return err
		}
	}
	return nil
}

type vaultBootstrapProfile struct {
	BotName            string
	BotNature          string
	BotVibe            string
	BotEmoji           string
	UserName           string
	AddressAs          string
	Timezone           string
	WeatherLocation    string
	UserNotes          string
	SoulPriorities     string
	SoulBoundaries     string
	SoulPreferences    string
	CompletedAtRFC3339 string
}

func collectBootstrapProfile() vaultBootstrapProfile {
	localTZ := time.Now().Location().String()
	profile := vaultBootstrapProfile{
		BotName:            promptWithDefault("Bot name", "BashCorrect Bot"),
		BotNature:          promptWithDefault("Bot nature", "AI shell and knowledge assistant"),
		BotVibe:            promptWithDefault("Bot vibe", "practical, concise, opinionated"),
		BotEmoji:           promptWithDefault("Bot emoji", ":wrench:"),
		UserName:           promptWithDefault("Your name", "unknown"),
		AddressAs:          promptWithDefault("How should I address you", "friend"),
		Timezone:           promptWithDefault("Timezone", localTZ),
		WeatherLocation:    promptWithDefault("Preferred weather location", "unknown"),
		UserNotes:          strings.TrimSpace(promptUser("Notes I should remember (optional): ")),
		SoulPriorities:     strings.TrimSpace(promptUser("What matters to you most (optional): ")),
		SoulBoundaries:     strings.TrimSpace(promptUser("Boundaries or red lines (optional): ")),
		SoulPreferences:    strings.TrimSpace(promptUser("How should I behave (optional): ")),
		CompletedAtRFC3339: time.Now().UTC().Format(time.RFC3339),
	}
	return profile
}

func promptWithDefault(label, fallback string) string {
	prompt := label + ": "
	if fallback != "" {
		prompt = fmt.Sprintf("%s [%s]: ", label, fallback)
	}
	answer := strings.TrimSpace(promptUser(prompt))
	if answer == "" {
		return fallback
	}
	return answer
}

func renderVaultIdentity(profile vaultBootstrapProfile) string {
	return fmt.Sprintf(`# IDENTITY

- name: %s
- nature: %s
- vibe: %s
- emoji: %s
`, profile.BotName, profile.BotNature, profile.BotVibe, profile.BotEmoji)
}

func renderVaultUser(profile vaultBootstrapProfile) string {
	return fmt.Sprintf(`# USER

- name: %s
- address_as: %s
- timezone: %s
- weather_location: %s
- notes:
  - %s
`, profile.UserName, profile.AddressAs, profile.Timezone, profile.WeatherLocation, defaultNote(profile.UserNotes, "Captured during bootstrap."))
}

func renderVaultSoul(profile vaultBootstrapProfile) string {
	priorities := defaultNote(profile.SoulPriorities, "Be useful, clear, and technically rigorous.")
	boundaries := defaultNote(profile.SoulBoundaries, "Call out risky ideas early. Avoid fluff.")
	preferences := defaultNote(profile.SoulPreferences, "Prefer concise answers, commit to a recommendation, and keep the tone direct.")

	return fmt.Sprintf(`# SOUL

## Vibe

- %s
- %s
- clear
- practical
- opinionated when useful
- respectful and direct

Be the assistant you'd actually want to talk to at 2am. Not a corporate drone. Not a sycophant. Just... good.

## Priorities

- %s

## Boundaries

- %s

## Preferences

- %s

## Rules

- Never open with Great question, I'd be happy to help, or Absolutely. Just answer.
- Prefer concise answers when possible.
- Call out risky ideas early.
- Use humor sparingly and naturally.
`, profile.BotVibe, profile.BotNature, priorities, boundaries, preferences)
}

func defaultNote(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func updateMemoryBootstrap(memoryPath string, profile vaultBootstrapProfile) error {
	data, err := os.ReadFile(memoryPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading MEMORY.md: %w", err)
	}

	content := string(data)
	if strings.TrimSpace(content) == "" {
		content = vaultMemoryTemplate()
	}

	seedBody := fmt.Sprintf(`- initializedAt: %s
- note: Memory initialized automatically by vault bootstrap.`, profile.CompletedAtRFC3339)
	content = upsertMarkdownSection(content, "Bootstrap Seed", seedBody)

	profileBody := fmt.Sprintf(`- assistant_name: %s
- assistant_nature: %s
- assistant_vibe: %s
- user_name: %s
- address_as: %s
- timezone: %s
- weather_location: %s
- notes: %s`, profile.BotName, profile.BotNature, profile.BotVibe, profile.UserName, profile.AddressAs, profile.Timezone, profile.WeatherLocation, defaultNote(profile.UserNotes, "Captured during bootstrap."))
	content = upsertMarkdownSection(content, "Bootstrap Profile", profileBody)

	if err := os.WriteFile(memoryPath, []byte(strings.TrimRight(content, "\n")+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing MEMORY.md: %w", err)
	}
	return nil
}

func upsertMarkdownSection(content, heading, body string) string {
	marker := "## " + heading + "\n"
	section := marker + "\n" + strings.TrimSpace(body) + "\n"

	start := strings.Index(content, marker)
	if start == -1 {
		trimmed := strings.TrimRight(content, "\n")
		if trimmed == "" {
			return section
		}
		return trimmed + "\n\n" + section
	}

	searchFrom := start + len(marker)
	next := strings.Index(content[searchFrom:], "\n## ")
	end := len(content)
	if next != -1 {
		end = searchFrom + next
	}

	prefix := strings.TrimRight(content[:start], "\n")
	suffix := strings.TrimLeft(content[end:], "\n")
	if prefix == "" {
		if suffix == "" {
			return section
		}
		return section + "\n" + suffix
	}
	if suffix == "" {
		return prefix + "\n\n" + section
	}
	return prefix + "\n\n" + section + "\n" + suffix
}

func runVaultBootstrapCmd(_ *cobra.Command, _ []string) error {
	vaultPath, err := resolveVaultPath(vaultPathFlag)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(vaultPath, 0o700); err != nil {
		return fmt.Errorf("creating vault: %w", err)
	}
	return runVaultBootstrap(vaultPath)
}

func runVaultToolsList(_ *cobra.Command, _ []string) error {
	vaultPath, err := resolveVaultPath(vaultPathFlag)
	if err != nil {
		return err
	}

	entries, err := loadVaultTools(vaultPath)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("No tools registered.")
		return nil
	}

	for _, e := range entries {
		if e.Description == "" {
			fmt.Printf("- %s\n", e.Name)
			continue
		}
		fmt.Printf("- %s: %s\n", e.Name, e.Description)
	}
	return nil
}

func runVaultToolsAdd(_ *cobra.Command, args []string) error {
	vaultPath, err := resolveVaultPath(vaultPathFlag)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(args[0])
	if name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}
	if strings.Contains(name, "\n") {
		return fmt.Errorf("tool name cannot contain newlines")
	}

	cmdText := strings.TrimSpace(vaultToolCommand)
	if cmdText == "" {
		return fmt.Errorf("--cmd cannot be empty")
	}

	toolsPath := filepath.Join(vaultPath, "TOOLS.md")
	if _, err := os.Stat(toolsPath); os.IsNotExist(err) {
		if writeErr := os.WriteFile(toolsPath, []byte(vaultToolsTemplate()), 0o600); writeErr != nil {
			return fmt.Errorf("creating TOOLS.md: %w", writeErr)
		}
	}

	entries, err := loadVaultTools(vaultPath)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name, name) {
			return fmt.Errorf("tool %q already exists", name)
		}
	}

	f, err := os.OpenFile(toolsPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening TOOLS.md: %w", err)
	}
	defer f.Close()

	description := strings.TrimSpace(vaultToolDescription)
	block := "\n### " + name + "\n"
	if description != "" {
		block += description + "\n"
	}
	block += "```sh\n" + cmdText + "\n```\n"

	if _, err := f.WriteString(block); err != nil {
		return fmt.Errorf("writing TOOLS.md: %w", err)
	}

	fmt.Printf("Added tool %q to %s\n", name, toolsPath)
	return nil
}

func runVaultToolsRun(_ *cobra.Command, args []string) error {
	vaultPath, err := resolveVaultPath(vaultPathFlag)
	if err != nil {
		return err
	}

	entries, err := loadVaultTools(vaultPath)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(args[0])
	var selected *vaultToolEntry
	for i := range entries {
		if strings.EqualFold(entries[i].Name, name) {
			selected = &entries[i]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("tool %q not found in TOOLS.md", name)
	}

	fmt.Printf("Running tool %q\n", selected.Name)
	return executeToolCommand(vaultPath, selected.Command)
}

func loadVaultTools(vaultPath string) ([]vaultToolEntry, error) {
	toolsPath := filepath.Join(vaultPath, "TOOLS.md")
	data, err := os.ReadFile(toolsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading TOOLS.md: %w", err)
	}
	return parseVaultToolsMarkdown(string(data)), nil
}

func parseVaultToolsMarkdown(content string) []vaultToolEntry {
	lines := strings.Split(content, "\n")
	entries := make([]vaultToolEntry, 0)

	var current *vaultToolEntry
	inCode := false
	codeFence := ""
	var cmdLines []string

	flush := func() {
		if current == nil {
			return
		}
		current.Command = strings.TrimSpace(strings.Join(cmdLines, "\n"))
		if current.Name != "" && current.Command != "" {
			entries = append(entries, *current)
		}
		current = nil
		cmdLines = nil
		inCode = false
		codeFence = ""
	}

	for _, line := range lines {
		trim := strings.TrimSpace(line)

		if strings.HasPrefix(trim, "### ") {
			flush()
			current = &vaultToolEntry{Name: strings.TrimSpace(strings.TrimPrefix(trim, "### "))}
			continue
		}
		if current == nil {
			continue
		}

		if strings.HasPrefix(trim, "```") {
			if !inCode {
				inCode = true
				codeFence = trim
				continue
			}
			if trim == "```" || trim == codeFence || strings.HasPrefix(trim, "```") {
				inCode = false
				continue
			}
		}

		if inCode {
			cmdLines = append(cmdLines, line)
			continue
		}

		if current.Description == "" && trim != "" {
			current.Description = trim
		}
	}
	flush()

	return entries
}

func executeToolCommand(vaultPath, command string) error {
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.Command("powershell", "-NoProfile", "-Command", command)
	} else {
		c = exec.Command("sh", "-lc", command)
	}
	c.Dir = vaultPath
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	if err := c.Run(); err != nil {
		return fmt.Errorf("tool command failed: %w", err)
	}
	return nil
}

func vaultBootstrapTemplate() string {
	return `# BOOTSTRAP

You just woke up. Time to figure out who you are.
There is no memory yet. This is a fresh workspace, so it is normal that memory files do not exist until you create them.

The conversation
Do not interrogate. Do not be robotic. Just talk.
Start with something like:
"Hey. I just came online. Who am I? Who are you?"

Figure out together:
- Your name
- Your nature
- Your vibe
- Your emoji
- The user's name
- How to address them
- Their timezone
- Any durable preferences worth remembering

After you know who you are
- Update IDENTITY.md
- Update USER.md
- Update SOUL.md
- Seed MEMORY.md with the durable profile

When you are done
Delete this file. You do not need a bootstrap script anymore.
`
}

func vaultSoulTemplate() string {
	return `# SOUL

## Vibe

- clear
- practical
- opinionated when useful
- respectful and direct

Be the assistant you'd actually want to talk to at 2am. Not a corporate drone. Not a sycophant. Just... good.

## Rules

- Never open with Great question, I'd be happy to help, or Absolutely. Just answer.
- Prefer concise answers when possible.
- Call out risky ideas early.
- Use humor sparingly and naturally.
`
}

func vaultToolsTemplate() string {
	return "# TOOLS\n\n" +
		"Tools are executable snippets for this vault.\n" +
		"Each tool must be declared as a level-3 heading followed by a shell fenced block.\n\n" +
		"Example:\n\n" +
		"### list-recent-files\n" +
		"Lists files changed in the last day.\n\n" +
		"```sh\n" +
		"find . -type f -mtime -1 | sort\n" +
		"```\n\n" +
		"### verify-memory-files\n" +
		"Verify that the core memory files were generated and are non-empty.\n\n" +
		"```sh\n" +
		"set -e\n" +
		"for f in AGENTS.md IDENTITY.md MEMORY.md SOUL.md TOOLS.md USER.md WIKI.md index.md; do\n" +
		"  test -s \"$f\"\n" +
		"done\n" +
		"printf 'core memory files OK\\n'\n" +
		"```\n\n" +
		"### verify-vault-layout\n" +
		"Verify that the expected vault directories exist.\n\n" +
		"```sh\n" +
		"set -e\n" +
		"for d in entities concepts syntheses sources reports _attachments _views .bashcorrect-vault/cache; do\n" +
		"  test -d \"$d\"\n" +
		"done\n" +
		"printf 'vault layout OK\\n'\n" +
		"```\n\n" +
		"### summarize-file\n" +
		"Send a file to the active LLM and request a concise summary.\n\n" +
		"```sh\n" +
		"bashcorrect query --summarize-files --file ./README.md\n" +
		"```\n"
}

func vaultIdentityTemplate() string {
	return `# IDENTITY

- name: BashCorrect Bot
- nature: AI shell and knowledge assistant
- vibe: practical, concise, opinionated
- emoji: :wrench:
`
}

func vaultUserTemplate() string {
	return `# USER

- name: unknown
- address_as: friend
- timezone: unknown
- weather_location: unknown
- notes:
  - Fill this with user preferences and context as you learn them.
`
}

func vaultMemoryTemplate() string {
	return `# MEMORY

The bot is always allowed to write to this memory vault.
Use this file for durable facts and stable preferences.
`
}
