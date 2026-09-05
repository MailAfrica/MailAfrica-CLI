package cmd

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/MailAfrica/MailAfrica-CLI/internal/api"
	"github.com/MailAfrica/MailAfrica-CLI/internal/output"
)

const minTopupTZS = 2000

func newWalletCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wallet",
		Short: "Check and top up your TZS wallet balance",
		Long: `Your wallet holds the TZS balance that every inbound/outbound message is
billed against. Balance reads are free; loading money goes through the pinned
mobile-money / card checkout provider (hosted page or a direct USSD push to a
verified phone). Min top-up is 2000 TZS. Payment details stay with the payment
provider — the CLI only ever prints the payment links it receives.`,
	}
	cmd.AddCommand(
		newWalletBalanceCmd(),
		newWalletTopupCmd(),
	)
	return cmd
}

func newWalletBalanceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "balance",
		Short: "Show the current wallet balance",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			var b struct {
				BalanceTZS int64 `json:"balance_tzs"`
			}
			if err := e.client.Do(cmd.Context(), "GET", "/api/billing/balance", nil, &b); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), b)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wallet balance: %d TZS\n", b.BalanceTZS)
			return nil
		},
	}
}

func newWalletTopupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "topup",
		Short: "Top up the wallet (hosted checkout, or USSD push with --via phone)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			amountStr, _ := cmd.Flags().GetString("amount")
			amount, err := strconv.ParseInt(amountStr, 10, 64)
			if err != nil || amount < minTopupTZS {
				return fmt.Errorf("--amount must be a whole TZS amount of at least %d", minTopupTZS)
			}
			via, _ := cmd.Flags().GetString("via")

			var resp api.BillingTopupResponse
			path, body := "/api/billing/topup", map[string]any{"amount_tzs": amount}
			if via != "" {
				if via != "phone" {
					return errors.New("--via must be 'phone' for a direct USSD push (omit for the hosted checkout)")
				}
				path = "/api/billing/topup/phone"
			}
			if err := e.client.Do(cmd.Context(), "POST", path, body, &resp); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), resp)
			}
			if resp.Topup != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "topup %d created · %d TZS · status %s\n", resp.Topup.ID, resp.Topup.AmountTZS, resp.Topup.Status)
			}
			if resp.CheckoutURL != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "checkout     : %s\n", resp.CheckoutURL)
			}
			if resp.PaymentLinkURL != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "payment link : %s\n", resp.PaymentLinkURL)
			}
			if resp.ProviderReference != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "reference    : %s\n", resp.ProviderReference)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "balance is credited when the payment completes (status: pending → completed)")
			return nil
		},
	}
	cmd.Flags().String("amount", "", "top-up amount in TZS (min 2000) (required)")
	cmd.Flags().String("via", "", "'phone' for a direct USSD push to your verified phone; omit for the hosted checkout")
	return cmd
}
