package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/MailAfrica/MailAfrica-CLI/internal/api"
	"github.com/MailAfrica/MailAfrica-CLI/internal/output"
)

// maxRecipientsPerCall is the client-side recipient cap for one API call.
// Anything larger must go through `send batch --auto-chunk`, which splits the
// list into sequential ≤ maxRecipientsPerCall sends and stops on the first
// failure.
const maxRecipientsPerCall = 50

// chunkPause is the pacing between chunked sends. The API rate-limits sends at
// 2/s per user; back-to-back chunks would hit HTTP 429.
const chunkPause = 500 * time.Millisecond

func newSendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send transactional email and manage templates",
		Long: `Send email through MailAfrica's own self-hosted MTA (local Postfix
with per-domain DKIM signing — no third-party SMTP supplier). Sends are billed
per recipient from your wallet balance and require a verified identity on the
account.`,
	}
	cmd.AddCommand(
		newSendEmailCmd(),
		newSendBatchCmd(),
		newSendListCmd(),
		newSendGetCmd(),
		newSendTemplateCmd(),
	)
	return cmd
}

func newSendEmailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "email",
		Short: "Send one email to a recipient list",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			req, err := buildSendRequest(cmd)
			if err != nil {
				return err
			}
			if len(req.To) == 0 {
				return errors.New("--to is required (comma-separated or repeated)")
			}
			if len(req.To) > maxRecipientsPerCall {
				return fmt.Errorf("%d recipients exceeds the %d-per-call limit — use `mailafrica send batch --to <same list> --auto-chunk`", len(req.To), maxRecipientsPerCall)
			}

			var msg api.OutboundMessage
			if err := e.client.Do(cmd.Context(), "POST", "/api/outbound/emails", req, &msg); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), msg)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "sent (%d recipients) · message %d · %d TZS\n", len(req.To), msg.ID, msg.AmountTZS)
			if msg.ProviderMessageID != nil && *msg.ProviderMessageID != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "mta id: %s\n", *msg.ProviderMessageID)
			}
			return nil
		},
	}
	addSendFlags(cmd)
	return cmd
}

func newSendBatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch",
		Short: "Send the same email to many recipients",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			req, err := buildSendRequest(cmd)
			if err != nil {
				return err
			}
			if len(req.To) == 0 {
				return errors.New("--to is required (comma-separated, repeated, or --to-file)")
			}
			autoChunk, _ := cmd.Flags().GetBool("auto-chunk")
			if len(req.To) > maxRecipientsPerCall && !autoChunk {
				return fmt.Errorf("%d recipients exceeds the %d-per-call limit — pass --auto-chunk to split into sequential sends (stop on first failure)", len(req.To), maxRecipientsPerCall)
			}

			out := &batchOutcome{total: len(req.To), cap: maxRecipientsPerCall}
			if !autoChunk {
				var msg api.OutboundMessage
				if err := e.client.Do(cmd.Context(), "POST", "/api/outbound/emails", req, &msg); err != nil {
					return err
				}
				out.sent = len(req.To)
				out.messageIDs = append(out.messageIDs, msg.ID)
				out.finish(cmd)
				return nil
			}

			// Chunked path: sequential single sends, stop on the first failure.
			base := *req
			for i := 0; i < out.chunks(); i++ {
				start, end := out.chunkBounds(i)
				chunk := base
				chunk.To = req.To[start:end]
				var msg api.OutboundMessage
				if err := e.client.Do(cmd.Context(), "POST", "/api/outbound/emails", chunk, &msg); err != nil {
					out.failedChunk = i + 1
					out.finish(cmd)
					return errors.New("batch stopped after a failed chunk")
				}
				out.sent += len(chunk.To)
				out.messageIDs = append(out.messageIDs, msg.ID)
				if i < out.chunks()-1 {
					time.Sleep(chunkPause)
				}
			}
			out.finish(cmd)
			return nil
		},
	}
	addSendFlags(cmd)
	cmd.Flags().Bool("auto-chunk", false, "split >50 recipients into sequential ≤50 sends, stopping on the first failure")
	return cmd
}

// batchOutcome computes and renders the batch result. The contracted human
// message on partial failure is explicit about exactly which chunk failed and
// that later chunks were never attempted — a rolled-up "X of Y" summary is not
// acceptable.
type batchOutcome struct {
	total       int
	cap         int
	sent        int
	failedChunk int
	messageIDs  []int64
}

func (b *batchOutcome) chunks() int {
	return (b.total + b.cap - 1) / b.cap
}

func (b *batchOutcome) chunkBounds(i int) (int, int) {
	start := i * b.cap
	end := start + b.cap
	if end > b.total {
		end = b.total
	}
	return start, end
}

// finish prints the outcome summary to the command's stdout. Chunk failures
// always render the explicit chunk report — never a rolled-up "X/Y" summary.
func (b *batchOutcome) finish(cmd *cobra.Command) {
	k := b.chunks()
	switch {
	case b.failedChunk != 0:
		fmt.Fprintf(cmd.OutOrStdout(), "sent %d/%d across %d chunks · chunk %d failed", b.sent, b.total, k, b.failedChunk)
		if b.failedChunk < k {
			fmt.Fprintf(cmd.OutOrStdout(), " · chunks %d..%d not attempted", b.failedChunk+1, k)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	case len(b.messageIDs) == 1:
		fmt.Fprintf(cmd.OutOrStdout(), "sent %d/%d · message %d\n", b.sent, b.total, b.messageIDs[0])
	default:
		fmt.Fprintf(cmd.OutOrStdout(), "sent %d/%d across %d chunks · message ids %s\n", b.sent, b.total, k, idsCSV(b.messageIDs))
	}
}

func idsCSV(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

// buildSendRequest assembles the API body from send flags: recipient lists,
// bodies (inline or file), a saved template plus variables, optional From
// identity, and attachments. Either a body must be given or a template id.
func buildSendRequest(cmd *cobra.Command) (*api.OutboundSendRequest, error) {
	to, _ := cmd.Flags().GetStringSlice("to")
	cc, _ := cmd.Flags().GetStringSlice("cc")
	bcc, _ := cmd.Flags().GetStringSlice("bcc")

	toFile, _ := cmd.Flags().GetString("to-file")
	if toFile != "" {
		lines, err := readLines(toFile)
		if err != nil {
			return nil, err
		}
		to = append(to, lines...)
	}

	req := &api.OutboundSendRequest{
		To:      to,
		Cc:      cc,
		Bcc:     bcc,
		Subject: mustString(cmd, "subject"),
	}

	htmlBody := mustString(cmd, "html-body")
	if htmlFile := mustString(cmd, "html-file"); htmlFile != "" {
		b, err := os.ReadFile(htmlFile)
		if err != nil {
			return nil, fmt.Errorf("read html file: %w", err)
		}
		htmlBody = string(b)
	}
	textBody := mustString(cmd, "text-body")
	if textFile := mustString(cmd, "text-file"); textFile != "" {
		b, err := os.ReadFile(textFile)
		if err != nil {
			return nil, fmt.Errorf("read text file: %w", err)
		}
		textBody = string(b)
	}
	req.HTMLBody = htmlBody
	req.TextBody = textBody

	if tpl, _ := cmd.Flags().GetString("template-id"); tpl != "" {
		id, err := strconv.ParseInt(tpl, 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("--template-id must be a positive integer")
		}
		req.TemplateID = &id
	}
	if vars, _ := cmd.Flags().GetStringSlice("var"); len(vars) > 0 {
		m := map[string]string{}
		for _, kv := range vars {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("--var must be key=value, got %q", kv)
			}
			m[parts[0]] = parts[1]
		}
		req.Variables = m
	}

	if req.HTMLBody == "" && req.TextBody == "" && req.TemplateID == nil {
		return nil, errors.New("give a body (--html*, --text*) or a --template-id")
	}

	if d, _ := cmd.Flags().GetString("from-domain-id"); d != "" {
		id, err := strconv.ParseInt(d, 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("--from-domain-id must be a positive integer")
		}
		req.FromDomainID = &id
	}
	if fa, _ := cmd.Flags().GetString("from-address"); fa != "" {
		req.FromAddress = &fa
	}

	if attachFiles, _ := cmd.Flags().GetStringSlice("attach"); len(attachFiles) > 0 {
		atts, err := buildAttachments(attachFiles)
		if err != nil {
			return nil, err
		}
		req.Attachments = atts
	}
	return req, nil
}

func mustString(cmd *cobra.Command, name string) string {
	s, _ := cmd.Flags().GetString(name)
	return s
}

// buildAttachments reads local files into the base64 attachment shape the API
// expects (mirroring the server's own 10-file/20MiB limits roughly by letting
// the API enforce them).
func buildAttachments(files []string) ([]api.Attachment, error) {
	atts := make([]api.Attachment, 0, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read attachment %s: %w", f, err)
		}
		atts = append(atts, api.Attachment{
			Filename:    filepath.Base(f),
			ContentType: contentTypeFor(f),
			Size:        len(b),
			DataBase64:  base64Encode(b),
		})
	}
	return atts, nil
}

func addSendFlags(cmd *cobra.Command) {
	cmd.Flags().StringSlice("to", nil, "recipient email (comma-separated or repeated)")
	cmd.Flags().StringSlice("cc", nil, "copy recipient (comma-separated or repeated)")
	cmd.Flags().StringSlice("bcc", nil, "blind-copy recipient (comma-separated or repeated)")
	cmd.Flags().String("to-file", "", "read recipients from a file (one email per line)")
	cmd.Flags().String("subject", "", "email subject (required when not using a template's)")
	cmd.Flags().String("html-body", "", "HTML body (alternatively --html-file or a template)")
	cmd.Flags().String("html-file", "", "read the HTML body from a file")
	cmd.Flags().String("text-body", "", "plain-text body (alternatively --text-file or a template)")
	cmd.Flags().String("text-file", "", "read the plain-text body from a file")
	cmd.Flags().String("template-id", "", "send using a saved template (inline bodies override it)")
	cmd.Flags().StringSlice("var", nil, "template variable as key=value (repeatable)")
	cmd.Flags().String("from-domain-id", "", "send from a verified sending domain (default: platform sender)")
	cmd.Flags().String("from-address", "", "From override (must be an identity you control)")
	cmd.Flags().StringSlice("attach", nil, "file to attach (repeatable; base64-embedded)")
}
