package project

import (
	"fmt"
	"os/exec"
	"strings"
)

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
func SetEnvironmentSecrets(repo, prefix, vpsUser, deployPath, sshKey, vpsHost string) error {
	secrets := map[string]string{
		prefix + "_VPS_USER":        vpsUser,
		prefix + "_VPS_DEPLOY_PATH": deployPath,
		prefix + "_VPS_SSH_KEY":     sshKey,
	}

	// VPS_HOST is shared across environments
	secrets["VPS_HOST"] = vpsHost

	for name, value := range secrets {
		fmt.Printf("  Setting secret %s...\n", name)
		if err := SetGitHubSecret(repo, name, value); err != nil {
			return err
		}
	}
	return nil
}
