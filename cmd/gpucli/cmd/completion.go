package cmd

import (
    "github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
    Use:   "completion [bash|zsh|fish|powershell]",
    Short: "Generate shell completion script",
    Long: `Generate shell completion scripts for gpucli.

To load completions:

Bash:
  source <(gpucli completion bash)

Zsh:
  gpucli completion zsh > "${fpath[1]}/_gpucli"

Fish:
  gpucli completion fish | source
`,
    Args: cobra.ExactValidArgs(1),
    ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
    Run: func(cmd *cobra.Command, args []string) {
        switch args[0] {
        case "bash":
            rootCmd.GenBashCompletion(cmd.OutOrStdout())
        case "zsh":
            rootCmd.GenZshCompletion(cmd.OutOrStdout())
        case "fish":
            rootCmd.GenFishCompletion(cmd.OutOrStdout(), true)
        case "powershell":
            rootCmd.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
        }
    },
}

func init() {
    rootCmd.AddCommand(completionCmd)
}