// Package cli implements the gobackend command-line interface.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/alphayan/go-backend-kit/internal/generate"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

func New(info BuildInfo, stdout, stderr io.Writer) *cobra.Command {
	if build, ok := debug.ReadBuildInfo(); ok {
		info.Version = buildVersion(info.Version, build.Main.Version, vcsModified(build))
	}
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
	root.AddCommand(newCommand(info), addCommand(info), generateCommand(info), checkCommand(info), upgradeCommand(info), versionCommand(info))
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
	}
	command.Flags().StringVar(&modulePath, "module", "", "public Go module path")
	_ = command.MarkFlagRequired("module")
	httpChoice := string(generate.HTTPEcho)
	databaseChoice := string(generate.DatabaseSQLite)
	cacheChoice := string(generate.CacheNone)
	messagingChoice := string(generate.MessagingNone)
	loggingChoice := string(generate.LoggingSlog)
	authChoice := string(generate.AuthNone)
	profileChoice := string(generate.ProfilePersonal)
	registerChoiceFlag(command, "http", &httpChoice, string(generate.HTTPEcho), "HTTP framework", []string{"echo", "fiber"})
	registerChoiceFlag(command, "database", &databaseChoice, string(generate.DatabaseSQLite), "database (production defaults to postgres)", []string{"sqlite", "postgres"})
	registerChoiceFlag(command, "cache", &cacheChoice, string(generate.CacheNone), "cache", []string{"none", "redis"})
	registerChoiceFlag(command, "messaging", &messagingChoice, string(generate.MessagingNone), "messaging", []string{"none", "nats"})
	registerChoiceFlag(command, "logging", &loggingChoice, string(generate.LoggingSlog), "logging backend", []string{"slog", "zap", "zerolog"})
	registerChoiceFlag(command, "auth", &authChoice, string(generate.AuthNone), "authentication", generate.AuthChoices())
	registerChoiceFlag(command, "profile", &profileChoice, string(generate.ProfilePersonal), "project profile", []string{"personal", "production"})
	command.RunE = func(cmd *cobra.Command, args []string) error {
		generator, err := scaffoldGenerator(info)
		if err != nil {
			return err
		}
		database := generate.DatabaseChoice(databaseChoice)
		if profileChoice == string(generate.ProfileProduction) && !cmd.Flags().Changed("database") {
			database = generate.DatabasePostgres
		}
		return generator.New(cmd.Context(), args[0], modulePath, generate.ProjectOptions{
			HTTP:      generate.HTTPChoice(httpChoice),
			Database:  database,
			Cache:     generate.CacheChoice(cacheChoice),
			Messaging: generate.MessagingChoice(messagingChoice),
			Logging:   generate.LoggingChoice(loggingChoice),
			Auth:      generate.AuthChoice(authChoice),
			Profile:   generate.ProfileChoice(profileChoice),
		})
	}
	return command
}

func registerChoiceFlag(command *cobra.Command, name string, target *string, defaultValue, usage string, values []string) {
	command.Flags().StringVar(target, name, defaultValue, usage+" ("+strings.Join(values, "|")+")")
	_ = command.RegisterFlagCompletionFunc(name, cobra.FixedCompletions(values, cobra.ShellCompDirectiveNoFileComp))
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

func upgradeCommand(info BuildInfo) *cobra.Command {
	var options generate.UpgradeOptions
	command := &cobra.Command{
		Use:   "upgrade",
		Short: "Preview scaffold upgrades; apply explicitly after reviewing conflicts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			generator, err := scaffoldGenerator(info)
			if err != nil {
				return err
			}
			report, err := generator.Upgrade(cmd.Context(), ".", options)
			for _, change := range report.Changes {
				if _, writeErr := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", change.Action, change.Path); writeErr != nil {
					return writeErr
				}
			}
			if report.Directory != "" {
				if _, writeErr := fmt.Fprintf(cmd.OutOrStdout(), "Review/recovery directory: %s\n", report.Directory); writeErr != nil {
					return writeErr
				}
			}
			if err != nil {
				return err
			}
			if report.Applied {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Upgrade applied. Review go.mod, regenerate/check, test, and migrate explicitly before deployment.")
			} else {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Preview only; project source files were not changed. Use --apply after review.")
			}
			return err
		},
	}
	command.Flags().BoolVar(&options.Apply, "apply", false, "apply a conflict-free upgrade with original-file backups")
	command.Flags().StringVar(&options.Baseline, "baseline", "", "verified pristine old project for projects without a scaffold baseline")
	command.Flags().StringSliceVar(&options.Keep, "keep", nil, "explicitly retain each manually merged scaffold path and advance its upstream baseline")
	return command
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
		return generate.CurrentVersion
	}
	return version
}

// go install @<tag|pushed commit> records a usable module version without
// ldflags. A build from a modified checkout (vcs.modified, "+dirty") is a source
// build: its stamped version names a different, already-published commit.
func buildVersion(explicit, module string, modified bool) string {
	if explicit == "" || explicit == "devel" || explicit == "(devel)" {
		if semver.IsValid(module) && semver.Build(module) == "" && !modified {
			return module
		}
	}
	return explicit
}

func vcsModified(build *debug.BuildInfo) bool {
	for _, setting := range build.Settings {
		if setting.Key == "vcs.modified" {
			return setting.Value == "true"
		}
	}
	return false
}

func scaffoldGenerator(info BuildInfo) (generate.Generator, error) {
	replace := os.Getenv("GOBACKEND_DEVELOPMENT_REPLACE")
	if !semver.IsValid(info.Version) && replace == "" {
		return generate.Generator{}, errors.New("this generator was built from source or a modified checkout; set GOBACKEND_DEVELOPMENT_REPLACE=/absolute/path/to/go-backend-kit, or install a published tag or pushed commit with go install")
	}
	return generate.Generator{Version: releaseVersion(info.Version), DevelopmentReplace: replace}, nil
}
