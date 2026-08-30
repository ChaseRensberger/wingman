package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/chaserensberger/wingman/internal/daemonstate"
	"github.com/chaserensberger/wingman/skill"
	"github.com/urfave/cli/v3"
)

type skillGitRunner func(context.Context, string, ...string) ([]byte, error)

type installedSkill struct {
	ID       string
	Target   string
	Revision string
}

func skillsCommand() *cli.Command {
	return &cli.Command{
		Name:  "skills",
		Usage: "Manage local Agent Skills",
		Commands: []*cli.Command{
			{
				Name:      "add",
				Usage:     "Install one skill from an HTTPS Git repository",
				ArgsUsage: "<repository-url>",
				Arguments: []cli.Argument{&cli.StringArgs{Name: "source", Min: 1, Max: 1}},
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "global", Usage: "Install in the global skill directory"},
				},
				Action: runSkillsAdd,
			},
		},
	}
}

func runSkillsAdd(ctx context.Context, cmd *cli.Command) error {
	root, err := skillInstallRoot(cmd.Bool("global"))
	if err != nil {
		return err
	}
	installed, err := installSkill(ctx, cmd.StringArgs("source")[0], root, runSkillGit)
	if err != nil {
		return err
	}
	fmt.Printf("Installed skill %q at %s (commit %s)\n", installed.ID, installed.Target, installed.Revision)
	return nil
}

func skillInstallRoot(global bool) (string, error) {
	if global {
		configDir, err := daemonstate.DefaultConfigDir()
		if err != nil {
			return "", fmt.Errorf("resolve global skill directory: %w", err)
		}
		return filepath.Join(configDir, "skills"), nil
	}
	workDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve current directory: %w", err)
	}
	return filepath.Join(workDir, ".wingman", "skills"), nil
}

func installSkill(ctx context.Context, source, root string, runGit skillGitRunner) (installedSkill, error) {
	repository, err := skillRepositoryName(source)
	if err != nil {
		return installedSkill{}, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return installedSkill{}, fmt.Errorf("create skill directory: %w", err)
	}
	staging, err := os.MkdirTemp(root, ".wingman-skill-")
	if err != nil {
		return installedSkill{}, fmt.Errorf("create skill staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	clone := filepath.Join(staging, repository)
	if output, err := runGit(ctx, "", "clone", "--depth", "1", source, clone); err != nil {
		return installedSkill{}, fmt.Errorf("clone skill repository: %w: %s", err, strings.TrimSpace(string(output)))
	}
	skills, err := skill.Discover(staging)
	if err != nil {
		return installedSkill{}, fmt.Errorf("discover installed skill: %w", err)
	}
	flat := false
	if len(skills) == 0 {
		skills, err = skill.Discover(clone)
		if err != nil {
			return installedSkill{}, fmt.Errorf("discover installed skill: %w", err)
		}
		flat = len(skills) == 1
	}
	if len(skills) != 1 {
		return installedSkill{}, fmt.Errorf("skill repository must contain exactly one skill, found %d", len(skills))
	}
	target := filepath.Join(root, skills[0].ID)
	if _, err := os.Lstat(target); err == nil {
		return installedSkill{}, fmt.Errorf("skill %q already exists at %s", skills[0].ID, target)
	} else if !os.IsNotExist(err) {
		return installedSkill{}, fmt.Errorf("inspect skill destination: %w", err)
	}
	output, err := runGit(ctx, clone, "rev-parse", "HEAD")
	if err != nil {
		return installedSkill{}, fmt.Errorf("resolve skill revision: %w: %s", err, strings.TrimSpace(string(output)))
	}
	revision := strings.TrimSpace(string(output))
	if revision == "" {
		return installedSkill{}, fmt.Errorf("resolve skill revision: Git returned an empty revision")
	}
	if flat {
		if err := os.Rename(skills[0].Location, filepath.Join(clone, "SKILL.md")); err != nil {
			return installedSkill{}, fmt.Errorf("prepare flat skill: %w", err)
		}
	}
	if err := os.Rename(clone, target); err != nil {
		return installedSkill{}, fmt.Errorf("install skill: %w", err)
	}
	return installedSkill{ID: skills[0].ID, Target: target, Revision: revision}, nil
}

func skillRepositoryName(source string) (string, error) {
	parsed, err := url.ParseRequestURI(source)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("skill source must be an HTTPS Git repository URL")
	}
	repository := strings.TrimSuffix(path.Base(strings.TrimSuffix(parsed.Path, "/")), ".git")
	if repository == "." || repository == "/" || repository == "" {
		return "", fmt.Errorf("skill source must name a Git repository")
	}
	return repository, nil
}

func runSkillGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}
