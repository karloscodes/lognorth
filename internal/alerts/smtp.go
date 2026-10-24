package alerts

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"lognorth/internal/settings"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// smtpTimeout bounds one email: the connection, the TLS handshake, and every
// command. The background jobs send the alerts, and a server that never
// answers would stop them for good. smtp.SendMail has no timeout.
const smtpTimeout = 30 * time.Second

// sendViaSMTP sends notification via email
func sendViaSMTP(s *settings.Settings, subject, body string) error {
	if !s.IsEmailConfigured() {
		return fmt.Errorf("SMTP not configured")
	}

	var auth smtp.Auth
	if s.SMTPUsername != "" && s.SMTPPassword != "" {
		auth = smtp.PlainAuth("", s.SMTPUsername, s.SMTPPassword, s.SMTPHost)
	}
	addr := net.JoinHostPort(s.SMTPHost, fmt.Sprint(s.SMTPPort))
	msg := buildMessage(s.SMTPFrom, s.NotifyEmail, subject, body)
	return sendMail(addr, s.SMTPHost, implicitTLS(s.SMTPPort), auth, s.SMTPFrom, s.NotifyEmail, msg, smtpTimeout)
}

// implicitTLS says whether a port speaks TLS from the first byte. Port 465
// does (SMTPS). The other ports start in plain text and upgrade with
// STARTTLS. smtp.SendMail knows only STARTTLS, so on 465 it waited for a
// greeting that never came.
func implicitTLS(port int) bool {
	return port == 465
}

// sendMail delivers one message, and gives up after timeout. It does what
// smtp.SendMail does: STARTTLS when the server offers it, AUTH, MAIL, RCPT,
// DATA.
func sendMail(addr, host string, useTLS bool, auth smtp.Auth, from, to string, msg []byte, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	dialer := &net.Dialer{Deadline: deadline}
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}

	var conn net.Conn
	var err error
	if useTLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); ok && !useTLS {
		if err := c.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if auth != nil {
		if ok, _ := c.Extension("AUTH"); !ok {
			return errors.New("smtp: the server does not support AUTH")
		}
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// buildMessage writes a plain-text email. Subjects carry error messages and
// an em dash, so the subject is an encoded word and the body is UTF-8 in
// quoted-printable, which also keeps each line under the 998 characters
// that SMTP allows. The Date and Message-ID are there because spam filters
// score a message without them.
func buildMessage(from, to, subject, body string) []byte {
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", from)
	fmt.Fprintf(&msg, "To: %s\r\n", to)
	fmt.Fprintf(&msg, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&msg, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&msg, "Message-ID: <%s@%s>\r\n", randomID(), domainOf(from))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	qp := quotedprintable.NewWriter(&msg)
	qp.Write([]byte(body))
	qp.Close()
	return msg.Bytes()
}

func randomID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// domainOf is the domain of an address, for the Message-ID.
func domainOf(address string) string {
	if at := strings.LastIndex(address, "@"); at >= 0 {
		return strings.Trim(address[at+1:], "> ")
	}
	return "lognorth"
}
