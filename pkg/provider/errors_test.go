package provider

import (
	"errors"
	"testing"
	"time"
)

func TestRateLimitedErrorCarriesRetryAfter(t *testing.T) {
	err := &RateLimitedError{RetryAfter: 30 * time.Second, Message: "slow down"}
	var target *RateLimitedError
	if !errors.As(err, &target) {
		t.Fatal("RateLimitedError does not satisfy errors.As")
	}
	if target.RetryAfter != 30*time.Second {
		t.Fatalf("RetryAfter = %v", target.RetryAfter)
	}
	if err.Error() != "slow down" {
		t.Fatalf("Error() = %q", err.Error())
	}
}

func TestInsufficientFundsErrorMessage(t *testing.T) {
	err := &InsufficientFundsError{Message: "no credits"}
	if err.Error() != "no credits" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if (&InsufficientFundsError{}).Error() == "" {
		t.Fatal("a bare InsufficientFundsError needs a default message")
	}
}

func TestRateLimitedfCarriesTheWindow(t *testing.T) {
	err := RateLimitedf(5*time.Second, "retry in %ds", 5)
	var target *RateLimitedError
	if !errors.As(err, &target) || target.RetryAfter != 5*time.Second || target.Message != "retry in 5s" {
		t.Fatalf("RateLimitedf = %v", err)
	}
}
