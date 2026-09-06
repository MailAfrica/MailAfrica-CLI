package cmd

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/MailAfrica/MailAfrica-CLI/internal/api"
	"github.com/MailAfrica/MailAfrica-CLI/internal/output"
)

var localPartRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

func newInboundCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inbound",
		Short: "Handle inbound receiving addresses, domains, and mail",
		Long: `Inbound email plumbing: receiving addresses (mailbox@…), the domains
they sit on, and the received messages. Mail arriving at an address is parsed
and delivered to the webhooks wired to that address.`,
	}
	cmd.AddCommand(
		newInboundAddressCmd(),
		newInboundDomainCmd(),
		newInboundMessageCmd(),
	)
	return cmd
}

func newInboundAddressCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "address", Short: "Manage receiving addresses"}
	cmd.AddCommand(
		newInboundAddressCreateCmd(),
		newInboundAddressListCmd(),
		newInboundAddressDeleteCmd(),
	)
	return cmd
}

func newInboundAddressCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a receiving address (local_part@domain)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			local, _ := cmd.Flags().GetString("local-part")
			if !localPartRe.MatchString(local) {
				return errors.New("--local-part must match ^[a-z0-9][a-z0-9-]{0,63}$ (lowercase letters, digits, hyphens)")
			}
			label, _ := cmd.Flags().GetString("label")
			domainIDStr, _ := cmd.Flags().GetString("domain-id")

			req := api.CreateInboundAddressRequest{LocalPart: local}
			if cmd.Flags().Changed("label") {
				req.Label = &label
			}
			if domainIDStr != "" {
				id, err := strconv.ParseInt(domainIDStr, 10, 64)
				if err != nil {
					return errors.New("--domain-id must be an integer")
				}
				req.DomainID = &id
			}

			var addr api.InboundAddress
			if err := e.client.Do(cmd.Context(), "POST", "/api/inbound/addresses/", req, &addr); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), addr)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "address created: %d\n", addr.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "mailbox         : %s\n", mailboxName(&addr))
			fmt.Fprintf(cmd.OutOrStdout(), "retention       : %d days\n", addr.RetentionDays)
			return nil
		},
	}
	cmd.Flags().String("local-part", "", "the part before @ (required)")
	cmd.Flags().String("label", "", "friendly label (optional)")
	cmd.Flags().String("domain-id", "", "custom inbound domain id (optional; defaults to the platform @mailafrica.online domain)")
	return cmd
}

func newInboundAddressListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List receiving addresses",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			var addrs []*api.InboundAddress
			if err := e.client.Do(cmd.Context(), "GET", "/api/inbound/addresses/", nil, &addrs); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), addrs)
			}
			if len(addrs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no receiving addresses — create one with `mailafrica inbound address create --local-part <name>`")
				return nil
			}
			rows := make([][]string, 0, len(addrs))
			for _, a := range addrs {
				rows = append(rows, []string{
					fmt.Sprintf("%d", a.ID),
					mailboxName(a),
					output.Empty(nonNil(a.Label)),
					fmt.Sprintf("%d", a.RetentionDays),
					fmtTime(a.CreatedAt),
				})
			}
			output.Table(cmd.OutOrStdout(), []string{"ID", "Mailbox", "Label", "Retention (days)", "Created"}, rows)
			return nil
		},
	}
}

func newInboundAddressDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a receiving address (mail stops being received)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if err := e.client.Do(cmd.Context(), "DELETE", fmt.Sprintf("/api/inbound/addresses/%d", id), nil, nil); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "address %d deleted\n", id)
			return nil
		},
	}
}

// mailboxName renders the full mailbox address for an inbound address. The
// platform domain (@mailafrica.online) applies when no custom domain is set.
func mailboxName(a *api.InboundAddress) string {
	if a.DomainID != nil {
		return fmt.Sprintf("%s@domain-%d", a.LocalPart, *a.DomainID)
	}
	return a.LocalPart + "@mailafrica.online"
}

func newInboundDomainCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "domain", Short: "Manage custom inbound domains"}
	cmd.AddCommand(
		newInboundDomainAddCmd(),
		newInboundDomainListCmd(),
		newInboundDomainVerifyCmd(),
		newInboundDomainDeleteCmd(),
	)
	return cmd
}

func newInboundDomainAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a custom inbound domain and get the DNS records to publish",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			domain, _ := cmd.Flags().GetString("domain")
			if domain == "" {
				return errors.New("--domain is required")
			}
			var resp api.CreateInboundDomainResponse
			if err := e.client.Do(cmd.Context(), "POST", "/api/inbound/domains/", api.CreateInboundDomainRequest{Domain: domain}, &resp); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), resp)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "domain added: %s (id %d)\n", resp.Domain.Domain, resp.Domain.ID)
			fmt.Fprintln(cmd.OutOrStdout(), "verification token:", resp.Domain.VerificationToken)
			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintln(cmd.OutOrStdout(), "Publish these DNS records, then run:")
			fmt.Fprintln(cmd.OutOrStdout(), "  mailafrica inbound domain verify "+fmt.Sprintf("%d", resp.Domain.ID))
			records := resp.VerificationRecords
			if len(records) == 0 && resp.VerificationRecord.Type != "" {
				records = []api.VerificationRecord{resp.VerificationRecord}
			}
			if len(records) > 0 {
				rows := make([][]string, 0, len(records))
				for _, r := range records {
					rows = append(rows, []string{r.Type, r.Host, r.Value})
				}
				output.Table(cmd.OutOrStdout(), []string{"Type", "Host", "Value"}, rows)
			}
			return nil
		},
	}
	cmd.Flags().String("domain", "", "the domain to receive mail on (e.g. mail.example.com)")
	return cmd
}

func newInboundDomainListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List custom inbound domains and verification status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			var domains []*api.InboundDomain
			if err := e.client.Do(cmd.Context(), "GET", "/api/inbound/domains/", nil, &domains); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), domains)
			}
			if len(domains) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no inbound domains — add one with `mailafrica inbound domain add --domain <name>`")
				return nil
			}
			rows := make([][]string, 0, len(domains))
			for _, d := range domains {
				verified := "no"
				if d.VerifiedAt != nil {
					verified = fmtTime(*d.VerifiedAt)
				}
				rows = append(rows, []string{
					fmt.Sprintf("%d", d.ID),
					output.Empty(d.Domain),
					verified,
					fmtTimePtr(d.LastCheckAt, "-"),
					fmtTime(d.CreatedAt),
				})
			}
			output.Table(cmd.OutOrStdout(), []string{"ID", "Domain", "Verified", "Last check", "Added"}, rows)
			return nil
		},
	}
}

func newInboundDomainVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify <id>",
		Short: "Re-check a domain's DNS and mark it verified when ready",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			var d api.InboundDomain
			if err := e.client.Do(cmd.Context(), "POST", fmt.Sprintf("/api/inbound/domains/%d/verify", id), nil, &d); err != nil {
				if IsPending(err) {
					return fmt.Errorf("domain %d not verified yet: the TXT record is not published/propagated — retry once DNS has propagated", id)
				}
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), d)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "domain %s verified\n", d.Domain)
			return nil
		},
	}
}

func newInboundDomainDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a custom inbound domain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if err := e.client.Do(cmd.Context(), "DELETE", fmt.Sprintf("/api/inbound/domains/%d", id), nil, nil); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "domain %d deleted\n", id)
			return nil
		},
	}
}

func newInboundMessageCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "message", Short: "Inspect received mail"}
	cmd.AddCommand(
		newInboundMessageListCmd(),
		newInboundMessageGetCmd(),
		newInboundMessageReadCmd(),
	)
	return cmd
}

func newInboundMessageListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List received messages at an address",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			addressIDStr, _ := cmd.Flags().GetString("address-id")
			if addressIDStr == "" {
				return errors.New("--address-id is required (see `mailafrica inbound address list`)")
			}
			addressID, err := strconv.ParseInt(addressIDStr, 10, 64)
			if err != nil {
				return errors.New("--address-id must be an integer")
			}

			path := fmt.Sprintf("/api/inbound/messages/?address_id=%d", addressID)
			if unread, _ := cmd.Flags().GetBool("unread"); unread {
				path += "&unread=true"
			}
			page, _ := cmd.Flags().GetInt("page")
			perPage, _ := cmd.Flags().GetInt("per-page")
			if page > 0 {
				path += fmt.Sprintf("&page=%d", page)
			}
			if perPage > 0 {
				path += fmt.Sprintf("&per_page=%d", perPage)
			}

			var msgs []*api.InboundMessage
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
				fmt.Fprintln(cmd.OutOrStdout(), "no messages at this address")
				return nil
			}
			rows := make([][]string, 0, len(msgs))
			for _, m := range msgs {
				rows = append(rows, []string{
					fmt.Sprintf("%d", m.ID),
					output.Empty(m.From),
					output.Empty(m.Subject),
					output.Bool(m.IsRead),
					fmtTime(m.ReceivedAt),
				})
			}
			output.Table(cmd.OutOrStdout(), []string{"ID", "From", "Subject", "Read", "Received"}, rows)
			if env.Pagination != nil && env.Pagination.Total > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "page %d of %d · %d messages (%d per page)\n",
					env.Pagination.Page, env.Pagination.TotalPages, env.Pagination.Total, env.Pagination.PerPage)
			}
			return nil
		},
	}
	cmd.Flags().String("address-id", "", "receiving address to list mail for (required)")
	cmd.Flags().Bool("unread", false, "only unread messages")
	cmd.Flags().Int("page", 0, "page number (server default when omitted)")
	cmd.Flags().Int("per-page", 0, "messages per page (server default when omitted)")
	return cmd
}

func newInboundMessageGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show one received message (including body)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			var m api.InboundMessage
			if err := e.client.Do(cmd.Context(), "GET", fmt.Sprintf("/api/inbound/messages/%d", id), nil, &m); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), m)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "From    : %s\n", output.Empty(m.From))
			fmt.Fprintf(cmd.OutOrStdout(), "To      : %s\n", output.Empty(m.To))
			fmt.Fprintf(cmd.OutOrStdout(), "Subject : %s\n", output.Empty(m.Subject))
			fmt.Fprintf(cmd.OutOrStdout(), "Received: %s\n", fmtTime(m.ReceivedAt))
			fmt.Fprintf(cmd.OutOrStdout(), "Read    : %s\n", output.Bool(m.IsRead))
			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(output.EmptyPtr(m.TextBody)))
			return nil
		},
	}
}

func newInboundMessageReadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "read <id>",
		Short: "Mark a received message as read",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if err := e.client.Do(cmd.Context(), "PATCH", fmt.Sprintf("/api/inbound/messages/%d/read", id), nil, nil); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "message %d marked read\n", id)
			return nil
		},
	}
}
