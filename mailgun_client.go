package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	mailgun "github.com/mailgun/mailgun-go/v4"
)

func sendPasswordResetEmail(toEmail, token string) error {
	domain := os.Getenv("MAILGUN_DOMAIN")
	apiKey := os.Getenv("MAILGUN_API_KEY")
	from := os.Getenv("MAILGUN_FROM")
	resetBase := os.Getenv("PASSWORD_RESET_URL")

	if domain == "" || apiKey == "" || from == "" || resetBase == "" {
		return fmt.Errorf("mailgun environment variables are not fully configured")
	}

	resetURL, err := buildResetURL(resetBase, token)
	if err != nil {
		return err
	}

	mg := mailgun.NewMailgun(domain, apiKey)
	body := fmt.Sprintf("Please reset your password by visiting %s", resetURL)
	message := mg.NewMessage(from, "Password reset request", body, toEmail)
	message.SetHtml(fmt.Sprintf("<p>Please reset your password by clicking <a href=\"%s\">this link</a>.</p>", resetURL))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _, err = mg.Send(ctx, message)
	if err != nil {
		return fmt.Errorf("send mailgun message: %w", err)
	}

	log.Printf("Password reset email sent to %s", toEmail)
	return nil
}

func buildResetURL(base, token string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid PASSWORD_RESET_URL: %w", err)
	}
	q := parsed.Query()
	q.Set("token", token)
	parsed.RawQuery = q.Encode()
	return parsed.String(), nil
}
