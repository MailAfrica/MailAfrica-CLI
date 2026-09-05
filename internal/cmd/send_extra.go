package cmd

import (
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/MailAfrica/MailAfrica-CLI/internal/api"
	"github.com/MailAfrica/MailAfrica-CLI/internal/output"
)

func newSendListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List outbound emails (billed sends)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			path := "/api/outbound/emails"
			sep := "?"
			if page, _ := cmd.Flags().GetInt("page"); page > 0 {
				path += sep + "page=" + strconv.Itoa(page)
				sep = "&"
			}
			if perPage, _ := cmd.Flags().GetInt("per-page"); perPage > 0 {
				path += sep + "per_page=" + strconv.Itoa(perPage)
			}

			var msgs []*api.OutboundMessage
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
				fmt.Fprintln(cmd.OutOrStdout(), "no sent emails — send one with `mailafrica send email --to <recipient> --subject ...`")
				return nil
			}
			rows := make([][]string, 0, len(msgs))
			for _, m := range msgs {
				rows = append(rows, []string{
					fmt.Sprintf("%d", m.ID),
					output.Empty(m.FromAddress),
					output.Empty(strings.Join(m.ToAddresses, ",")),
					output.Empty(m.Subject),
					m.Status,
					fmt.Sprintf("%d TZS", m.AmountTZS),
					fmtTime(m.CreatedAt),
				})
			}
			output.Table(cmd.OutOrStdout(), []string{"ID", "From", "To", "Subject", "Status", "Cost", "Sent"}, rows)
			if env.Pagination != nil && env.Pagination.Total > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "page %d of %d · %d emails\n", env.Pagination.Page, env.Pagination.TotalPages, env.Pagination.Total)
			}
			return nil
		},
	}
	cmd.Flags().Int("page", 0, "page number")
	cmd.Flags().Int("per-page", 0, "emails per page")
	return cmd
}

func newSendGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show one outbound email and its per-recipient statuses",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			var d api.OutboundMessageDetail
			if err := e.client.Do(cmd.Context(), "GET", fmt.Sprintf("/api/outbound/emails/%d", id), nil, &d); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), d)
			}
			m := d.Message
			fmt.Fprintf(cmd.OutOrStdout(), "ID      : %d\n", m.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "From    : %s\n", output.Empty(m.FromAddress))
			fmt.Fprintf(cmd.OutOrStdout(), "To      : %s\n", output.Empty(strings.Join(m.ToAddresses, ", ")))
			fmt.Fprintf(cmd.OutOrStdout(), "Subject : %s\n", output.Empty(m.Subject))
			fmt.Fprintf(cmd.OutOrStdout(), "Status  : %s\n", output.Empty(m.Status))
			fmt.Fprintf(cmd.OutOrStdout(), "Cost    : %d TZS\n", m.AmountTZS)
			fmt.Fprintf(cmd.OutOrStdout(), "Sent    : %s\n", fmtTime(m.CreatedAt))
			if m.ErrorCode != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Error   : %s\n", *m.ErrorCode)
			}
			if len(d.Recipients) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "")
				rows := make([][]string, 0, len(d.Recipients))
				for _, r := range d.Recipients {
					rows = append(rows, []string{r.Recipient, r.Status, output.Empty(nonNil(r.ProviderCode))})
				}
				output.Table(cmd.OutOrStdout(), []string{"Recipient", "Status", "Provider code"}, rows)
			}
			return nil
		},
	}
}

func newSendTemplateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		Short: "Manage reusable send templates",
	}
	cmd.AddCommand(
		newTemplateCreateCmd(),
		newTemplateListCmd(),
		newTemplateUpdateCmd(),
		newTemplateDeleteCmd(),
	)
	return cmd
}

// templateBody collects the name/subject/bodies from template flags, reading
// body text from files when file flags are used.
func templateBody(cmd *cobra.Command) (*api.TemplateRequest, error) {
	name, _ := cmd.Flags().GetString("name")
	if name == "" {
		return nil, errors.New("--name is required")
	}
	subject, _ := cmd.Flags().GetString("subject")
	req := &api.TemplateRequest{Name: name, Subject: subject}

	html := mustString(cmd, "html-body")
	if f := mustString(cmd, "html-file"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read html file: %w", err)
		}
		html = string(b)
	}
	text := mustString(cmd, "text-body")
	if f := mustString(cmd, "text-file"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read text file: %w", err)
		}
		text = string(b)
	}
	req.HTMLBody = html
	req.TextBody = text
	if req.HTMLBody == "" && req.TextBody == "" {
		return nil, errors.New("give a body (--html*, --text*)")
	}
	return req, nil
}

func addTemplateFlags(cmd *cobra.Command) {
	cmd.Flags().String("name", "", "template name (required)")
	cmd.Flags().String("subject", "", "subject with optional {{placeholders}}")
	cmd.Flags().String("html-body", "", "HTML body with optional {{placeholders}}")
	cmd.Flags().String("html-file", "", "read the HTML body from a file")
	cmd.Flags().String("text-body", "", "plain-text body with optional {{placeholders}}")
	cmd.Flags().String("text-file", "", "read the plain-text body from a file")
}

func renderTemplate(cmd *cobra.Command, e *env, t *api.Template) error {
	if e.jsonOut {
		return output.JSON(cmd.OutOrStdout(), t)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "template %d: %s\n", t.ID, t.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "subject : %s\n", output.Empty(t.Subject))
	fmt.Fprintf(cmd.OutOrStdout(), "html    : %t\n", t.HTMLBody != "")
	fmt.Fprintf(cmd.OutOrStdout(), "text    : %t\n", t.TextBody != "")
	return nil
}

func newTemplateCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a template",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			req, err := templateBody(cmd)
			if err != nil {
				return err
			}
			var t api.Template
			if err := e.client.Do(cmd.Context(), "POST", "/api/outbound/templates", req, &t); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "template created: %d · %s\n", t.ID, t.Name)
			return nil
		},
	}
	addTemplateFlags(cmd)
	return cmd
}

func newTemplateListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List templates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			var tpls []*api.Template
			if err := e.client.Do(cmd.Context(), "GET", "/api/outbound/templates", nil, &tpls); err != nil {
				return err
			}
			if e.jsonOut {
				return output.JSON(cmd.OutOrStdout(), tpls)
			}
			if len(tpls) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no templates — create one with `mailafrica send template create --name ...`")
				return nil
			}
			rows := make([][]string, 0, len(tpls))
			for _, t := range tpls {
				rows = append(rows, []string{
					fmt.Sprintf("%d", t.ID),
					output.Empty(t.Name),
					output.Empty(t.Subject),
					fmtTime(t.UpdatedAt),
				})
			}
			output.Table(cmd.OutOrStdout(), []string{"ID", "Name", "Subject", "Updated"}, rows)
			return nil
		},
	}
}

func newTemplateUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Replace a template (full replace of name/subject/bodies)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			req, err := templateBody(cmd)
			if err != nil {
				return err
			}
			var t api.Template
			if err := e.client.Do(cmd.Context(), "PATCH", fmt.Sprintf("/api/outbound/templates/%d", id), req, &t); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "template %d updated · %s\n", t.ID, t.Name)
			return nil
		},
	}
	addTemplateFlags(cmd)
	return cmd
}

func newTemplateDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := envFrom(cmd)
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if err := e.client.Do(cmd.Context(), "DELETE", fmt.Sprintf("/api/outbound/templates/%d", id), nil, nil); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "template %d deleted\n", id)
			return nil
		},
	}
}

func readLines(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var lines []string
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

func base64Encode(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// contentTypeFor guesses a MIME type from a file extension for attachments.
func contentTypeFor(path string) string {
	if ct := mime.TypeByExtension(filepath.Ext(strings.ToLower(path))); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
