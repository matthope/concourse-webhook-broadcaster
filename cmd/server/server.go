package server

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/creasty/defaults"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.szostok.io/version/extension"

	"github.com/matthope/concourse-webhook-broadcaster/internal/server"
)

func Execute(ctx context.Context) error {
	params := &server.Params{}

	if err := defaults.Set(params); err != nil {
		panic(err)
	}

	rootCmd := &cobra.Command{
		Use:   "concourse-webhook-broadcaster",
		Short: "Webhook receiver from VCS hosts to Concourse",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(os.Stderr, "Params: %+#v\n", params)

			if err := params.IsValid(); err != nil {
				return err
			}

			if err := server.Run(cmd.Context(), params, server.NewLogger(params.Debug)); err != nil {
				return err
			}

			return nil
		},
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			initEnvFlags(cmd, "BCAST")
		},
		CompletionOptions: cobra.CompletionOptions{HiddenDefaultCmd: true},
	}

	rootCmd.AddCommand(
		extension.NewVersionCobraCmd(),
	)

	rootCmd.Flags().StringVar(&params.ExtListenAddr, "ext-listen-addr", params.ExtListenAddr, "Listen address of webhook ingester")
	rootCmd.Flags().StringVar(&params.IntListenAddr, "int-listen-addr", params.IntListenAddr, "Listen address of metrics")
	rootCmd.Flags().DurationVar(&params.RefreshInterval, "refresh-interval", params.RefreshInterval, "Resource refresh interval")
	rootCmd.Flags().StringSliceVar(&params.ConcourseURL, "concourse-url", params.ConcourseURL, "External URL of Concourse API, in https://user:pass@host/ format")
	rootCmd.Flags().IntVar(&params.WebhookConcurrency, "webhook-concurrency", params.WebhookConcurrency, "How many resources to notify in parallel")
	rootCmd.Flags().BoolVar(&params.Debug, "debug", params.Debug, "Debugging")

	return rootCmd.ExecuteContext(ctx)
}

func initEnvFlags(cmd *cobra.Command, envPrefix string) {
	v := viper.New()

	v.SetConfigName("concourse-webhook-broadcaster")
	v.SetEnvPrefix(envPrefix)

	v.AutomaticEnv()

	bindFlags(cmd, v, envPrefix)
}

func bindFlags(cmd *cobra.Command, v *viper.Viper, envPrefix string) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// Environment variables can't have dashes in them, so bind them to
		// their equivalent keys with underscores, e.g. --favorite-color to
		// STING_FAVORITE_COLOR
		if strings.Contains(f.Name, "-") {
			envVarSuffix := strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))

			err := v.BindEnv(f.Name, fmt.Sprintf("%s_%s", envPrefix, envVarSuffix))
			if err != nil {
				panic(err.Error())
			}
		}

		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)

			err := cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val))
			if err != nil {
				panic(err.Error())
			}
		}
	})
}
