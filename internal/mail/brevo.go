// Package mail sends the application's emails: through Brevo in production,
// or to the log in local development.
package mail

import (
	"bytes"
	"context"
	"fmt"
	"html/template"

	"moozo/internal/app/command"
	"moozo/moozo"
	"moozo/pkg/brevo"
)

// verificationCodeLifetime is quoted in the emails.
const verificationCodeLifetime = command.VerificationCodeLifetime

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

var verificationHTML = template.Must(template.New("verification").Parse(
	`<p>Your Moozo verification code is:</p>` +
		`<p style="font-size:24px;font-weight:bold;letter-spacing:4px">{{.Code}}</p>` +
		`<p>It expires in {{.Minutes}} minutes. If you did not create an account, ignore this email.</p>`,
))

func (b *Brevo) SendVerificationCode(ctx context.Context, email, code string) error {
	minutes := int(verificationCodeLifetime.Minutes())
	var html bytes.Buffer
	if err := verificationHTML.Execute(&html, map[string]any{"Code": code, "Minutes": minutes}); err != nil {
		return err
	}
	return b.client.SendEmail(ctx, brevo.Email{
		Sender:  b.sender,
		To:      []brevo.Contact{{Email: email}},
		Subject: "Your Moozo verification code",
		TextContent: fmt.Sprintf("Your Moozo verification code is %s. It expires in %d minutes. "+
			"If you did not create an account, ignore this email.", code, minutes),
		HTMLContent: html.String(),
	})
}
