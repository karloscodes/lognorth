package tests

import (
	"bufio"
	"mime"
	"net"
	"strings"
	"testing"

	"lognorth/internal/alerts"
	"lognorth/internal/settings"
)

// smtpInbox is a local SMTP server that keeps the DATA of each message.
func smtpInbox(t *testing.T) (host string, port int, messages chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	messages = make(chan string, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		reply := func(line string) { conn.Write([]byte(line + "\r\n")) }

		reply("220 test")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "DATA"):
				reply("354 go on")
				var data strings.Builder
				for {
					l, err := r.ReadString('\n')
					if err != nil || l == ".\r\n" {
						break
					}
					data.WriteString(l)
				}
				messages <- data.String()
				reply("250 ok")
			case strings.HasPrefix(cmd, "QUIT"):
				reply("221 bye")
				return
			default:
				reply("250 ok")
			}
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port, messages
}

// Alert subjects carry event data, such as an endpoint path sent by an SDK.
func TestAlertSubjectCannotAddEmailHeaders(t *testing.T) {
	host, port, messages := smtpInbox(t)
	s := &settings.Settings{NotifyChannel: "email", SMTPHost: host, SMTPPort: port,
		SMTPFrom: "alerts@example.com", NotifyEmail: "owner@example.com"}
	path := "/x\r\nReply-To: attacker@example.test\r\n\r\nforged body"

	err := alerts.Send(s, "shop — spike: "+path+" at 50.0%", "body")

	if err != nil {
		t.Fatalf("send: %v", err)
	}
	msg := <-messages
	headers, _, _ := strings.Cut(msg, "\r\n\r\n")
	if strings.Contains(headers, "\r\nReply-To:") {
		t.Errorf("the path added a header:\n%s", headers)
	}
	// The subject is an encoded word (it holds an em dash): decode it.
	var subject string
	for _, line := range strings.Split(headers, "\r\n") {
		if value, ok := strings.CutPrefix(line, "Subject: "); ok {
			subject, _ = new(mime.WordDecoder).DecodeHeader(value)
		}
	}
	if subject != "shop — spike: /x Reply-To: attacker@example.test  forged body at 50.0%" {
		t.Errorf("subject = %q in headers %q, want the whole path on the Subject line", subject, headers)
	}
}
