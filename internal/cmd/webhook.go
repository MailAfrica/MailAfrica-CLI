package cmd

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/MailAfrica/MailAfrica-CLI/internal/api"
	"github.com/MailAfrica/MailAfrica-CLI/internal/output"
)

func newWebhookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhook",
		Short: "Wire delivery callbacks to your receiving addresses",
		Long: `Webhooks deliver received mail to your endpoint: MailAfrica POSTs a
notification (signed with the webhook's secret in the X-Signature header) each
time mail arrives at the wired inbound address.`,
	}
	cmd.AddCommand(
		newWebhookCreateCmd(),
		newWebhookListCmd(),
		newWebhookDeleteCmd(),
		newWebhookDeliveriesCmd(),
		newWebhookTestCmd(),
		newWebhookTriggerCmd(),
	)
	return cmd
}

func newWebhookCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a webhook for an inbound address",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			addressIDStr, _ := cmd.Flags().GetString("address-id")
			addressID, err := requiredID(addressIDStr, "--address-id")
			if err != nil {
				return err
			}
			url, _ := cmd.Flags().GetString("url")
			if url == "" {
				return errors.New("--url is required (the HTTPS endpoint MailAfrica will POST to)")
			}
			req := api.CreateWebhookRequest{AddressID: addressID, URL: url}
			if sec, _ := cmd.Flags().GetString("secret"); sec != "" {
				req.Secret = sec
			}

			var wh api.Webhook
			if err := e.client.Do(cmd.Context(), "POST", "/api/webhook/webhooks/", req, &wh); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), wh)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "webhook %d created for address %d\n", wh.ID, wh.AddressID)
			fmt.Fprintf(cmd.OutOrStdout(), "url    : %s\n", wh.URL)
			fmt.Fprintf(cmd.OutOrStdout(), "secret : %s\n", wh.Secret)
			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintln(cmd.OutOrStdout(), "SAVE THIS SECRET — it signs the X-Signature header on every delivery and is returned fully only here.")
			fmt.Fprintln(cmd.OutOrStdout(), "Verify it with: mailafrica webhook test "+fmt.Sprintf("%d", wh.ID))
			return nil
		},
	}
	cmd.Flags().String("address-id", "", "inbound address to receive mail (required)")
	cmd.Flags().String("url", "", "endpoint to POST delivery notifications to (required)")
	cmd.Flags().String("secret", "", "verify the X-Signature header (optional; a random one is generated if omitted)")
	return cmd
}

func newWebhookListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List webhooks for an inbound address",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			addressIDStr, _ := cmd.Flags().GetString("address-id")
			addressID, err := requiredID(addressIDStr, "--address-id")
			if err != nil {
				return err
			}
			var hooks []*api.Webhook
			if err := e.client.Do(cmd.Context(), "GET", fmt.Sprintf("/api/webhook/webhooks/?address_id=%d", addressID), nil, &hooks); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), hooks)
			}
			if len(hooks) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no webhooks for this address — create one with `mailafrica webhook create --address-id "+fmt.Sprintf("%d", addressID)+" --url <url>`")
				return nil
			}
			rows := make([][]string, 0, len(hooks))
			for _, h := range hooks {
				rows = append(rows, []string{
					fmt.Sprintf("%d", h.ID),
					h.URL,
					output.SecretShort(h.Secret),
					output.Bool(h.IsActive),
					fmtTime(h.CreatedAt),
				})
			}
			fmt.Fprintln(cmd.OutOrStdout(), "NOTE: webhook secrets are masked in listings — recreate to view a full secret.")
			output.Table(cmd.OutOrStdout(), []string{"ID", "URL", "Secret", "Active", "Created"}, rows)
			return nil
		},
	}
	cmd.Flags().String("address-id", "", "inbound address to list webhooks for (required)")
	return cmd
}

func newWebhookDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a webhook (deliveries stop)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if err := e.client.Do(cmd.Context(), "DELETE", fmt.Sprintf("/api/webhook/webhooks/%d", id), nil, nil); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "webhook %d deleted\n", id)
			return nil
		},
	}
}

func newWebhookDeliveriesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deliveries <id>",
		Short: "Show the recent delivery attempts for a webhook",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			var deliveries []*api.WebhookDelivery
			if err := e.client.Do(cmd.Context(), "GET", fmt.Sprintf("/api/webhook/webhooks/%d/deliveries", id), nil, &deliveries); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), deliveries)
			}
			if len(deliveries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no deliveries recorded yet for webhook", id)
				return nil
			}
			rows := make([][]string, 0, len(deliveries))
			for _, d := range deliveries {
				status := output.Empty(d.Status)
				if d.StatusCode != 0 {
					status = fmt.Sprintf("%s (%d)", output.Empty(d.Status), d.StatusCode)
				}
				rows = append(rows, []string{
					fmt.Sprintf("%d", d.ID),
					fmt.Sprintf("%d", d.MessageID),
					fmt.Sprintf("attempt %d", d.Attempt),
					status,
					output.Empty(d.LastError),
					fmtTimePtr(d.DeliveredAt, "-"),
				})
			}
			output.Table(cmd.OutOrStdout(), []string{"ID", "Message", "Attempt", "Status", "Last error", "Delivered"}, rows)
			return nil
		},
	}
}

func newWebhookTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <id>",
		Short: "Ask MailAfrica to POST a test ping to the webhook URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			var resp map[string]any
			if err := e.client.Do(cmd.Context(), "POST", fmt.Sprintf("/api/webhook/webhooks/%d/test", id), nil, &resp); err != nil {
				var ae *api.APIError
				if errors.As(err, &ae) && ae.Code == "DELIVERY_FAILED" {
					return fmt.Errorf("test ping failed: your endpoint did not accept the delivery (%s)", ae.Message)
				}
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), resp)
			}
			if code, _ := resp["status_code"].(float64); code != 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "test ping delivered — endpoint answered HTTP %d\n", int(code))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "test ping delivered")
			}
			return nil
		},
	}
}

func newWebhookTriggerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trigger <message-id>",
		Short: "Manually dispatch the delivery notification for a received message",
		Long:  "Re-dispatches the webhook payload for an already-received message id (useful when your endpoint was down).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if err := e.client.Do(cmd.Context(), "POST", fmt.Sprintf("/api/webhook/trigger/%d", id), nil, nil); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "dispatch for message %d queued\n", id)
			return nil
		},
	}
}

// IsPending reports whether err is the API-specific "DNS not ready yet" state
// the inbound domain verify endpoint returns as HTTP 200 with code PENDING.
func IsPending(err error) bool {
	var ae *api.APIError
	return errors.As(err, &ae) && ae.Code == "PENDING"
}

// parseID validates a positional integer id argument.
func parseID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("id must be a positive integer")
	}
	return id, nil
}

// requiredID reads a required flag value returning its integer form.
func requiredID(v string, flag string) (int64, error) {
	if v == "" {
		return 0, errors.New(flag + " is required")
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New(flag + " must be a positive integer")
	}
	return id, nil
}
