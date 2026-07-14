// Package app composes and runs the aws-cost-exporter process.
package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/logging"
	"github.com/sakuya1998/aws-cost-exporter/internal/version"
)

type runtimeFunc func(context.Context, config.Config, *slog.Logger) error

// Execute parses CLI arguments and starts the production runtime.
func Execute(ctx context.Context, args []string, output, errorOutput io.Writer) error {
	return execute(ctx, args, output, errorOutput, Run)
}

func execute(ctx context.Context, args []string, output, errorOutput io.Writer, runtime runtimeFunc) error {
	var path, listenAddress, logLevel string
	var checkConfig bool
	command := &cobra.Command{
		Use: "aws-cost-exporter", Short: "Export AWS costs to Prometheus",
		Version: version.Current().String(), SilenceErrors: true, SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			overrides := make(map[string]any)
			if command.Flags().Changed("listen-address") {
				overrides["server.listen_address"] = listenAddress
			}
			if command.Flags().Changed("log-level") {
				overrides["log.level"] = logLevel
			}
			value, err := config.Load(config.Options{Path: path, Overrides: overrides})
			if err != nil {
				return err
			}
			if len(enabledCollectors(value)) == 0 {
				return fmt.Errorf("cost_explorer: no collectors enabled")
			}
			logger, err := logging.New(value.Log, errorOutput)
			if err != nil {
				return err
			}
			if checkConfig {
				_, err = fmt.Fprintln(output, "configuration valid")
				return err
			}
			return runtime(ctx, value, logger)
		},
	}
	command.SetVersionTemplate("{{.Version}}\n")
	command.SetOut(output)
	command.SetErr(errorOutput)
	command.SetArgs(args)
	flags := command.Flags()
	flags.StringVar(&path, "config", "", "path to YAML configuration")
	flags.StringVar(&listenAddress, "listen-address", "", "HTTP listen address")
	flags.StringVar(&logLevel, "log-level", "", "log level")
	flags.BoolVar(&checkConfig, "check-config", false, "validate configuration and exit")
	return command.Execute()
}
