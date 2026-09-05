package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/MailAfrica/MailAfrica-CLI/internal/api"
	"github.com/MailAfrica/MailAfrica-CLI/internal/output"
)

const (
	flagEmail      = "email"
	flagPhone      = "phone"
	flagPassword   = "password"
	flagIdentifier = "identifier"
	flagName       = "name"
	flagCompany    = "company"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Register, log in, and manage the session",
	}
	cmd.AddCommand(
		newAuthRegisterCmd(),
		newAuthLoginCmd(),
		newAuthRefreshCmd(),
		newAuthLogoutCmd(),
		newAuthMeCmd(),
		newAuthUpdateCmd(),
		newAuthVerifyCmd(),
	)
	return cmd
}

func newAuthRegisterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Create an account (email and/or phone) and start a session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			email, _ := cmd.Flags().GetString(flagEmail)
			phone, _ := cmd.Flags().GetString(flagPhone)
			if email == "" && phone == "" {
				return errors.New("provide at least one of --email or --phone")
			}
			name, _ := cmd.Flags().GetString(flagName)
			if name == "" {
				return errors.New("--name is required")
			}
			password, _ := cmd.Flags().GetString(flagPassword)
			if password == "" {
				p, err := promptPassword("Password: ")
				if err != nil {
					return err
				}
				password = p
			}
			company, _ := cmd.Flags().GetString(flagCompany)

			body := map[string]any{
				"password":     password,
				"name":         name,
				"company_name": company,
			}
			if email != "" {
				body["email"] = email
			}
			if phone != "" {
				body["phone_number"] = phone
			}

			var ar api.AuthResponse
			if err := e.client.DoPublic(cmd.Context(), "POST", "/api/auth/register", body, &ar); err != nil {
				return err
			}
			return e.persistSession(cmd, ar)
		},
	}
	cmd.Flags().String(flagEmail, "", "email address (or --phone)")
	cmd.Flags().String(flagPhone, "", "mobile number in +255... / 07xx form (or --email)")
	cmd.Flags().String(flagPassword, "", "password (prompted if omitted)")
	cmd.Flags().String(flagName, "", "display name")
	cmd.Flags().String(flagCompany, "", "company name (optional)")
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in with email or phone and a password",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			identifier, _ := cmd.Flags().GetString(flagIdentifier)
			if identifier == "" {
				return errors.New("--identifier is required (email or phone)")
			}
			password, _ := cmd.Flags().GetString(flagPassword)
			if password == "" {
				p, err := promptPassword("Password: ")
				if err != nil {
					return err
				}
				password = p
			}

			var ar api.AuthResponse
			err := e.client.DoPublic(cmd.Context(), "POST", "/api/auth/login",
				map[string]string{"identifier": identifier, "password": password}, &ar)
			if err != nil {
				return err
			}
			return e.persistSession(cmd, ar)
		},
	}
	cmd.Flags().String(flagIdentifier, "", "email or phone identifier")
	cmd.Flags().String(flagPassword, "", "password (prompted if omitted)")
	return cmd
}

func newAuthRefreshCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Rotate the stored refresh token now",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			if err := e.client.Refresh(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "session refreshed")
			return nil
		},
	}
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Forget stored credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			if err := e.cfg.ClearAuth(); err != nil {
				return fmt.Errorf("clear credentials: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "logged out — stored credentials cleared")
			return nil
		},
	}
}

func newAuthMeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Show the authenticated user",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			var u api.User
			if err := e.client.Do(cmd.Context(), "GET", "/api/auth/me", nil, &u); err != nil {
				return err
			}
			return renderUser(cmd, e, &u)
		},
	}
}

func newAuthUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update name and/or company name",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			body := map[string]any{}
			if cmd.Flags().Changed(flagName) {
				n, _ := cmd.Flags().GetString(flagName)
				body["name"] = n
			}
			if cmd.Flags().Changed(flagCompany) {
				c, _ := cmd.Flags().GetString(flagCompany)
				body["company_name"] = c
			}
			if len(body) == 0 {
				return errors.New("no fields to update — pass --name and/or --company")
			}
			var u api.User
			if err := e.client.Do(cmd.Context(), "PATCH", "/api/auth/me", body, &u); err != nil {
				return err
			}
			return renderUser(cmd, e, &u)
		},
	}
	cmd.Flags().String(flagName, "", "new display name")
	cmd.Flags().String(flagCompany, "", "new company name (empty clears)")
	return cmd
}

func newAuthVerifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify an email address or phone number",
	}
	verEmail := &cobra.Command{
		Use:   "email --token <token>",
		Short: "Confirm email via the token from the verification link",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			token, _ := cmd.Flags().GetString("token")
			if token == "" {
				return errors.New("--token is required")
			}
			if err := e.client.DoPublic(cmd.Context(), "POST", "/api/auth/email/verify", map[string]string{"token": token}, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "email verified")
			return nil
		},
	}
	verEmail.Flags().String("token", "", "verification token from the emailed link")

	resendEmail := &cobra.Command{
		Use:   "resend-email",
		Short: "Resend the email verification link",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			if err := e.client.Do(cmd.Context(), "POST", "/api/auth/email/resend", nil, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "verification email resent")
			return nil
		},
	}

	verPhone := &cobra.Command{
		Use:   "phone --code <6-digit-code>",
		Short: "Confirm phone via the 6-digit OTP",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			code, _ := cmd.Flags().GetString("code")
			if code == "" {
				return errors.New("--code is required")
			}
			if err := e.client.Do(cmd.Context(), "POST", "/api/auth/phone/verify", map[string]string{"code": code}, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "phone verified")
			return nil
		},
	}
	verPhone.Flags().String("code", "", "6-digit OTP")

	resendPhone := &cobra.Command{
		Use:   "resend-phone",
		Short: "Resend the phone OTP",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e := envFrom(cmd)
			if err := e.client.Do(cmd.Context(), "POST", "/api/auth/phone/resend", nil, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "OTP resent")
			return nil
		},
	}

	cmd.AddCommand(verEmail, resendEmail, verPhone, resendPhone)
	return cmd
}

// persistSession stores the refresh token (never the JWT) and confirms the
// login to the user. The JWT lives only in the process; later invocations
// obtain a fresh one from the stored refresh token.
func (e *env) persistSession(cmd *cobra.Command, ar api.AuthResponse) error {
	e.client.SetAccessToken(ar.Token)
	if err := e.cfg.SetRefreshToken(ar.RefreshToken); err != nil {
		return fmt.Errorf("store refresh token: %w", err)
	}
	if ar.User.Email != nil {
		_ = e.cfg.SetLastEmail(*ar.User.Email)
	}
	return renderUser(cmd, e, &ar.User)
}

func renderUser(cmd *cobra.Command, e *env, u *api.User) error {
	w := cmd.OutOrStdout()
	if e.jsonOut {
		return output.JSON(w, u)
	}
	ident := strings.TrimSpace(nonNil(u.Email) + " / " + nonNil(u.PhoneNumber))
	output.Table(
		w,
		[]string{"Field", "Value"},
		[][]string{
			{"ID", fmt.Sprintf("%d", u.ID)},
			{"Name", output.Empty(u.Name)},
			{"Identity", output.Empty(ident)},
			{"Company", output.Empty(nonNil(u.CompanyName))},
			{"Email verified", output.Bool(u.EmailVerifiedAt != nil)},
			{"Phone verified", output.Bool(u.PhoneVerifiedAt != nil)},
			{"Created", fmt.Sprintf("%v", u.CreatedAt.Format("2006-01-02 15:04"))},
		},
	)
	return nil
}

func nonNil(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// promptPassword reads a password without echoing it to the terminal.
func promptPassword(prompt string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("password required but stdin is not a terminal — pass --password (or use an API key via MAILAFRICA_API_KEY)")
	}
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	if len(b) == 0 {
		return "", errors.New("password cannot be empty")
	}
	return string(b), nil
}
