package mongodb

import (
	"context"

	"github.com/google/uuid"

	"moozo/moozo"
)

// PasswordResetCodesCollectionName stores verificationCodeDocuments too; see
// schemas/password_reset_codes.json.
const PasswordResetCodesCollectionName = "password_reset_codes"

var _ moozo.PasswordResetCodeRepository = (*Repository)(nil)

func (r *Repository) SavePasswordResetCode(ctx context.Context, c *moozo.VerificationCode) error {
	return codeStore{r.resetCodes}.save(ctx, c)
}

func (r *Repository) FindPasswordResetCode(ctx context.Context, userID uuid.UUID) (*moozo.VerificationCode, error) {
	return codeStore{r.resetCodes}.find(ctx, userID)
}

func (r *Repository) ConsumePasswordResetAttempt(ctx context.Context, userID uuid.UUID) (*moozo.VerificationCode, error) {
	return codeStore{r.resetCodes}.consume(ctx, userID)
}

func (r *Repository) DeletePasswordResetCode(ctx context.Context, userID uuid.UUID) (bool, error) {
	return codeStore{r.resetCodes}.delete(ctx, userID)
}
