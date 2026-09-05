package cmd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/MailAfrica/MailAfrica-CLI/internal/config"
)

// newConfigCmd exposes the safe subset of configuration: the API URL and the
// config file path. Credential keys can only report *whether* they are set,
// never reveal their value — a stored key is read back only by the API client.
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect configuration",
		Long: `Inspect configuration. Precedence: flags > environment
(MAILAFRICA_API_URL, MAILAFRICA_API_KEY) > ~/.config/mailafrica/config.json.

Secrets are never printed. Credential keys report only "set" or "not set".
Use  mailafrica auth login  to manage the stored session, and
mailafrica apikeys create --save  to store an API key.`,
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "path",
			Short: "Print the config file location",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, _ []string) {
				fmt.Fprintln(cmd.OutOrStdout(), config.Path())
			},
		},
		newConfigGetCmd(),
		newConfigSetCmd(),
	)
	return cmd
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Show a configuration value (secrets report set/unset only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			switch strings.ToLower(args[0]) {
			case "api-url":
				fmt.Fprintln(cmd.OutOrStdout(), e.rawAPIURL)
				return nil
			case "api-key", "refresh-token":
				set := ""
				if args[0] == "api-key" && e.cfg.EffectiveAPIKey() != "" {
					set = "set"
				} else if args[0] == "refresh-token" && e.cfg.GetRefreshToken() != "" {
					set = "set"
				}
				if set == "" {
					set = "not set"
				}
				fmt.Fprintln(cmd.OutOrStdout(), set)
				return nil
			default:
				return fmt.Errorf("unknown key %q (valid: api-url, api-key, refresh-token)", args[0])
			}
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value (api-url only)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			switch strings.ToLower(args[0]) {
			case "api-url":
				u := args[1]
				if !strings.Contains(u, "://") {
					u = "https://" + u
				}
				parsed, err := url.Parse(u)
				if err != nil || parsed.Scheme == "" || parsed.Host == "" {
					return fmt.Errorf("invalid URL %q", args[1])
				}
				e.cfg.APIURL = strings.TrimRight(parsed.String(), "/")
				if err := e.cfg.Save(); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "api-url set to %s\n", e.cfg.APIURL)
				return nil
			case "api-key":
				return fmt.Errorf("api-key is not configurable via `config set` — use `mailafrica apikeys create --save` or the %s environment variable", config.EnvAPIKey)
			case "refresh-token":
				return fmt.Errorf("refresh-token is not configurable via `config set` — use `mailafrica auth login`")
			default:
				return fmt.Errorf("unknown key %q (only api-url is settable)", args[0])
			}
		},
	}
}
