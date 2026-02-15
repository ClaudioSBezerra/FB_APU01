package services

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

// EmailConfig holds SMTP configuration
type EmailConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// GetEmailConfig returns SMTP configuration from environment or defaults
func GetEmailConfig() *EmailConfig {
	portStr := os.Getenv("SMTP_PORT")
	port := 465
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}

	host := os.Getenv("SMTP_HOST")
	if host == "" {
		host = "smtp.hostinger.com"
	}

	username := os.Getenv("SMTP_USER")
	password := os.Getenv("SMTP_PASSWORD")

	from := os.Getenv("SMTP_FROM")
	if from == "" {
		from = "FBTax Cloud <noreply@fbtax.cloud>"
	}

	return &EmailConfig{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		From:     from,
	}
}

// sendMailSSL sends email over implicit TLS (port 465)
func sendMailSSL(config *EmailConfig, to []string, msg []byte) error {
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)

	tlsConfig := &tls.Config{
		ServerName: config.Host,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("TLS dial failed: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, config.Host)
	if err != nil {
		return fmt.Errorf("SMTP client creation failed: %w", err)
	}
	defer client.Quit()

	auth := smtp.PlainAuth("", config.Username, config.Password, config.Host)
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP auth failed: %w", err)
	}

	if err = client.Mail(config.Username); err != nil {
		return fmt.Errorf("SMTP MAIL FROM failed: %w", err)
	}

	for _, recipient := range to {
		if err = client.Rcpt(recipient); err != nil {
			return fmt.Errorf("SMTP RCPT TO failed for %s: %w", recipient, err)
		}
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA failed: %w", err)
	}

	_, err = writer.Write(msg)
	if err != nil {
		return fmt.Errorf("SMTP write failed: %w", err)
	}

	return writer.Close()
}

// SendPasswordResetEmail sends a password reset email to the user
func SendPasswordResetEmail(email, resetToken string) error {
	config := GetEmailConfig()

	if config.Password == "" {
		log.Printf("[Email Service] SMTP not configured. Skipping email send to %s", email)
		return fmt.Errorf("serviço de e-mail não configurado - configure SMTP_PASSWORD")
	}

	// Use APP_URL env var for the reset link (defaults to production)
	appURL := os.Getenv("APP_URL")
	if appURL == "" {
		appURL = "https://fbtax.cloud"
	}
	resetLink := fmt.Sprintf("%s/reset-senha?token=%s", appURL, resetToken)

	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: FBTax Cloud - =?UTF-8?B?UmVkZWZpbmnDp8OjbyBkZSBTZW5oYQ==?=\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n"+
		`<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<style>
		body { font-family: Arial, sans-serif; line-height: 1.6; color: #333333; max-width: 600px; margin: 0 auto; }
		.container { background-color: #f4f4f8; padding: 40px; border-radius: 8px; }
		.header { background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); color: white; padding: 20px; border-radius: 8px; text-align: center; }
		.logo { font-size: 24px; font-weight: bold; }
		.content { background: white; padding: 30px; border-radius: 8px; }
		h1 { color: #333; margin-bottom: 20px; }
		p { margin: 0 0 15px 0; color: #666; line-height: 1.8; }
		.button { display: inline-block; padding: 12px 24px; background: #2c3e50; color: white; text-decoration: none; border-radius: 4px; font-weight: bold; }
		.footer { background: #f8f9fa; padding: 20px; border-radius: 8px; color: #666; font-size: 12px; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<div class="logo">FBTax Cloud</div>
			<h1 style="color: white;">Redefinição de Senha</h1>
		</div>
		<div class="content">
			<p>Olá,</p>
			<p>Recebemos uma solicitação de redefinição de senha para sua conta no FBTax Cloud.</p>
			<p>Se você não solicitou esta alteração, por favor ignore este e-mail.</p>
			<div style="text-align: center; margin: 30px 0;">
				<a href="%s" class="button">Redefinir Minha Senha</a>
			</div>
			<p style="margin: 30px 0; font-size: 14px; color: #666;">
				Ou copie e cole o link no seu navegador:<br>
				<strong style="color: #2c3e50;">%s</strong>
			</p>
			<p style="font-size: 12px; color: #999;">Este link expira em 1 hora por motivos de segurança.</p>
			<p style="font-size: 12px; color: #999;">Se você não solicitou esta redefinição, entre em contato com o suporte.</p>
		</div>
		<div class="footer">
			<p>&copy; 2026 FBTax Cloud - Todos os direitos reservados</p>
		</div>
	</div>
</body>
</html>
`, config.From, email, resetLink, resetLink)

	log.Printf("[Email Service] Sending password reset email to %s via %s:%d", email, config.Host, config.Port)

	var err error
	if config.Port == 465 {
		err = sendMailSSL(config, []string{email}, []byte(message))
	} else {
		addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
		auth := smtp.PlainAuth("", config.Username, config.Password, config.Host)
		err = smtp.SendMail(addr, auth, config.Username, []string{email}, []byte(message))
	}

	if err != nil {
		log.Printf("[Email Service] Failed to send email to %s: %v", email, err)
		return fmt.Errorf("falha ao enviar e-mail: %w", err)
	}

	log.Printf("[Email Service] Password reset email sent successfully to %s", email)
	return nil
}

// SendAIReportEmail sends AI-generated executive summary to company managers
func SendAIReportEmail(recipients []string, companyName, periodo, narrativaMarkdown, dadosBrutosJSON string) error {
	config := GetEmailConfig()

	if config.Password == "" {
		log.Printf("[Email Service] SMTP not configured. Skipping AI report email to %d recipients", len(recipients))
		return fmt.Errorf("servico de e-mail nao configurado - configure SMTP_PASSWORD")
	}

	if len(recipients) == 0 {
		log.Printf("[Email Service] No recipients for AI report email")
		return nil
	}

	// Use APP_URL env var for dashboard link
	appURL := os.Getenv("APP_URL")
	if appURL == "" {
		appURL = "http://localhost:3000"
	}

	// Convert markdown to simple HTML for email
	narrativaHTML := convertMarkdownToHTML(narrativaMarkdown)

	// Keep markdown for plain text version (strip HTML tags)
	narrativaPlain := stripHTMLTags(narrativaHTML)

	// Send individual emails to each recipient (same pattern as password reset)
	for _, email := range recipients {
		// Generate unique boundary for multipart
		boundary := fmt.Sprintf("boundary_%d", time.Now().Unix())

		// Build multipart message (HTML + Plain Text for better corporate email acceptance)
		message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: FBTax Cloud - Resumo Executivo - %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", config.From, email, periodo, boundary)

		// Plain text version (for corporate spam filters and text-only clients)
		message += fmt.Sprintf("--%s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n", boundary)
		message += fmt.Sprintf("FBTax Cloud - Resumo Executivo\n\nEmpresa: %s\nPeriodo: %s\nGerado em: %s\n\n%s\n\nAcesse o painel completo: %s\n\n---\n(c) 2026 FBTax Cloud - Todos os direitos reservados\n",
			companyName, periodo, getTimeBrasil(), narrativaPlain, appURL)
		message += fmt.Sprintf("\r\n--%s\r\n", boundary)

		// HTML version (for rich display)
		message += "Content-Type: text/html; charset=UTF-8\r\n\r\n"
		message += fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<style>
		body { font-family: Arial, sans-serif; line-height: 1.6; color: #333333; max-width: 600px; margin: 0 auto; }
		.container { background-color: #f4f4f8; padding: 40px; border-radius: 8px; }
		.header { background: #4a5568; color: white; padding: 20px; border-radius: 8px; text-align: center; }
		.logo { font-size: 24px; font-weight: bold; }
		.content { background: white; padding: 30px; border-radius: 8px; margin-top: 20px; }
		h2 { color: #333; margin-bottom: 20px; }
		p { margin: 0 0 15px 0; color: #666; line-height: 1.8; }
		.info-box { background-color: #e7f3ff; border-left: 4px solid #2196f3; padding: 15px; margin: 20px 0; }
		.button { display: inline-block; padding: 12px 24px; background: #2c3e50; color: white; text-decoration: none; border-radius: 4px; font-weight: bold; margin: 20px 0; }
		.footer { background: #f8f9fa; padding: 20px; border-radius: 8px; color: #666; font-size: 12px; margin-top: 20px; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<div class="logo">FBTax Cloud</div>
			<h2 style="color: white; margin-top: 10px;">Resumo Executivo - %s</h2>
		</div>
		<div class="content">
			<div class="info-box">
				<strong>Empresa:</strong> %s<br>
				<strong>Periodo:</strong> %s<br>
				<strong>Gerado em:</strong> %s
			</div>
			%s
			<div style="text-align: center; margin: 30px 0;">
				<a href="%s" class="button">Acessar Painel Completo</a>
			</div>
		</div>
		<div class="footer">
			<p>&copy; 2026 FBTax Cloud - Todos os direitos reservados</p>
		</div>
	</div>
</body>
</html>`, periodo, companyName, periodo, getTimeBrasil(), narrativaHTML, appURL)
		message += fmt.Sprintf("\r\n--%s--\r\n", boundary)

		log.Printf("[Email Service] Sending AI report email to %s via %s:%d", email, config.Host, config.Port)

		var err error
		if config.Port == 465 {
			err = sendMailSSL(config, []string{email}, []byte(message))
		} else {
			addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
			auth := smtp.PlainAuth("", config.Username, config.Password, config.Host)
			err = smtp.SendMail(addr, auth, config.Username, []string{email}, []byte(message))
		}

		if err != nil {
			log.Printf("[Email Service] Failed to send AI report email to %s: %v", email, err)
			return fmt.Errorf("falha ao enviar e-mail de relatorio IA: %w", err)
		}

		log.Printf("[Email Service] AI report email sent successfully to %s", email)
	}

	return nil
}

// convertMarkdownToHTML converts basic markdown to HTML for email rendering
// stripHTMLTags removes HTML tags for plain text email version
func stripHTMLTags(html string) string {
	// Simple HTML tag removal for plain text version
	result := html
	result = strings.ReplaceAll(result, "<p>", "")
	result = strings.ReplaceAll(result, "</p>", "\n")
	result = strings.ReplaceAll(result, "<br>", "\n")
	result = strings.ReplaceAll(result, "<br/>", "\n")
	result = strings.ReplaceAll(result, "<br />", "\n")
	result = strings.ReplaceAll(result, "<h2>", "\n\n")
	result = strings.ReplaceAll(result, "</h2>", "\n")
	result = strings.ReplaceAll(result, "<h3>", "\n")
	result = strings.ReplaceAll(result, "</h3>", "\n")
	result = strings.ReplaceAll(result, "<strong>", "")
	result = strings.ReplaceAll(result, "</strong>", "")
	result = strings.ReplaceAll(result, "<em>", "")
	result = strings.ReplaceAll(result, "</em>", "")
	result = strings.ReplaceAll(result, "<ul>", "\n")
	result = strings.ReplaceAll(result, "</ul>", "\n")
	result = strings.ReplaceAll(result, "<ol>", "\n")
	result = strings.ReplaceAll(result, "</ol>", "\n")
	result = strings.ReplaceAll(result, "<li>", "  - ")
	result = strings.ReplaceAll(result, "</li>", "\n")
	return result
}

func convertMarkdownToHTML(markdown string) string {
	// Simple markdown to HTML conversion
	// For production, consider using a proper markdown library like github.com/gomarkdown/markdown
	html := markdown
	lines := strings.Split(html, "\n")

	var result strings.Builder
	inList := false
	inCodeBlock := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Code blocks
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			if inCodeBlock {
				result.WriteString("<div class=\"code-block\">")
			} else {
				result.WriteString("</div>")
			}
			continue
		}

		if inCodeBlock {
			result.WriteString(line + "<br>")
			continue
		}

		// Headers
		if strings.HasPrefix(trimmed, "### ") {
			if inList {
				result.WriteString("</ul>")
				inList = false
			}
			text := strings.TrimPrefix(trimmed, "### ")
			result.WriteString(fmt.Sprintf("<h3>%s</h3>", text))
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			if inList {
				result.WriteString("</ul>")
				inList = false
			}
			text := strings.TrimPrefix(trimmed, "## ")
			result.WriteString(fmt.Sprintf("<h2>%s</h2>", text))
			continue
		}

		// Lists
		if strings.HasPrefix(trimmed, "- ") {
			if !inList {
				result.WriteString("<ul>")
				inList = true
			}
			text := strings.TrimPrefix(trimmed, "- ")
			result.WriteString(fmt.Sprintf("<li>%s</li>", text))
			continue
		}

		// Close list if needed
		if inList && trimmed == "" {
			result.WriteString("</ul>")
			inList = false
			continue
		}

		// Bold text
		line = strings.ReplaceAll(line, "**", "<strong>")
		// Italic text
		line = strings.ReplaceAll(line, "_", "<em>")
		line = strings.ReplaceAll(line, "<em><em>", "")
		line = strings.ReplaceAll(line, "</em></em>", "")

		// Regular paragraph
		if trimmed != "" {
			if inList {
				result.WriteString("</ul>")
				inList = false
			}
			result.WriteString(fmt.Sprintf("<p>%s</p>", line))
		}

		// Add line break unless it's the last line
		if i < len(lines)-1 {
			result.WriteString("\n")
		}
	}

	if inList {
		result.WriteString("</ul>")
	}

	return result.String()
}

// getTimeBrasil returns current time in Brazil timezone formatted
func getTimeBrasil() string {
	// Brazil time is UTC-3
	loc := time.FixedZone("BRT", -3*3600)
	return time.Now().In(loc).Format("02/01/2006 as 15:04")
}
