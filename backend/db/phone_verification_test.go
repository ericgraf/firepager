package db

import "testing"

func TestNormalizeE164(t *testing.T) {
	got, err := NormalizeE164(" +1 (212) 555-0100 ")
	if err != nil || got != "+12125550100" {
		t.Fatalf("normalize phone = %q, %v", got, err)
	}
	for _, value := range []string{"2125550100", "+012345678", "+1-abc-5550100"} {
		if _, err := NormalizeE164(value); err == nil {
			t.Errorf("expected invalid phone %q", value)
		}
	}
}

func TestRandomOTPIsSixDigits(t *testing.T) {
	code, err := randomOTP()
	if err != nil || len(code) != 6 {
		t.Fatalf("OTP = %q, %v", code, err)
	}
}
