package handlers

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// emailSem bounds how many export emails may be sent concurrently. Exports are
// fire-and-forget, so without a cap a burst of large exports would spawn
// unbounded goroutines each holding a full file copy in memory.
var emailSem = make(chan struct{}, 4)

const emailSendTimeout = 30 * time.Second

// dispatchEmail runs an email send on the bounded worker set with panic
// recovery. If all slots are busy the send is dropped (and logged) rather than
// leaking a goroutine — the export itself already succeeded for the client.
func dispatchEmail(name string, fn func()) {
	select {
	case emailSem <- struct{}{}:
	default:
		log.Printf("Email queue full, dropping send: %s", name)
		return
	}
	go func() {
		defer func() {
			<-emailSem
			if rec := recover(); rec != nil {
				log.Printf("PANIC in email send %s: %v", name, rec)
			}
		}()
		fn()
	}()
}

// sendMailWithTimeout is like smtp.SendMail but every network step has a
// deadline. The stdlib smtp.SendMail can block forever on a stalled server,
// which would leak the goroutine and its buffered attachment indefinitely.
func sendMailWithTimeout(addr, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", addr, emailSendTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(emailSendTimeout))

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Close()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return err
		}
	}
	if auth != nil {
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(auth); err != nil {
				return err
			}
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	wc, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(msg); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return c.Quit()
}

var emailReceivers = []string{
	"yogendra.maurya@kabuprojects.com",
	"rampratap.singh@kabuprojects.com",
}

const (
	companyName    = "Grain Technik"
	companyWebsite = "https://graintechnik.com"
	companyPhone   = "+91-9217845040"
	companyEmail   = "service@graintechnik.com"
	senderName     = "Grain Technik RMS"
	senderEmail    = "noreply.rms@graintechnik.com"
)

// sendExcelEmail sends the exported Excel file as an email attachment
func sendExcelEmail(excelData []byte, filename string, tableName string, fromDate string, toDate string, rowCount int) {
	from := os.Getenv("EMAIL_FROM")
	if from == "" {
		from = senderEmail
	}
	pass := os.Getenv("EMAIL_PASS")
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")

	if pass == "" {
		log.Println("Email not configured (no EMAIL_PASS), skipping email send")
		return
	}

	if smtpHost == "" {
		smtpHost = "smtp.gmail.com"
	}
	if smtpPort == "" {
		smtpPort = "587"
	}

	to := emailReceivers
	subject := fmt.Sprintf("[%s] Data Export Report - %s", companyName, tableName)

	// HTML email body
	body := fmt.Sprintf(`<html>
<body style="font-family: Arial, sans-serif; color: #333; margin: 0; padding: 20px; background-color: #f5f5f5;">
  <div style="max-width: 600px; margin: 0 auto; background: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.1);">

    <!-- Header -->
    <div style="background: linear-gradient(135deg, #1a5276, #2e86c1); padding: 24px 30px; text-align: center;">
      <h1 style="color: #ffffff; margin: 0; font-size: 22px; letter-spacing: 1px;">%s</h1>
      <p style="color: #d4e6f1; margin: 6px 0 0 0; font-size: 13px;">Remote Monitoring System</p>
    </div>

    <!-- Content -->
    <div style="padding: 30px;">
      <h2 style="color: #1a5276; margin-top: 0; font-size: 18px;">Data Export Report</h2>

      <table style="width: 100%%; border-collapse: collapse; margin: 16px 0;">
        <tr style="background-color: #f8f9fa;">
          <td style="padding: 10px 14px; border: 1px solid #dee2e6; font-weight: bold; color: #555; width: 140px;">Machine / Table</td>
          <td style="padding: 10px 14px; border: 1px solid #dee2e6;">%s</td>
        </tr>
        <tr>
          <td style="padding: 10px 14px; border: 1px solid #dee2e6; font-weight: bold; color: #555;">Date Range</td>
          <td style="padding: 10px 14px; border: 1px solid #dee2e6;">%s to %s</td>
        </tr>
        <tr style="background-color: #f8f9fa;">
          <td style="padding: 10px 14px; border: 1px solid #dee2e6; font-weight: bold; color: #555;">Total Records</td>
          <td style="padding: 10px 14px; border: 1px solid #dee2e6;">%d</td>
        </tr>
        <tr>
          <td style="padding: 10px 14px; border: 1px solid #dee2e6; font-weight: bold; color: #555;">Generated At</td>
          <td style="padding: 10px 14px; border: 1px solid #dee2e6;">%s</td>
        </tr>
      </table>

      <p style="color: #555; font-size: 14px; line-height: 1.6;">
        Please find the exported data attached as an Excel file.<br>
        The file contains filtered and deduplicated records with timestamps in the machine's local timezone.
      </p>

      <div style="background-color: #eaf2f8; border-left: 4px solid #2e86c1; padding: 12px 16px; margin: 20px 0; border-radius: 0 4px 4px 0;">
        <p style="margin: 0; color: #1a5276; font-size: 13px;">
          <strong>Attachment:</strong> %s
        </p>
      </div>
    </div>

    <!-- Footer -->
    <div style="background-color: #f8f9fa; padding: 20px 30px; border-top: 1px solid #dee2e6;">
      <p style="margin: 0 0 6px 0; color: #1a5276; font-weight: bold; font-size: 14px;">%s</p>
      <p style="margin: 0; color: #777; font-size: 12px; line-height: 1.8;">
        %s | %s<br>
        <a href="%s" style="color: #2e86c1; text-decoration: none;">%s</a>
      </p>
      <p style="margin: 12px 0 0 0; color: #aaa; font-size: 11px;">
        This is an automated email from %s. Please do not reply to this email.
      </p>
    </div>
  </div>
</body>
</html>`,
		companyName,
		tableName,
		fromDate, toDate,
		rowCount,
		time.Now().Format("2006-01-02 15:04:05 MST"),
		filename,
		companyName,
		companyPhone, companyEmail,
		companyWebsite, companyWebsite,
		senderName,
	)

	// Build MIME email
	var msg bytes.Buffer
	boundary := "==GRAIN_EXPORT_BOUNDARY=="

	// Headers
	msg.WriteString(fmt.Sprintf("From: \"%s\" <%s>\r\n", senderName, from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(to, ",")))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary))
	msg.WriteString("\r\n")

	// HTML body
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
	msg.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)
	msg.WriteString("\r\n")

	// Excel attachment
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString(fmt.Sprintf("Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet; name=\"%s\"\r\n", filename))
	msg.WriteString("Content-Transfer-Encoding: base64\r\n")
	msg.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", filename))
	msg.WriteString("\r\n")

	encoded := base64.StdEncoding.EncodeToString(excelData)
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		msg.WriteString(encoded[i:end])
		msg.WriteString("\r\n")
	}

	msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	// Send email
	auth := smtp.PlainAuth("", from, pass, smtpHost)
	addr := smtpHost + ":" + smtpPort

	err := sendMailWithTimeout(addr, smtpHost, auth, from, to, msg.Bytes())
	if err != nil {
		log.Printf("Failed to send export email: %v", err)
	} else {
		log.Printf("Export email sent to %v for %s", to, filename)
	}
}

// sendCSVEmail kept for backward compatibility with CSV export
func sendCSVEmail(csvData []byte, filename string, tableName string) {
	from := os.Getenv("EMAIL_FROM")
	if from == "" {
		from = senderEmail
	}
	pass := os.Getenv("EMAIL_PASS")
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")

	if pass == "" {
		log.Println("Email not configured, skipping email send")
		return
	}

	if smtpHost == "" {
		smtpHost = "smtp.gmail.com"
	}
	if smtpPort == "" {
		smtpPort = "587"
	}

	to := emailReceivers
	subject := fmt.Sprintf("[%s] Data Export - %s", companyName, tableName)
	body := fmt.Sprintf("Please find attached the data export for: %s\n\nFile: %s\n\n--%s\n%s | %s\n%s", tableName, filename, companyName, companyPhone, companyEmail, companyWebsite)

	var msg bytes.Buffer
	boundary := "==GRAIN_EXPORT_CSV_BOUNDARY=="

	msg.WriteString(fmt.Sprintf("From: \"%s\" <%s>\r\n", senderName, from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(to, ",")))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary))
	msg.WriteString("\r\n")

	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)
	msg.WriteString("\r\n")

	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString(fmt.Sprintf("Content-Type: text/csv; name=\"%s\"\r\n", filename))
	msg.WriteString("Content-Transfer-Encoding: base64\r\n")
	msg.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", filename))
	msg.WriteString("\r\n")

	encoded := base64.StdEncoding.EncodeToString(csvData)
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		msg.WriteString(encoded[i:end])
		msg.WriteString("\r\n")
	}

	msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	auth := smtp.PlainAuth("", from, pass, smtpHost)
	addr := smtpHost + ":" + smtpPort

	err := sendMailWithTimeout(addr, smtpHost, auth, from, to, msg.Bytes())
	if err != nil {
		log.Printf("Failed to send CSV export email: %v", err)
	} else {
		log.Printf("CSV export email sent to %v for %s", to, filename)
	}
}
