package cmd

import (
	"fmt"

	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/arnor/internal/project"
	"github.com/spf13/cobra"
)

var deployEnv string

var deployCmd = &cobra.Command{
	Use:   "deploy <project-name>",
	Short: "Trigger a deploy via GitHub Actions",
	Args:  cobra.ExactArgs(1),
	RunE:  runDeploy,
}

func init() {
	deployCmd.Flags().StringVar(&deployEnv, "env", "", "environment to deploy (dev or prod)")
	deployCmd.MarkFlagRequired("env")
	rootCmd.AddCommand(deployCmd)
}

func runDeploy(cmd *cobra.Command, args []string) error {
	projectName := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p := cfg.FindProject(projectName)
	if p == nil {
		return fmt.Errorf("project not found: %s", projectName)
	}

	env, ok := p.Environments[deployEnv]
	if !ok {
		return fmt.Errorf("environment %q not configured for project %s", deployEnv, projectName)
	}

	fmt.Println("Ensuring workflow supports manual dispatch...")
	if err := project.EnsureWorkflowDispatch(p.Repo, deployEnv, p.Name); err != nil {
		return fmt.Errorf("ensuring workflow dispatch: %w", err)
	}

	workflowFile := project.WorkflowFile(deployEnv)
	ref := project.DeployRef(env)

	fmt.Printf("Triggering %s deploy for %s (ref: %s)...\n", deployEnv, p.Repo, ref)

	if err := project.TriggerWorkflow(p.Repo, workflowFile, ref); err != nil {
		return err
	}

	fmt.Println("Workflow dispatched successfully.")
	return nil
}
