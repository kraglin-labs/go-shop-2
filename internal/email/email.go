package email

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

type Client struct {
	apiKey     string
	fromEmail  string
	adminEmail string
	httpClient *http.Client
}

func New(apiKey, fromEmail, adminEmail string) *Client {
	return &Client{
		apiKey:     apiKey,
		fromEmail:  fromEmail,
		adminEmail: adminEmail,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type brevoEmail struct {
	Sender      brevoSender   `json:"sender"`
	To          []brevoTo     `json:"to"`
	Subject     string        `json:"subject"`
	HTMLContent string        `json:"htmlContent"`
}

type brevoSender struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type brevoTo struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func Generate7DigitCode() (rawCode, codeHash string, err error) {
	b := make([]byte, 4)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	val := 1000000 + int(binary.BigEndian.Uint32(b))%9000000
	rawCode = strconv.Itoa(val)
	sum := sha256.Sum256([]byte(rawCode))
	codeHash = hex.EncodeToString(sum[:])
	return rawCode, codeHash, nil
}

func (c *Client) send(to, subject, html string) error {
	payload := brevoEmail{
		Sender:      brevoSender{Email: c.fromEmail, Name: "Go Shop 2"},
		To:          []brevoTo{{Email: to, Name: to}},
		Subject:     subject,
		HTMLContent: html,
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "https://api.brevo.com/v3/smtp/email", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// FIX: Brevo requires lowercase "api-key". Go canonicalizes to "Api-Key" via Set().
	// We must write directly to the map to preserve case.
	req.Header["api-key"] = []string{c.apiKey}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("BREVO HTTP ERROR: %v", err)
		return fmt.Errorf("brevo request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("BREVO RESPONSE: status=%d body=%s", resp.StatusCode, string(respBody))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("brevo error: %s", string(respBody))
	}
	return nil
}

func (c *Client) SendEmailVerification(to, code string) error {
	subject := "Verify Your Email"
	html := fmt.Sprintf(`
		<h2>Email Verification</h2>
		<p>Your verification code is:</p>
		<h1 style="font-size:48px;letter-spacing:8px;">%s</h1>
		<p>This code expires in 1 hour. If you did not request this, ignore this email.</p>
	`, code)
	return c.send(to, subject, html)
}

func (c *Client) SendPasswordReset(to, code string) error {
	subject := "Password Reset Code"
	html := fmt.Sprintf(`
		<h2>Password Reset</h2>
		<p>Your password reset code is:</p>
		<h1 style="font-size:48px;letter-spacing:8px;">%s</h1>
		<p>This code expires in 1 hour. If you did not request this, ignore this email.</p>
	`, code)
	return c.send(to, subject, html)
}

func (c *Client) SendOrderConfirmation(userEmail string, orderID int32, total int64, items []string) error {
	subject := fmt.Sprintf("Order #%d Confirmed", orderID)
	itemsList := ""
	for _, item := range items {
		itemsList += "<li>" + item + "</li>"
	}
	html := fmt.Sprintf(`
		<h2>Thank you for your order!</h2>
		<p>Your order <strong>#%d</strong> has been placed successfully.</p>
		<p><strong>Total:</strong> $%.2f</p>
		<h3>Items:</h3>
		<ul>%s</ul>
		<p>We will notify you when your order ships.</p>
	`, orderID, float64(total)/100, itemsList)
	return c.send(userEmail, subject, html)
}

func (c *Client) SendNewOrderAdmin(orderID int32, total int64, userEmail string, items []string) error {
	subject := fmt.Sprintf("New Order #%d - $%.2f", orderID, float64(total)/100)
	itemsList := ""
	for _, item := range items {
		itemsList += "<li>" + item + "</li>"
	}
	html := fmt.Sprintf(`
		<h2>New Order Received</h2>
		<p><strong>Order #%d</strong></p>
		<p><strong>Customer:</strong> %s</p>
		<p><strong>Total:</strong> $%.2f</p>
		<h3>Items:</h3>
		<ul>%s</ul>
	`, orderID, userEmail, float64(total)/100, itemsList)
	return c.send(c.adminEmail, subject, html)
}

func (c *Client) SendOrderStatusUpdate(userEmail string, orderID int32, status string) error {
	subject := fmt.Sprintf("Order #%d Update", orderID)
	var message string
	switch status {
	case "processing":
		message = "Your order is being processed and prepared for shipment."
	case "shipped":
		message = "Your order has been shipped! You will receive tracking information shortly."
	case "delivered":
		message = "Your order has been delivered. Enjoy!"
	case "cancelled":
		message = "Your order has been cancelled. If you have questions, please contact support."
	default:
		message = fmt.Sprintf("Your order status has been updated to: %s", status)
	}
	html := fmt.Sprintf(`
		<h2>Order Update</h2>
		<p>Your order <strong>#%d</strong> has been updated.</p>
		<p><strong>Status:</strong> %s</p>
		<p>%s</p>
	`, orderID, status, message)
	return c.send(userEmail, subject, html)
}
