package cmd

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/MailAfrica/MailAfrica-CLI/internal/api"
	"github.com/MailAfrica/MailAfrica-CLI/internal/output"
)

func newAPIKeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apikeys",
		Short: "Manage developer API keys (MAIL_…)",
		Long: `Manage developer API keys for programmatic access. The plaintext
MAIL_… key is returned by the server exactly once, at creation, so create shows
it and that is the only time it is ever displayed.`,
	}
	cmd.AddCommand(
		newAPIKeyCreateCmd(),
		newAPIKeyListCmd(),
		newAPIKeyRevokeCmd(),
	)
	return cmd
}

func newAPIKeyCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an API key",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			name, _ := cmd.Flags().GetString("name")
			if name == "" {
				return errors.New("--name is required")
			}
			scopes, _ := cmd.Flags().GetString("scopes")
			expires, _ := cmd.Flags().GetString("expires-at")

			body := map[string]any{"name": name, "scopes": scopes}
			if expires != "" {
				body["expires_at"] = expires
			}

			var resp api.CreateAPIKeyResponse
			if err := e.client.Do(cmd.Context(), "POST", "/api/apikeys/", body, &resp); err != nil {
				return err
			}

			save, _ := cmd.Flags().GetBool("save")
			if save {
				if err := e.cfg.SetAPIKey(resp.Key); err != nil {
					return fmt.Errorf("store api key: %w", err)
				}
			}

			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), resp)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "API key created — THIS IS THE ONLY TIME IT WILL BE SHOWN. Save it now.")
			fmt.Fprintf(cmd.OutOrStdout(), "name : %s\n", resp.APIKey.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "key  : %s\n", resp.Key)
			if save {
				fmt.Fprintln(cmd.OutOrStdout(), "stored: saved to", "config file", "(chmod 0600)")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "tip  : pass --save to store this key for future CLI use")
			}
			return nil
		},
	}
	cmd.Flags().String("name", "", "label for the key")
	cmd.Flags().String("scopes", "full", "scopes (default full)")
	cmd.Flags().String("expires-at", "", "RFC3339 expiry, e.g. 2027-01-01T00:00:00Z (optional)")
	cmd.Flags().Bool("save", false, "store the created key in the config file (chmod 0600)")
	return cmd
}

func newAPIKeyListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List active API keys",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			var keys []*api.APIKey
			if err := e.client.Do(cmd.Context(), "GET", "/api/apikeys/", nil, &keys); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), keys)
			}
			if len(keys) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no API keys — create one with `mailafrica apikeys create`")
				return nil
			}
			rows := make([][]string, 0, len(keys))
			for _, k := range keys {
				rows = append(rows, []string{
					fmt.Sprintf("%d", k.ID),
					output.Empty(k.Name),
					output.Empty(k.KeyPrefix),
					output.Empty(k.Scopes),
					fmtTime(k.CreatedAt),
					fmtTimePtr(k.ExpiresAt, "never"),
				})
			}
			output.Table(cmd.OutOrStdout(), []string{"ID", "Name", "Key prefix", "Scopes", "Created", "Expires"}, rows)
			return nil
		},
	}
}

func newAPIKeyRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <id>",
		Short: "Revoke an API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return errors.New("id must be an integer")
			}
			if err := e.client.Do(cmd.Context(), "DELETE", fmt.Sprintf("/api/apikeys/%d", id), nil, nil); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "API key %d revoked\n", id)
			return nil
		},
	}
}
