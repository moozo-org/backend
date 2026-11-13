// Package mail sends the application's emails: through Brevo in production,
// or to the log in local development.
package mail

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"

	"moozo/internal/app/command"
	"moozo/moozo"
	"moozo/pkg/brevo"
)

// The code lifetimes are quoted in the emails.
const (
	verificationCodeLifetime  = command.VerificationCodeLifetime
	passwordResetCodeLifetime = command.PasswordResetCodeLifetime
)

// Brevo writes the application's emails and sends them through a Brevo
// client.
type Brevo struct {
	client *brevo.Client
	sender brevo.Contact
}

var _ moozo.Mailer = (*Brevo)(nil)

// NewBrevo returns a mailer sending from senderName <senderEmail>, which must
// be verified in the Brevo account.
func NewBrevo(client *brevo.Client, senderEmail, senderName string) *Brevo {
	return &Brevo{client: client, sender: brevo.Contact{Email: senderEmail, Name: senderName}}
}

// codeHTML is the body of every email that carries a code.
var codeHTML = template.Must(template.New("code").Parse(
	`<p>{{.Intro}}</p>` +
		`<p style="font-size:24px;font-weight:bold;letter-spacing:4px">{{.Code}}</p>` +
		`<p>It expires in {{.Minutes}} minutes. {{.Ignore}}</p>`,
))

func (b *Brevo) SendVerificationCode(ctx context.Context, email, code string) error {
	return b.sendCode(ctx, email, code, verificationCodeLifetime,
		"Your Moozo verification code",
		"Your Moozo verification code is",
		"If you did not create an account, ignore this email.")
}

func (b *Brevo) SendPasswordResetCode(ctx context.Context, email, code string) error {
	return b.sendCode(ctx, email, code, passwordResetCodeLifetime,
		"Reset your Moozo password",
		"Your Moozo password reset code is",
		"If you did not ask to reset your password, ignore this email; your password stays the same.")
}

// sendCode emails code with intro before it, then how long it lasts and
// ignore.
func (b *Brevo) sendCode(ctx context.Context, email, code string, lifetime time.Duration, subject, intro, ignore string) error {
	minutes := int(lifetime.Minutes())
	var html bytes.Buffer
	err := codeHTML.Execute(&html, map[string]any{
		"Intro": intro + ":", "Code": code, "Minutes": minutes, "Ignore": ignore,
	})
	if err != nil {
		return err
	}
	return b.client.SendEmail(ctx, brevo.Email{
		Sender:      b.sender,
		To:          []brevo.Contact{{Email: email}},
		Subject:     subject,
		TextContent: fmt.Sprintf("%s %s. It expires in %d minutes. %s", intro, code, minutes, ignore),
		HTMLContent: html.String(),
	})
}
