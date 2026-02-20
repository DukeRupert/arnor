package project

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/dukerupert/arnor/internal/config"
)

// GitHubRepo represents a GitHub repository from `gh repo list`.
type GitHubRepo struct {
	Name          string `json:"name"`
	NameWithOwner string `json:"nameWithOwner"`
}

// GitHubSecret represents a repository secret (name + last updated; values are never exposed).
type GitHubSecret struct {
	Name      string `json:"name"`
	UpdatedAt string `json:"updatedAt"`
}

// WorkflowRun represents a recent GitHub Actions workflow run.
type WorkflowRun struct {
	DisplayTitle string `json:"displayTitle"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	HeadBranch   string `json:"headBranch"`
	CreatedAt    string `json:"createdAt"`
	Event        string `json:"event"`
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

// ListGitHubSecrets returns the secret names and last-updated timestamps for a repo.
func ListGitHubSecrets(repo string) ([]GitHubSecret, error) {
	cmd := exec.Command("gh", "secret", "list", "--repo", repo, "--json", "name,updatedAt")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing secrets for %s: %w", repo, err)
	}
	var secrets []GitHubSecret
	if err := json.Unmarshal(out, &secrets); err != nil {
		return nil, fmt.Errorf("parsing secrets: %w", err)
	}
	return secrets, nil
}

// ListWorkflowRuns returns recent workflow runs for a repo.
func ListWorkflowRuns(repo string, limit int) ([]WorkflowRun, error) {
	cmd := exec.Command("gh", "run", "list", "--repo", repo,
		"--json", "displayTitle,status,conclusion,headBranch,createdAt,event",
		"--limit", fmt.Sprintf("%d", limit))
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing workflow runs for %s: %w", repo, err)
	}
	var runs []WorkflowRun
	if err := json.Unmarshal(out, &runs); err != nil {
		return nil, fmt.Errorf("parsing workflow runs: %w", err)
	}
	return runs, nil
}

// SetGitHubSecret sets a repository secret using the gh CLI.
// Values are passed via stdin to safely handle multi-line content like SSH keys.
func SetGitHubSecret(repo, name, value string) error {
	cmd := exec.Command("gh", "secret", "set", name, "--repo", repo)
	cmd.Stdin = strings.NewReader(value)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("setting secret %s: %w\n%s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// PushWorkflowFile creates or updates a file in a GitHub repo via the Contents API.
func PushWorkflowFile(repo, path, content, branch, commitMsg string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))

	// Check if file already exists to get its SHA (required for updates).
	sha := ""
	cmd := exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/contents/%s?ref=%s", repo, path, branch),
		"--jq", ".sha")
	if out, err := cmd.Output(); err == nil {
		sha = strings.TrimSpace(string(out))
	}

	args := []string{
		"api", "-X", "PUT",
		fmt.Sprintf("repos/%s/contents/%s", repo, path),
		"-f", "message=" + commitMsg,
		"-f", "content=" + encoded,
		"-f", "branch=" + branch,
	}
	if sha != "" {
		args = append(args, "-f", "sha="+sha)
	}

	cmd = exec.Command("gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pushing %s to %s: %w\n%s", path, repo, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// DefaultBranch returns the default branch name for a GitHub repo.
func DefaultBranch(repo string) (string, error) {
	cmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s", repo), "--jq", ".default_branch")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("getting default branch for %s: %w", repo, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// TriggerWorkflow dispatches a GitHub Actions workflow via the gh CLI.
// If ref does not exist in the repo it falls back to the default branch.
func TriggerWorkflow(repo, workflowFile, ref string) error {
	// Verify the ref exists; fall back to default branch if it doesn't.
	check := exec.Command("gh", "api", fmt.Sprintf("repos/%s/branches/%s", repo, ref), "--silent")
	if err := check.Run(); err != nil {
		fallback, fbErr := DefaultBranch(repo)
		if fbErr != nil {
			return fmt.Errorf("ref %q not found and could not determine default branch: %w", ref, fbErr)
		}
		ref = fallback
	}

	cmd := exec.Command("gh", "workflow", "run", workflowFile, "--repo", repo, "--ref", ref)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("triggering workflow %s: %w\n%s", workflowFile, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// EnsureWorkflowDispatch checks that the workflow file exists on the default
// branch with a workflow_dispatch trigger. If the file is missing it generates
// and pushes it; if it exists without the trigger it patches the file in place.
func EnsureWorkflowDispatch(repo, envName, projectName, dockerHubUsername string) error {
	filename := WorkflowFile(envName)
	path := ".github/workflows/" + filename

	branch, err := DefaultBranch(repo)
	if err != nil {
		return fmt.Errorf("getting default branch: %w", err)
	}

	dockerImage := dockerHubUsername + "/" + projectName

	// Try to fetch the existing workflow file.
	cmd := exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/contents/%s?ref=%s", repo, path, branch),
		"--jq", ".content")
	out, err := cmd.Output()

	if err != nil {
		// File doesn't exist — generate and push a fresh copy.
		var content string
		switch envName {
		case "dev":
			content, err = GenerateDevWorkflow(dockerImage)
		case "prod":
			content, err = GenerateProdWorkflow(dockerImage, branch)
		default:
			return fmt.Errorf("unknown environment: %s", envName)
		}
		if err != nil {
			return fmt.Errorf("generating workflow: %w", err)
		}
		return PushWorkflowFile(repo, path, content, branch,
			fmt.Sprintf("Add %s deploy workflow", envName))
	}

	// File exists — check if it's up to date.
	decoded, err := base64.StdEncoding.DecodeString(
		strings.ReplaceAll(strings.TrimSpace(string(out)), "\n", ""))
	if err != nil {
		return fmt.Errorf("decoding workflow content: %w", err)
	}

	content := string(decoded)
	if strings.Contains(content, "workflow_dispatch") && strings.Contains(content, "scp-action") && strings.Contains(content, "docker login") {
		return nil
	}

	// Workflow is out of date — regenerate from template.
	var updated string
	switch envName {
	case "dev":
		updated, err = GenerateDevWorkflow(dockerImage)
	case "prod":
		updated, err = GenerateProdWorkflow(dockerImage, branch)
	default:
		return fmt.Errorf("unknown environment: %s", envName)
	}
	if err != nil {
		return fmt.Errorf("generating workflow: %w", err)
	}

	return PushWorkflowFile(repo, path, updated, branch,
		fmt.Sprintf("Update %s deploy workflow", envName))
}

// WorkflowFile returns the workflow filename for a given environment.
func WorkflowFile(envName string) string {
	return "deploy-" + envName + ".yml"
}

// managedWorkflows is the set of workflow filenames that arnor manages.
var managedWorkflows = map[string]bool{
	"deploy-dev.yml":  true,
	"deploy-prod.yml": true,
}

// DeleteStaleWorkflows removes any workflow files from the repo that are not
// managed by arnor. This prevents old hand-written workflows (e.g. deploy.yml)
// from conflicting with the generated deploy-dev.yml / deploy-prod.yml.
func DeleteStaleWorkflows(repo, branch string) error {
	cmd := exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/contents/.github/workflows?ref=%s", repo, branch),
		"--jq", ".[].name")
	out, err := cmd.Output()
	if err != nil {
		// No workflows directory — nothing to clean up.
		return nil
	}

	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if name == "" || managedWorkflows[name] {
			continue
		}
		path := ".github/workflows/" + name
		// Get the file SHA required for deletion.
		shaCmd := exec.Command("gh", "api",
			fmt.Sprintf("repos/%s/contents/%s?ref=%s", repo, path, branch),
			"--jq", ".sha")
		shaOut, err := shaCmd.Output()
		if err != nil {
			continue
		}
		sha := strings.TrimSpace(string(shaOut))

		delCmd := exec.Command("gh", "api", "-X", "DELETE",
			fmt.Sprintf("repos/%s/contents/%s", repo, path),
			"-f", "message=Remove stale workflow "+name,
			"-f", "sha="+sha,
			"-f", "branch="+branch)
		if delOut, err := delCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("deleting stale workflow %s: %w\n%s", name, err, strings.TrimSpace(string(delOut)))
		}
	}
	return nil
}

// DeployRef returns the git ref to deploy for a given environment.
func DeployRef(env config.Environment) string {
	return env.Branch
}

// SetEnvironmentSecrets sets all GitHub Actions secrets for an environment.
// prefix is "DEV" or "PROD".
func SetEnvironmentSecrets(repo, prefix, vpsUser, deployPath, sshKey, vpsHost, dockerHubUsername, dockerHubToken string, port int) error {
	secrets := map[string]string{
		prefix + "_VPS_USER":        vpsUser,
		prefix + "_VPS_DEPLOY_PATH": deployPath,
		prefix + "_VPS_SSH_KEY":     sshKey,
		prefix + "_PORT":            fmt.Sprintf("%d", port),
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
