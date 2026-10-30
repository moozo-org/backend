package brevo

import "context"

type Contact struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

// Email is a transactional email. The sender must be verified in the Brevo
// account. At least one of TextContent and HTMLContent is required.
type Email struct {
	Sender      Contact   `json:"sender"`
	To          []Contact `json:"to"`
	Subject     string    `json:"subject"`
	TextContent string    `json:"textContent,omitempty"`
	HTMLContent string    `json:"htmlContent,omitempty"`
}

// SendEmail sends e through Brevo's transactional email API.
func (c *Client) SendEmail(ctx context.Context, e Email) error {
	return c.post(ctx, "/smtp/email", e)
}
