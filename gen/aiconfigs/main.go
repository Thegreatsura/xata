package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type Config struct {
	AiGenerateTargets []string
}

type AIMarkdownFile struct {
	Path    string
	Content string
	IsRoot  bool
}

type ToolConfig struct {
	Name            string
	OutputFileGlobs []string
	RootOutputFile  string `json:",omitempty"`
	Transform       TransformFunc
}

type ToolOutput struct {
	Content    string
	OutputPath string
}

type TransformFunc func(content string, filePath string) ToolOutput

const (
	AISourceFile = "AI.md"
	ClaudeMd     = "CLAUDE.md"
	AgentMd      = "AGENT.md"
	AgentsMd     = "AGENTS.md"
)

var IgnoreGlobs = []string{
	"**/.git/**",
}

var toolConfigs = []ToolConfig{
	{
		Name:            "claude-code",
		OutputFileGlobs: []string{"**/" + ClaudeMd},
		Transform:       transformCopyTo(ClaudeMd),
	},
	{
		Name:            "cursor",
		OutputFileGlobs: []string{".cursor/rules/*"},
		Transform: func(content string, filePath string) ToolOutput {
			dir := filepath.Dir(filePath)
			description := "root project"
			if dir != "." {
				description = dir
			}

			frontmatter := "---\n" +
				fmt.Sprintf("description: AI instructions for %s\n", description) +
				"globs: [\"**/*\"]\n" +
				"---\n"

			fileName := strings.ReplaceAll(strings.ReplaceAll(dir, "/", "-"), "\\", "-")
			if fileName == "" || fileName == "." {
				fileName = "AI"
			}

			return ToolOutput{
				Content:    frontmatter + content,
				OutputPath: filepath.Join(".cursor/rules", fmt.Sprintf("%s.mdc", fileName)),
			}
		},
	},
	{
		Name:            "amp",
		OutputFileGlobs: []string{"**/" + AgentMd},
		RootOutputFile:  AgentMd,
		Transform:       transformCopyTo(AgentMd),
	},
	{
		Name:            "openai-codex",
		OutputFileGlobs: []string{"**/" + AgentsMd},
		Transform:       transformCopyTo(AgentsMd),
	},
}

func transformCopyTo(fileName string) TransformFunc {
	return func(content string, filePath string) ToolOutput {
		dir := filepath.Dir(filePath)
		return ToolOutput{
			Content:    content,
			OutputPath: filepath.Join(dir, fileName),
		}
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var aiGenerateTargetsFlag string
	var aiGenerateTargets []string
	flag.StringVar(&aiGenerateTargetsFlag, "t", "", "comma separated list of AI_GENERATE_TARGETS")
	flag.Parse()

	if aiGenerateTargetsFlag != "" {
		var err error
		aiGenerateTargets, err = parseAIGenerateTargets(aiGenerateTargetsFlag)
		if err != nil {
			return err
		}
	}

	config, err := readConfig()
	if err != nil {
		return err
	}
	config.AiGenerateTargets = append(config.AiGenerateTargets, aiGenerateTargets...)
	slices.Sort(config.AiGenerateTargets)
	config.AiGenerateTargets = slices.Compact(config.AiGenerateTargets)

	if len(config.AiGenerateTargets) == 0 {
		return fmt.Errorf("no AI_GENERATE_TARGETS specified")
	}

	return generateAIConfigs(config)
}

func readConfig() (Config, error) {
	var config Config
	envAIGenerateTargets := os.Getenv("AI_GENERATE_TARGET")
	targets, err := parseAIGenerateTargets(envAIGenerateTargets)
	if err != nil {
		return Config{}, err
	}
	config.AiGenerateTargets = targets
	return config, nil
}

func parseAIGenerateTargets(in string) ([]string, error) {
	in = strings.TrimSpace(in)
	if in == "" {
		return nil, nil
	}

	targets := strings.Split(in, ",")
	targets = slices.Compact(targets)
	for _, target := range targets {
		if _, err := findToolConfig(target); err != nil {
			return nil, fmt.Errorf("invalid AI_GENERATE_TARGET: %s: %w", target, err)
		}
	}
	return targets, nil
}

func findToolConfig(name string) (ToolConfig, error) {
	for _, toolConfig := range toolConfigs {
		if toolConfig.Name == name {
			return toolConfig, nil
		}
	}
	return ToolConfig{}, fmt.Errorf("unknown tool: %s", name)
}

func generateAIConfigs(config Config) error {
	selectedConfigs := make([]ToolConfig, 0, len(config.AiGenerateTargets))
	for _, target := range config.AiGenerateTargets {
		toolConfig, err := findToolConfig(target)
		if err != nil {
			return err
		}
		selectedConfigs = append(selectedConfigs, toolConfig)
	}

	if err := deleteAllExistingTargetFiles(); err != nil {
		return err
	}

	// Find all AI source files
	aiFilePaths, err := glob(filepath.Join("**", AISourceFile), IgnoreGlobs)
	if err != nil {
		return err
	}
	log.Printf("Found %d %s files", len(aiFilePaths), AISourceFile)

	// Process each file
	aiFiles := make([]AIMarkdownFile, 0, len(aiFilePaths))
	for _, path := range aiFilePaths {
		log.Printf("Processing %s...", path)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		aiFiles = append(aiFiles, AIMarkdownFile{
			Path:    path,
			Content: string(content),
			IsRoot:  filepath.Dir(path) == "." || filepath.Dir(path) == "",
		})
	}

	// Process each tool config
	for _, toolConfig := range selectedConfigs {
		log.Printf("\nProcessing tool: %s", toolConfig.Name)
		if toolConfig.RootOutputFile != "" {
			if err := processRootAggregation(aiFiles, toolConfig); err != nil {
				return err
			}
		} else {
			if err := processIndividualFiles(aiFiles, toolConfig); err != nil {
				return err
			}
		}
	}

	log.Println("\nAI config generation completed successfully!")
	return nil
}

func deleteAllExistingTargetFiles() error {
	for _, toolConfig := range toolConfigs {
		for _, globPattern := range toolConfig.OutputFileGlobs {
			files, err := glob(globPattern, IgnoreGlobs)
			if err != nil {
				return err
			}
			for _, file := range files {
				log.Println("deleting file", file)
				if err := os.Remove(file); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func processRootAggregation(aiFiles []AIMarkdownFile, toolConfig ToolConfig) error {
	if toolConfig.RootOutputFile == "" {
		log.Printf("no root output file configured for tool: %s", toolConfig.Name)
		return nil
	}

	rootContent := generateRootAggregate(aiFiles, toolConfig)
	if err := writeFile(toolConfig.RootOutputFile, rootContent); err != nil {
		return err
	}
	log.Printf("Generated root aggregate: %s", toolConfig.RootOutputFile)
	return nil
}

func generateRootAggregate(aiFiles []AIMarkdownFile, toolConfig ToolConfig) string {
	var rootFile *AIMarkdownFile
	var otherFiles []AIMarkdownFile
	for _, file := range aiFiles {
		if file.IsRoot {
			rootFile = &file
		} else {
			otherFiles = append(otherFiles, file)
		}
	}

	var rootSection string
	if rootFile != nil {
		rootSection = toolConfig.Transform(rootFile.Content, rootFile.Path).Content
	}

	var otherSections []string
	for _, file := range otherFiles {
		sectionHeader := fmt.Sprintf("## %s", filepath.Base(filepath.Dir(file.Path)))
		transformedContent := toolConfig.Transform(file.Content, file.Path).Content
		if transformedContent != "" {
			otherSections = append(otherSections, fmt.Sprintf("%s\n\n%s", sectionHeader, transformedContent))
		}
	}

	var sections []string
	if rootSection != "" {
		sections = append(sections, rootSection)
	}
	sections = append(sections, otherSections...)

	return strings.Join(sections, "\n\n---\n\n")
}

func processIndividualFiles(aiFiles []AIMarkdownFile, toolConfig ToolConfig) error {
	for _, file := range aiFiles {
		transformed := toolConfig.Transform(file.Content, file.Path)
		if err := writeFile(transformed.OutputPath, transformed.Content); err != nil {
			return err
		}
	}
	return nil
}

func glob(pattern string, ignorePatterns []string) ([]string, error) {
	matches, err := doublestar.FilepathGlob(pattern, doublestar.WithFailOnIOErrors())
	if err != nil {
		return nil, err
	}

	filteredMatches := matches[:0]
	for _, match := range matches {
		shouldIgnore := false
		for _, pattern := range ignorePatterns {
			if doublestar.MatchUnvalidated(pattern, match) {
				shouldIgnore = true
				break
			}
		}
		if !shouldIgnore {
			filteredMatches = append(filteredMatches, match)
		}
	}
	return filteredMatches, nil
}

func writeFile(path string, content string) error {
	log.Println("writing file", path)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}
