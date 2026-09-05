package cmd

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/MailAfrica/MailAfrica-CLI/internal/api"
	"github.com/MailAfrica/MailAfrica-CLI/internal/output"
)

func newSMSCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sms",
		Short: "Get a short SMS when an inbound address receives mail",
		Long: `Wire a Tanzania mobile number to an inbound address: every received email
triggers a short provider SMS (encrypted at rest; your provider key is shown
only once at creation). Phone numbers are normalized to +255 international
form.`,
	}
	cmd.AddCommand(
		newSMSNotificationCmd(),
	)
	return cmd
}

func newSMSNotificationCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "notification", Short: "Manage SMS notifications on inbound addresses"}
	cmd.AddCommand(
		newSMSNotificationCreateCmd(),
		newSMSNotificationListCmd(),
		newSMSNotificationRevokeCmd(),
		newSMSNotificationDeliveriesCmd(),
	)
	return cmd
}

func renderSMSNotification(cmd *cobra.Command, n *api.SMSNotification) {
	fmt.Fprintf(cmd.OutOrStdout(), "id        : %d\n", n.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "address id: %d\n", n.AddressID)
	fmt.Fprintf(cmd.OutOrStdout(), "phone     : %s\n", n.PhoneNumber)
	fmt.Fprintf(cmd.OutOrStdout(), "active    : %t\n", n.IsActive)
	fmt.Fprintf(cmd.OutOrStdout(), "created   : %s\n", fmtTime(n.CreatedAt))
}

// newSMSNotificationCreateCmd wires a phone to an address with a provider key.
// The key is surfaced only once (exactly like SMTP passwords and API keys).
func newSMSNotificationCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an SMS notification (provider key shown once)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			addressID, err := cmd.Flags().GetInt64("address-id")
			if err != nil || addressID <= 0 {
				return errors.New("--address-id is required (see `mailafrica inbound address list`)")
			}
			phone, _ := cmd.Flags().GetString("phone")
			if phone == "" {
				return errors.New("--phone is required (a Tanzania mobile: 07xx/06xx, +255xxx, or 255xxx)")
			}
			providerKey, _ := cmd.Flags().GetString("sendafrica-key")
			if providerKey == "" {
				return errors.New("--sendafrica-key is required (your SendAfrica API key)")
			}

			var resp api.SMSCreateResponse
			if err := e.client.Do(cmd.Context(), "POST", "/api/sms/notifications",
				api.SMSCreateRequest{AddressID: addressID, PhoneNumber: phone, APIKey: providerKey}, &resp); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), resp)
			}
			renderSMSNotification(cmd, &resp.SMSNotification)
			fmt.Fprintf(cmd.OutOrStdout(), "provider key: %s\n", resp.APIKey)
			fmt.Fprintln(cmd.OutOrStdout(), "SAVE THIS PROVIDER KEY — it is shown only once.")
			return nil
		},
	}
	cmd.Flags().Int64("address-id", 0, "inbound address to attach the alert to (required)")
	cmd.Flags().String("phone", "", "Tanzania mobile number (07xx/06xx, +255xxx, or 255xxx) (required)")
	cmd.Flags().String("sendafrica-key", "", "your SendAfrica API key (required; never stored by the CLI, shown once) (required)")
	return cmd
}

func newSMSNotificationListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List SMS notifications for an inbound address",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			addressID, err := cmd.Flags().GetInt64("address-id")
			if err != nil || addressID <= 0 {
				return errors.New("--address-id is required")
			}
			var notifs []*api.SMSNotification
			if err := e.client.Do(cmd.Context(), "GET",
				fmt.Sprintf("/api/sms/notifications?address_id=%d", addressID), nil, &notifs); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), notifs)
			}
			if len(notifs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no SMS notifications for this address — create one with `mailafrica sms notification create ...`")
				return nil
			}
			rows := make([][]string, 0, len(notifs))
			for _, n := range notifs {
				rows = append(rows, []string{
					fmt.Sprintf("%d", n.ID),
					fmt.Sprintf("%d", n.AddressID),
					n.PhoneNumber,
					output.Bool(n.IsActive),
					fmtTime(n.CreatedAt),
				})
			}
			output.Table(cmd.OutOrStdout(), []string{"ID", "Address", "Phone", "Active", "Created"}, rows)
			return nil
		},
	}
	cmd.Flags().Int64("address-id", 0, "inbound address (required)")
	return cmd
}

func newSMSNotificationRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <id>",
		Short: "Deactivate an SMS notification (stops future alerts)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if err := e.client.Do(cmd.Context(), "POST", fmt.Sprintf("/api/sms/notifications/%d/revoke", id), nil, nil); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "sms notification %d revoked\n", id)
			return nil
		},
	}
}

func newSMSNotificationDeliveriesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deliveries <id>",
		Short: "Delivery history for one SMS notification",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			var dels []*api.SMSDelivery
			if err := e.client.Do(cmd.Context(), "GET", fmt.Sprintf("/api/sms/notifications/%d/deliveries", id), nil, &dels); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), dels)
			}
			if len(dels) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no deliveries recorded yet")
				return nil
			}
			rows := make([][]string, 0, len(dels))
			for _, d := range dels {
				rows = append(rows, []string{
					fmt.Sprintf("%d", d.ID),
					fmt.Sprintf("%d", d.MessageID),
					d.Status,
					output.EmptyPtr(d.ErrorCode),
					strconv.Itoa(d.Attempt),
					fmtTime(d.CreatedAt),
				})
			}
			output.Table(cmd.OutOrStdout(), []string{"ID", "Message", "Status", "Error", "Attempt", "At"}, rows)
			return nil
		},
	}
}
