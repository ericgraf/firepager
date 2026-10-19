package db

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrVerificationRateLimited = errors.New("verification rate limit exceeded")
	ErrVerificationInvalid     = errors.New("invalid or expired verification code")
	ErrWhatsAppNotEligible     = errors.New("whatsapp requires a verified phone number and consent")
)

func IsE164(number string) bool {
	if len(number) < 9 || len(number) > 16 || number[0] != '+' || number[1] == '0' {
		return false
	}
	for _, r := range number[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// NormalizeE164 accepts separators but never guesses a country code.
func NormalizeE164(number string) (string, error) {
	number = strings.TrimSpace(number)
	var b strings.Builder
	for i, r := range number {
		if i == 0 && r == '+' {
			b.WriteRune(r)
			continue
		}
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		if strings.ContainsRune(" -().", r) {
			continue
		}
		return "", fmt.Errorf("invalid E.164 phone number")
	}
	if !IsE164(b.String()) {
		return "", fmt.Errorf("invalid E.164 phone number")
	}
	return b.String(), nil
}

func randomOTP() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	value := (uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])) % 1000000
	return fmt.Sprintf("%06d", value), nil
}

// CreatePhoneVerificationChallenge stores only a bcrypt hash. The code is
// deliberately not returned; a delivery adapter can be attached internally.
func CreatePhoneVerificationChallenge(userID int) error {
	var phone string
	if err := DB.QueryRow(`SELECT phone_number FROM users WHERE id = $1`, userID).Scan(&phone); err != nil {
		return err
	}
	var recent int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM phone_verification_challenges WHERE user_id=$1 AND created_at > CURRENT_TIMESTAMP - INTERVAL '60 seconds'`, userID).Scan(&recent); err != nil {
		return err
	}
	if recent > 0 {
		return ErrVerificationRateLimited
	}
	var daily int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM phone_verification_challenges WHERE user_id=$1 AND created_at > CURRENT_TIMESTAMP - INTERVAL '24 hours'`, userID).Scan(&daily); err != nil {
		return err
	}
	if daily >= 10 {
		return ErrVerificationRateLimited
	}
	code, err := randomOTP()
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = DB.Exec(`UPDATE phone_verification_challenges SET consumed_at=CURRENT_TIMESTAMP WHERE user_id=$1 AND consumed_at IS NULL`, userID)
	if err != nil {
		return err
	}
	_, err = DB.Exec(`INSERT INTO phone_verification_challenges (user_id, phone_number, code_hash, expires_at) VALUES ($1,$2,$3,CURRENT_TIMESTAMP + INTERVAL '10 minutes')`, userID, phone, string(hash))
	if err == nil {
		_, err = DB.Exec(`UPDATE users SET phone_verification_status='pending' WHERE id=$1 AND phone_number=$2 AND phone_verification_status <> 'verified'`, userID, phone)
	}
	return err
}

func ConfirmPhoneVerification(userID int, code string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	var hash, phone string
	var expires time.Time
	var attempts int
	err = tx.QueryRow(`SELECT id, phone_number, code_hash, expires_at, attempts FROM phone_verification_challenges WHERE user_id=$1 AND consumed_at IS NULL ORDER BY id DESC LIMIT 1 FOR UPDATE`, userID).Scan(&id, &phone, &hash, &expires, &attempts)
	if err != nil || time.Now().After(expires) || attempts >= 5 {
		return ErrVerificationInvalid
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(code)) != nil {
		_, _ = tx.Exec(`UPDATE phone_verification_challenges SET attempts=attempts+1 WHERE id=$1`, id)
		return ErrVerificationInvalid
	}
	now := time.Now()
	if _, err = tx.Exec(`UPDATE phone_verification_challenges SET consumed_at=$1 WHERE id=$2`, now, id); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE users SET phone_number=$1, phone_verification_status='verified', phone_verified_at=$2 WHERE id=$3`, phone, now, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func SetWhatsAppConsent(userID int, consented bool, policyVersion string) error {
	policyVersion = strings.TrimSpace(policyVersion)
	if policyVersion == "" {
		return errors.New("consent policy version is required")
	}
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var phone string
	if err = tx.QueryRow(`SELECT phone_number FROM users WHERE id=$1`, userID).Scan(&phone); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO whatsapp_consent_audit (user_id, phone_number, consented, policy_version) VALUES ($1,$2,$3,$4)`, userID, phone, consented, policyVersion); err != nil {
		return err
	}
	if consented {
		_, err = tx.Exec(`UPDATE users SET whatsapp_consented=TRUE, whatsapp_consent_at=CURRENT_TIMESTAMP, whatsapp_consent_policy_version=$1 WHERE id=$2`, policyVersion, userID)
	} else {
		_, err = tx.Exec(`UPDATE users SET whatsapp_consented=FALSE, whatsapp_consent_at=CURRENT_TIMESTAMP, whatsapp_consent_policy_version=$1 WHERE id=$2`, policyVersion, userID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func WhatsAppEligible(userID int) (bool, error) {
	var verified, consented bool
	err := DB.QueryRow(`SELECT phone_verification_status='verified', whatsapp_consented FROM users WHERE id=$1`, userID).Scan(&verified, &consented)
	return verified && consented, err
}
