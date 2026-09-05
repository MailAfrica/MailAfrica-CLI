// Package cmd wires cobra commands together and owns the shared CLI
// environment: config resolution, the authenticated API client, and output
// mode. Commands retrieve the environment via envFrom(cmd).
package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/MailAfrica/MailAfrica-CLI/internal/api"
	"github.com/MailAfrica/MailAfrica-CLI/internal/config"
	"github.com/MailAfrica/MailAfrica-CLI/internal/version"
)

const defaultTimeout = 60 * time.Second

// env is the per-invocation CLI environment built from global flags and the
// config file.
type env struct {
	cfg    *config.Config
	client *api.Client
	// rawAPIURL is the resolved base URL used by commands that need to display
	// it without a client round trip.
	rawAPIURL string
	jsonOut   bool
	debug     bool
}

type envKey struct{}

// NewRootCmd builds the mailafrica command tree.
func NewRootCmd(args []string) *cobra.Command {
	root := &cobra.Command{
		Use:   "mailafrica",
		Short: "MailAfrica CLI — email infrastructure for Africa",
		Long: `mailafrica is a user-side command-line client for the MailAfrica
email infrastructure API: inbound receiving addresses, webhooks, transactional
sending, wallet billing, and AI auto-reply configuration — all from your
terminal. It ships no admin/operator commands.

Credentials live in ~/.config/mailafrica/config.json with owner-only
permissions. Set MAILAFRICA_API_URL and MAILAFRICA_API_KEY to override
configuration for the current process.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Bare `mailafrica` prints help; any stray argument is an unknown
			// subcommand, which NoArgs rejects with a non-zero exit.
			return cmd.Help()
		},
		Version: version.Version,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			e, err := buildEnv(cmd)
			if err != nil {
				return err
			}
			cmd.SetContext(context.WithValue(cmd.Context(), envKey{}, e))
			return nil
		},
	}

	root.PersistentFlags().String("api-url", "", "API base URL (env MAILAFRICA_API_URL)")
	root.PersistentFlags().String("api-key", "", "API key to use for this invocation (env MAILAFRICA_API_KEY)")
	root.PersistentFlags().Bool("json", false, "render output as JSON")
	root.PersistentFlags().Bool("debug", false, "print redacted request/response exchange to stderr")

	root.SetArgs(args)
	root.SetVersionTemplate("mailafrica {{.Version}}\n")

	root.AddCommand(
		newVersionCmd(),
		newConfigCmd(),
		newAuthCmd(),
		newAPIKeysCmd(),
		newInboundCmd(),
		newWebhookCmd(),
		newSendCmd(),
		newDomainCmd(),
		newSandboxCmd(),
	)
	return root
}

// buildEnv resolves global flags against the config file and constructs the
// API client.
func buildEnv(cmd *cobra.Command) (*env, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	apiURL, _ := cmd.Flags().GetString("api-url")
	if apiURL == "" {
		apiURL = cfg.EffectiveAPIURL()
	}
	jsonOut, _ := cmd.Flags().GetBool("json")
	debug, _ := cmd.Flags().GetBool("debug")

	cl := api.New(apiURL, cfg, &http.Client{Timeout: defaultTimeout})
	cl.Debug = debug
	cl.DebugOut = os.Stderr
	if k, _ := cmd.Flags().GetString("api-key"); k != "" {
		cl.SetAPIKey(k)
	}

	return &env{
		cfg:       cfg,
		client:    cl,
		rawAPIURL: apiURL,
		jsonOut:   jsonOut,
		debug:     debug,
	}, nil
}

// envFrom retrieves the shared environment injected by PersistentPreRunE.
func envFrom(cmd *cobra.Command) *env {
	e, ok := cmd.Context().Value(envKey{}).(*env)
	if !ok {
		// Persistently defensive: all commands go through PersistentPreRunE.
		e, _ = buildEnv(cmd)
		cmd.SetContext(context.WithValue(cmd.Context(), envKey{}, e))
	}
	return e
}
