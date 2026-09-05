package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/MailAfrica/MailAfrica-CLI/internal/api"
	"github.com/MailAfrica/MailAfrica-CLI/internal/output"
)

func newSandboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sandbox",
		Short: "Test mail flows with a sandbox SMTP server",
		Long: `A full SMTP server (custom against this app's own sandbox) that captures
every message it receives. Point your app at the sandbox credentials, send via
SMTP AUTH, and inspect the captured messages — without paying or touching real
recipients.`,
	}
	cmd.AddCommand(
		newSandboxCredentialCmd(),
		newSandboxSMTPCmd(),
		newSandboxSMTPRegenerateCmd(),
		newSandboxMessageCmd(),
	)
	return cmd
}

func newSandboxCredentialCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "credential", Short: "Manage sandbox API credentials"}
	cmd.AddCommand(
		newSandboxCredentialCreateCmd(),
		newSandboxCredentialListCmd(),
		newSandboxCredentialRevokeCmd(),
	)
	return cmd
}

func newSandboxCredentialCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create sandbox API credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			scopes, _ := cmd.Flags().GetString("scopes")
			body := map[string]any{}
			if scopes != "" {
				body["scopes"] = scopes
			}
			var c api.SandboxCredential
			if err := e.client.Do(cmd.Context(), "POST", "/api/sandbox/credentials", body, &c); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), c)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "credential %d created — SAVE BOTH VALUES NOW\n", c.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "client id    : %s\n", c.ClientID)
			fmt.Fprintf(cmd.OutOrStdout(), "client secret: %s\n", c.ClientSecret)
			fmt.Fprintf(cmd.OutOrStdout(), "scopes       : %s\n", output.Empty(nonNil(c.Scopes)))
			return nil
		},
	}
	cmd.Flags().String("scopes", "", "comma-separated scopes (optional)")
	return cmd
}

func newSandboxCredentialListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List sandbox credentials (secrets masked)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			var creds []*api.SandboxCredential
			if err := e.client.Do(cmd.Context(), "GET", "/api/sandbox/credentials", nil, &creds); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), creds)
			}
			if len(creds) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no sandbox credentials — create one with `mailafrica sandbox credential create`")
				return nil
			}
			rows := make([][]string, 0, len(creds))
			for _, c := range creds {
				rows = append(rows, []string{
					fmt.Sprintf("%d", c.ID),
					output.SecretShort(c.ClientID),
					output.Bool(!c.Revoked),
					output.Empty(nonNil(c.Scopes)),
					fmtTime(c.CreatedAt),
				})
			}
			output.Table(cmd.OutOrStdout(), []string{"ID", "Client ID", "Active", "Scopes", "Created"}, rows)
			return nil
		},
	}
}

func newSandboxCredentialRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <id>",
		Short: "Revoke a sandbox credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if err := e.client.Do(cmd.Context(), "POST", fmt.Sprintf("/api/sandbox/credentials/%d/revoke", id), nil, nil); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "credential %d revoked\n", id)
			return nil
		},
	}
}

func renderSMTPCreds(cmd *cobra.Command, e *env, c api.SandboxSMTPCredentials) error {
	if e.jsonOut {
		return output.JSON(cmd.OutOrStdout(), c)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "host     : %s\n", c.Host)
	fmt.Fprintf(cmd.OutOrStdout(), "port     : %d\n", c.Port)
	fmt.Fprintf(cmd.OutOrStdout(), "username : %s\n", c.Username)
	if c.Password != nil && *c.Password != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "password : %s\n", *c.Password)
		fmt.Fprintln(cmd.OutOrStdout(), "SAVE THIS PASSWORD — it is shown only when generated.")
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "password : (already set — regenerate to rotate it)")
	}
	return nil
}

// newSandboxSMTPCmd emits the SMTP AUTH settings for a given sandbox profile
// (get-or-create semantics on the server).
func newSandboxSMTPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "smtp",
		Short: "Show your sandbox SMTP server credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			var c api.SandboxSMTPCredentials
			if err := e.client.Do(cmd.Context(), "GET", "/api/sandbox/credentials/smtp", nil, &c); err != nil {
				return err
			}
			return renderSMTPCreds(cmd, e, c)
		},
	}
}

func newSandboxSMTPRegenerateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "smtp-regenerate",
		Short: "Rotate the sandbox SMTP password (old password stops working)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			var c api.SandboxSMTPCredentials
			if err := e.client.Do(cmd.Context(), "POST", "/api/sandbox/credentials/smtp/regenerate", nil, &c); err != nil {
				return err
			}
			return renderSMTPCreds(cmd, e, c)
		},
	}
}

func newSandboxMessageCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "message", Short: "Inspect captured sandbox messages"}
	cmd.AddCommand(
		newSandboxMessageListCmd(),
		newSandboxMessageGetCmd(),
		newSandboxMessageClearCmd(),
	)
	return cmd
}

func newSandboxMessageListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List captured sandbox messages",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			path := "/api/sandbox/messages"
			sep := "?"
			if page, _ := cmd.Flags().GetInt("page"); page > 0 {
				path += sep + "page=" + strconv.Itoa(page)
				sep = "&"
			}
			if perPage, _ := cmd.Flags().GetInt("per-page"); perPage > 0 {
				path += sep + "per_page=" + strconv.Itoa(perPage)
			}

			var msgs []*api.SandboxMessage
			env, err := e.client.DoRaw(cmd.Context(), "GET", path, nil, &msgs)
			if err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), map[string]any{
					"messages":   msgs,
					"pagination": env.Pagination,
				})
			}
			if len(msgs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no sandbox messages — send an email via SMTP to the sandbox server")
				return nil
			}
			rows := make([][]string, 0, len(msgs))
			for _, m := range msgs {
				rows = append(rows, []string{
					fmt.Sprintf("%d", m.ID),
					output.Empty(m.From),
					output.Empty(m.To),
					output.Empty(m.Subject),
					fmtTime(m.ReceivedAt),
				})
			}
			output.Table(cmd.OutOrStdout(), []string{"ID", "From", "To", "Subject", "Received"}, rows)
			if env.Pagination != nil && env.Pagination.Total > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "page %d of %d · %d messages\n", env.Pagination.Page, env.Pagination.TotalPages, env.Pagination.Total)
			}
			return nil
		},
	}
	cmd.Flags().Int("page", 0, "page number")
	cmd.Flags().Int("per-page", 0, "messages per page")
	return cmd
}

func newSandboxMessageGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show one captured sandbox message",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			var m api.SandboxMessage
			if err := e.client.Do(cmd.Context(), "GET", fmt.Sprintf("/api/sandbox/messages/%d", id), nil, &m); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), m)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "From    : %s\n", output.Empty(m.From))
			fmt.Fprintf(cmd.OutOrStdout(), "To      : %s\n", output.Empty(m.To))
			fmt.Fprintf(cmd.OutOrStdout(), "Subject : %s\n", output.Empty(m.Subject))
			fmt.Fprintf(cmd.OutOrStdout(), "Received: %s\n", fmtTime(m.ReceivedAt))
			if m.TextBody != nil && *m.TextBody != "" {
				fmt.Fprintln(cmd.OutOrStdout(), "")
				fmt.Fprintln(cmd.OutOrStdout(), *m.TextBody)
			}
			return nil
		},
	}
}

func newSandboxMessageClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Delete all captured sandbox messages",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			if err := e.client.Do(cmd.Context(), "DELETE", "/api/sandbox/messages", nil, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "sandbox messages cleared")
			return nil
		},
	}
}
