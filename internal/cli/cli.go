// Package cli implements the gobackend command-line interface.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/alphayan/go-backend-kit/internal/generate"
	"github.com/spf13/cobra"
)

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

func New(info BuildInfo, stdout, stderr io.Writer) *cobra.Command {
	if info.Version == "" {
		info.Version = "devel"
	}
	root := &cobra.Command{
		Use:           "gobackend",
		Short:         "Generate production-shaped Go CRUD backends",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.AddCommand(newCommand(info), addCommand(info), generateCommand(info), checkCommand(info), versionCommand(info))
	return root
}

func Execute(ctx context.Context, info BuildInfo, stdout, stderr io.Writer, args []string) error {
	command := New(info, stdout, stderr)
	command.SetArgs(args)
	return command.ExecuteContext(ctx)
}

func newCommand(info BuildInfo) *cobra.Command {
	var modulePath string
	command := &cobra.Command{
		Use:   "new <dir>",
		Short: "Create a complete backend project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return (generate.Generator{Version: releaseVersion(info.Version), DevelopmentReplace: os.Getenv("GOBACKEND_DEVELOPMENT_REPLACE")}).New(cmd.Context(), args[0], modulePath)
		},
	}
	command.Flags().StringVar(&modulePath, "module", "", "public Go module path")
	_ = command.MarkFlagRequired("module")
	return command
}

func addCommand(info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "add <resource.yaml>",
		Short: "Copy, register, and generate a resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return (generate.Generator{Version: releaseVersion(info.Version)}).Add(cmd.Context(), ".", args[0])
		},
	}
}

func generateCommand(info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "generate",
		Short: "Deterministically regenerate all managed files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return (generate.Generator{Version: releaseVersion(info.Version)}).Generate(cmd.Context(), ".")
		},
	}
}

func checkCommand(info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Fail when generated files have drifted",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return (generate.Generator{Version: releaseVersion(info.Version)}).Check(cmd.Context(), ".")
		},
	}
}

func versionCommand(info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and build information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			version, commit, date := info.Version, info.Commit, info.Date
			if build, ok := debug.ReadBuildInfo(); ok {
				if version == "" || version == "devel" {
					version = build.Main.Version
				}
				for _, setting := range build.Settings {
					switch setting.Key {
					case "vcs.revision":
						if commit == "" {
							commit = setting.Value
						}
					case "vcs.time":
						if date == "" {
							date = setting.Value
						}
					}
				}
			}
			if version == "" {
				version = "devel"
			}
			if commit == "" {
				commit = "unknown"
			}
			if date == "" {
				date = "unknown"
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "gobackend %s commit=%s built=%s\n", version, commit, date)
			return err
		},
	}
}

func releaseVersion(version string) string {
	if version == "" || version == "devel" || version == "(devel)" {
		return "v0.1.0"
	}
	return version
}
