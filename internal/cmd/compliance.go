package cmd

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/MailAfrica/MailAfrica-CLI/internal/api"
	"github.com/MailAfrica/MailAfrica-CLI/internal/output"
)

func newComplianceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compliance",
		Short: "Data-handling compliance profile (PDPC) and audit export",
		Long: `MailAfrica keeps per-user compliance posture: optional PDPC registration,
the default retention window for stored inbound mail, and a point-in-time audit
export. Retention is enforced server-side by a background purger; the CLI only
sets the policy and reads the snapshot.`,
	}
	cmd.AddCommand(
		newComplianceProfileCmd(),
		newComplianceUpdateCmd(),
		newComplianceAuditCmd(),
	)
	return cmd
}

func renderProfile(cmd *cobra.Command, e *env, p *api.ComplianceProfile) error {
	if e.jsonOut {
		return output.JSON(cmd.OutOrStdout(), p)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "PDPC registered        : %t\n", p.PDPCRegistered)
	fmt.Fprintf(cmd.OutOrStdout(), "PDPC certificate number: %s\n", output.EmptyPtr(p.PDPCCertificateNumber))
	fmt.Fprintf(cmd.OutOrStdout(), "PDPC registered at     : %s\n", fmtTimePtr(p.PDPCRegisteredAt, "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "retention (days)       : %d\n", p.DefaultRetentionDays)
	fmt.Fprintf(cmd.OutOrStdout(), "consent recorded       : %t\n", p.DataConsentAt != nil)
	fmt.Fprintf(cmd.OutOrStdout(), "privacy policy version : %s\n", output.EmptyPtr(&p.PrivacyPolicyVersion))
	fmt.Fprintf(cmd.OutOrStdout(), "updated                : %s\n", fmtTime(p.UpdatedAt))
	return nil
}

func newComplianceProfileCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "profile",
		Short: "Show the compliance profile (auto-creates one on first read)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			var p api.ComplianceProfile
			if err := e.client.Do(cmd.Context(), "GET", "/api/compliance/profile", nil, &p); err != nil {
				return err
			}
			return renderProfile(cmd, e, &p)
		},
	}
}

func newComplianceUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update compliance settings (only the flags you pass are changed)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)

			upd := api.ComplianceProfileUpdate{}
			if cmd.Flags().Changed("pdpc") {
				v, _ := cmd.Flags().GetBool("pdpc")
				upd.PDPCRegistered = &v
			}
			if cmd.Flags().Changed("certificate-number") {
				v, _ := cmd.Flags().GetString("certificate-number")
				upd.PDPCCertificateNumber = &v
			}
			if cmd.Flags().Changed("registered-at") {
				v, _ := cmd.Flags().GetString("registered-at")
				if err := validateYYYYMMDD(v); err != nil {
					return err
				}
				upd.PDPCRegisteredAt = &v
			}
			if cmd.Flags().Changed("retention-days") {
				v, _ := cmd.Flags().GetInt("retention-days")
				if v < 0 {
					return errors.New("--retention-days cannot be negative")
				}
				upd.DefaultRetentionDays = &v
			}
			if upd == (api.ComplianceProfileUpdate{}) {
				return errors.New("pass at least one of --pdpc, --certificate-number, --registered-at, --retention-days")
			}

			var p api.ComplianceProfile
			if err := e.client.Do(cmd.Context(), "PATCH", "/api/compliance/profile", upd, &p); err != nil {
				return err
			}
			return renderProfile(cmd, e, &p)
		},
	}
	cmd.Flags().Bool("pdpc", false, "mark the account PDPC-registered (true/false)")
	cmd.Flags().String("certificate-number", "", "PDPC certificate number")
	cmd.Flags().String("registered-at", "", "PDPC registration date as YYYY-MM-DD")
	cmd.Flags().Int("retention-days", 0, "default retention window in days")
	return cmd
}

func newComplianceAuditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "audit",
		Short: "Point-in-time audit export (profile, counts, generated_at)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			var a api.ComplianceAuditExport
			if err := e.client.Do(cmd.Context(), "GET", "/api/compliance/audit-export", nil, &a); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), a)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "PDPC registered : %t\n", a.PDPCRegistered)
			fmt.Fprintf(cmd.OutOrStdout(), "certificate     : %s\n", output.EmptyPtr(&a.PDPCCertificateNumber))
			fmt.Fprintf(cmd.OutOrStdout(), "retention (days): %d\n", a.DefaultRetentionDays)
			fmt.Fprintf(cmd.OutOrStdout(), "inbound address : %d\n", a.AddressCount)
			fmt.Fprintf(cmd.OutOrStdout(), "stored messages : %d\n", a.MessageCount)
			fmt.Fprintf(cmd.OutOrStdout(), "generated at    : %s\n", fmtTime(a.GeneratedAt))
			return nil
		},
	}
}

func validateYYYYMMDD(s string) error {
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return errors.New("--registered-at must be YYYY-MM-DD")
	}
	return nil
}
