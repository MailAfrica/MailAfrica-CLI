package cmd

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/MailAfrica/MailAfrica-CLI/internal/api"
	"github.com/MailAfrica/MailAfrica-CLI/internal/output"
)

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Turn an inbound address into an AI auto-responder",
		Long: `The strategic MailAfrica feature: wire an inbound address up to an AI agent
that answers your customers automatically, configured by the message mode:

  off    no auto-reply (the default)
  draft  the agent drafts replies for you to review — never auto-sends
  auto   the agent reads incoming mail and replies on its own

Config lives on the platform (shared with the web app), so agent behavior is
consistent no matter which client turns the address on. The reply-from fields
need a verified sending domain you own (see 'mailafrica domain').`,
	}
	cmd.AddCommand(
		newAgentListCmd(),
		newAgentConfigCmd(),
		newAgentDraftCmd(),
	)
	return cmd
}

func renderAgentConfig(cmd *cobra.Command, e *env, c *api.AgentConfig) error {
	if e.jsonOut {
		return output.JSON(cmd.OutOrStdout(), c)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "address id   : %d\n", c.AddressID)
	fmt.Fprintf(cmd.OutOrStdout(), "mode         : %s\n", c.Mode)
	fmt.Fprintf(cmd.OutOrStdout(), "enabled      : %t\n", c.Enabled)
	if c.Persona != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "persona      : %s\n", *c.Persona)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "persona      : (default assistant persona)")
	}
	if c.ReplyFromDomainID != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "reply from   : domain %d (%s)\n", *c.ReplyFromDomainID, output.EmptyPtr(c.ReplyFromAddress))
	} else if c.ReplyFromAddress != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "reply from   : %s\n", *c.ReplyFromAddress)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "updated      : %s\n", fmtTime(c.UpdatedAt))
	if c.Mode == "auto" {
		fmt.Fprintln(cmd.OutOrStdout(), "mode auto: the agent sends replies automatically.")
	}
	return nil
}

func newAgentListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Which addresses have auto-reply configured, and their modes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			var configs []*api.AgentConfig
			if err := e.client.Do(cmd.Context(), "GET", "/api/agent/configs", nil, &configs); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), configs)
			}
			if len(configs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no addresses configured — turn one on with `mailafrica agent config <address-id> --mode auto`")
				return nil
			}
			rows := make([][]string, 0, len(configs))
			for _, c := range configs {
				rows = append(rows, []string{
					fmt.Sprintf("%d", c.AddressID),
					c.Mode,
					output.Bool(c.Enabled),
					output.EmptyPtr(c.ReplyFromAddress),
					fmtTime(c.UpdatedAt),
				})
			}
			output.Table(cmd.OutOrStdout(), []string{"Address", "Mode", "Enabled", "Reply from", "Updated"}, rows)
			return nil
		},
	}
}

// newAgentConfigCmd reads or writes one address's auto-reply config. With no
// config flags it is a read (GET); pass --mode/--persona/... to update (PUT).
func newAgentConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config <address-id>",
		Short: "Read or update the auto-reply config for an inbound address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if !anyAgentFlagChanged(cmd) {
				var c api.AgentConfig
				if err := e.client.Do(cmd.Context(), "GET", fmt.Sprintf("/api/agent/configs/%d", id), nil, &c); err != nil {
					return err
				}
				return renderAgentConfig(cmd, e, &c)
			}

			upd := api.AgentConfigUpdate{}
			if cmd.Flags().Changed("mode") {
				m, _ := cmd.Flags().GetString("mode")
				switch m {
				case "off", "draft", "auto":
				default:
					return errors.New("--mode must be off, draft, or auto")
				}
				upd.Mode = m
			} else {
				return errors.New("--mode is required when updating (off|draft|auto)")
			}
			if cmd.Flags().Changed("persona") {
				p, _ := cmd.Flags().GetString("persona")
				upd.Persona = &p
			}
			if cmd.Flags().Changed("enabled") {
				en, _ := cmd.Flags().GetBool("enabled")
				upd.Enabled = &en
			}
			if cmd.Flags().Changed("reply-from-domain-id") {
				d, _ := cmd.Flags().GetString("reply-from-domain-id")
				did, err := strconv.ParseInt(d, 10, 64)
				if err != nil || did <= 0 {
					return errors.New("--reply-from-domain-id must be a positive integer (see `mailafrica domain list`)")
				}
				upd.ReplyFromDomainID = &did
			}
			if cmd.Flags().Changed("reply-from-address") {
				a, _ := cmd.Flags().GetString("reply-from-address")
				upd.ReplyFromAddress = &a
			}

			var c api.AgentConfig
			if err := e.client.Do(cmd.Context(), "PUT", fmt.Sprintf("/api/agent/configs/%d", id), upd, &c); err != nil {
				return err
			}
			if err := renderAgentConfig(cmd, e, &c); err != nil {
				return err
			}
			if c.Mode != "off" && c.ReplyFromDomainID == nil && c.ReplyFromAddress == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "note: replies will use the platform sender until you set --reply-from-domain-id / --reply-from-address.")
			}
			return nil
		},
	}
	cmd.Flags().String("mode", "", "off | draft | auto (required to update)")
	cmd.Flags().String("persona", "", "system prompt overriding the default assistant persona")
	cmd.Flags().Bool("enabled", false, "master switch (default true when omitted)")
	cmd.Flags().String("reply-from-domain-id", "", "verified sending domain the replies go out from")
	cmd.Flags().String("reply-from-address", "", "From identity for replies (must be on the chosen domain)")
	return cmd
}

func anyAgentFlagChanged(cmd *cobra.Command) bool {
	for _, n := range []string{"mode", "persona", "enabled", "reply-from-domain-id", "reply-from-address"} {
		if cmd.Flags().Changed(n) {
			return true
		}
	}
	return false
}

// newAgentDraftCmd produces a one-off reply preview through MailAfrica. It
// never sends — the result is written to stdout for review.
func newAgentDraftCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "draft <address-id>",
		Short: "Preview a reply draft the agent would send (never sends)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			subject, _ := cmd.Flags().GetString("subject")
			text, _ := cmd.Flags().GetString("text-body")
			if subject == "" && text == "" {
				return errors.New("pass --subject and/or --text-body to draft a reply against")
			}
			var resp api.AgentDraftResponse
			if err := e.client.Do(cmd.Context(), "POST", fmt.Sprintf("/api/agent/configs/%d/draft", id),
				api.AgentDraftRequest{Subject: subject, TextBody: text}, &resp); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), resp)
			}
			fmt.Fprintln(cmd.OutOrStdout(), resp.Draft)
			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintln(cmd.OutOrStdout(), "(draft preview — nothing was sent)")
			return nil
		},
	}
	cmd.Flags().String("subject", "", "the incoming message subject")
	cmd.Flags().String("text-body", "", "the incoming message body")
	return cmd
}
