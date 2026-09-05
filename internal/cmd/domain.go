package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/MailAfrica/MailAfrica-CLI/internal/api"
	"github.com/MailAfrica/MailAfrica-CLI/internal/output"
)

func newDomainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "domain",
		Short: "Manage verified sending domains (DKIM/SPF/DMARC)",
		Long: `Sending domains let you sign outbound mail with your own DKIM key. After
adding a domain you must publish the three DNS records (DKIM, SPF, DMARC) it
returns; once verified the local Postfix MTA will sign and relay for it. No
SMTP credentials are needed — the platform manages DKIM keys internally.`,
	}
	cmd.AddCommand(
		newDomainAddCmd(),
		newDomainListCmd(),
		newDomainVerifyCmd(),
		newDomainDeleteCmd(),
		newDomainSenderCmd(),
	)
	return cmd
}

func newDomainAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a verified sending domain (returns the DNS records to publish)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			domain, _ := cmd.Flags().GetString("domain")
			if domain == "" {
				return errors.New("--domain is required (e.g. mail.example.com)")
			}
			from, _ := cmd.Flags().GetString("from-local-part")

			req := api.AddSendingDomainRequest{Domain: domain}
			if from != "" {
				req.FromLocalPart = from
			}

			var resp api.AddSendingDomainResponse
			if err := e.client.Do(cmd.Context(), "POST", "/api/domains", req, &resp); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), resp)
			}
			d := resp.Domain
			fmt.Fprintf(cmd.OutOrStdout(), "domain added: %s (id %d) — status %s\n", d.Domain, d.ID, d.Status)
			fmt.Fprintf(cmd.OutOrStdout(), "from local part: %s\n", d.FromLocalPart)
			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintln(cmd.OutOrStdout(), "Publish these DNS records, then run:")
			fmt.Fprintln(cmd.OutOrStdout(), "  mailafrica domain verify "+fmt.Sprintf("%d", d.ID))
			records := api.ParseDNSRecords(resp.DNSRecords)
			printDNSRecords(cmd, &records)
			return nil
		},
	}
	cmd.Flags().String("domain", "", "domain name to send from (e.g. mail.example.com)")
	cmd.Flags().String("from-local-part", "", "default From local part (default: noreply)")
	return cmd
}

func newDomainListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List verified sending domains and verification status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			var domains []*api.SendingDomain
			if err := e.client.Do(cmd.Context(), "GET", "/api/domains", nil, &domains); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), domains)
			}
			if len(domains) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no sending domains — add one with `mailafrica domain add --domain <name>`")
				return nil
			}
			rows := make([][]string, 0, len(domains))
			for _, d := range domains {
				rows = append(rows, []string{
					fmt.Sprintf("%d", d.ID),
					output.Empty(d.Domain),
					d.Status,
					output.Empty(d.FromLocalPart),
					fmtTimePtr(d.VerifiedAt, "-"),
					fmtTime(d.CreatedAt),
				})
			}
			output.Table(cmd.OutOrStdout(), []string{"ID", "Domain", "Status", "From", "Verified", "Added"}, rows)
			return nil
		},
	}
}

func newDomainVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify <id>",
		Short: "Re-check a sending domain's DKIM/SPF/DMARC records",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			var d api.SendingDomain
			if err := e.client.Do(cmd.Context(), "POST", fmt.Sprintf("/api/domains/%d/verify", id), nil, &d); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), d)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", d.Domain, d.Status)
			switch d.Status {
			case "verified":
				fmt.Fprintln(cmd.OutOrStdout(), "DKIM / SPF / DMARC confirmed — this domain is ready to send")
			default:
				fmt.Fprintln(cmd.OutOrStdout(), "not all DNS records are published yet — check the TXT/MX records at your DNS provider and retry")
			}
			return nil
		},
	}
}

func newDomainDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a sending domain (stops outbound signing for it)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if err := e.client.Do(cmd.Context(), "DELETE", fmt.Sprintf("/api/domains/%d", id), nil, nil); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "domain %d deleted\n", id)
			return nil
		},
	}
}

func newDomainSenderCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "sender", Short: "Manage From identities on verified domains"}
	cmd.AddCommand(
		newSenderCreateCmd(),
		newSenderListCmd(),
		newSenderDeleteCmd(),
	)
	return cmd
}

func newSenderCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new From identity on a verified sending domain",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			domainIDStr, _ := cmd.Flags().GetString("domain-id")
			if domainIDStr == "" {
				return errors.New("--domain-id is required (see `mailafrica domain list`)")
			}
			domainID, err := strconv.ParseInt(domainIDStr, 10, 64)
			if err != nil {
				return errors.New("--domain-id must be an integer")
			}
			localPart, _ := cmd.Flags().GetString("local-part")
			if localPart == "" {
				return errors.New("--local-part is required (the email name before @)")
			}

			var sa api.SenderAddress
			if err := e.client.Do(cmd.Context(), "POST", fmt.Sprintf("/api/domains/%d/senders", domainID),
				api.SenderAddressRequest{LocalPart: localPart}, &sa); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), sa)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "sender created: %s@%s (id %d)\n", sa.LocalPart, sa.Domain, sa.ID)
			return nil
		},
	}
	cmd.Flags().String("domain-id", "", "sending domain to create the identity on (required)")
	cmd.Flags().String("local-part", "", "email local part (required)")
	return cmd
}

func newSenderListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all From identities across your sending domains",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			var senders []*api.SenderAddress
			if err := e.client.Do(cmd.Context(), "GET", "/api/domains/senders", nil, &senders); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), senders)
			}
			if len(senders) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no sender addresses — create one with `mailafrica domain sender create --domain-id <id> --local-part <lp>`")
				return nil
			}
			rows := make([][]string, 0, len(senders))
			for _, s := range senders {
				rows = append(rows, []string{
					fmt.Sprintf("%d", s.ID),
					fmt.Sprintf("%s@%s", s.LocalPart, s.Domain),
					fmt.Sprintf("%d", s.DomainID),
					fmtTime(s.CreatedAt),
				})
			}
			output.Table(cmd.OutOrStdout(), []string{"ID", "Sender", "Domain ID", "Created"}, rows)
			return nil
		},
	}
}

func newSenderDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a From identity",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if err := e.client.Do(cmd.Context(), "DELETE", fmt.Sprintf("/api/domains/senders/%d", id), nil, nil); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "sender %d deleted\n", id)
			return nil
		},
	}
}

func printDNSRecords(cmd *cobra.Command, recs *api.DNSRecords) {
	output.Table(cmd.OutOrStdout(), []string{"Type", "Host", "Value"}, [][]string{
		{"DKIM", recs.DKIM.Host, recs.DKIM.Value},
		{"SPF", recs.SPF.Host, recs.SPF.Value},
		{"DMARC", recs.DMARC.Host, recs.DMARC.Value},
	})
	for _, u := range recs.Unrecognized {
		fmt.Fprintf(cmd.OutOrStdout(), "unknown record type %q on host %q — publish this manually: %s\n", u.Type, u.Host, strings.TrimSpace(u.Value))
	}
}
