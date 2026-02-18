package project

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// GitHubRepo represents a GitHub repository from `gh repo list`.
type GitHubRepo struct {
	Name          string `json:"name"`
	NameWithOwner string `json:"nameWithOwner"`
}

// ListGitHubRepos returns repositories for the authenticated user via the gh CLI.
func ListGitHubRepos() ([]GitHubRepo, error) {
	cmd := exec.Command("gh", "repo", "list", "--json", "name,nameWithOwner", "--limit", "50", "--no-archived")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing repos (is gh CLI installed and authenticated?): %w", err)
	}
	var repos []GitHubRepo
	if err := json.Unmarshal(out, &repos); err != nil {
		return nil, fmt.Errorf("parsing repo list: %w", err)
	}
	return repos, nil
}

// SetGitHubSecret sets a repository secret using the gh CLI.
func SetGitHubSecret(repo, name, value string) error {
	cmd := exec.Command("gh", "secret", "set", name, "--repo", repo, "--body", value)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("setting secret %s: %w\n%s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// SetEnvironmentSecrets sets all GitHub Actions secrets for an environment.
// prefix is "DEV" or "PROD".
func SetEnvironmentSecrets(repo, prefix, vpsUser, deployPath, sshKey, vpsHost, dockerHubUsername, dockerHubToken string) error {
	secrets := map[string]string{
		prefix + "_VPS_USER":        vpsUser,
		prefix + "_VPS_DEPLOY_PATH": deployPath,
		prefix + "_VPS_SSH_KEY":     sshKey,
	}

	// Shared secrets (same across environments)
	secrets["VPS_HOST"] = vpsHost
	secrets["DOCKERHUB_USERNAME"] = dockerHubUsername
	secrets["DOCKERHUB_TOKEN"] = dockerHubToken

	for name, value := range secrets {
		if err := SetGitHubSecret(repo, name, value); err != nil {
			return err
		}
	}
	return nil
}
