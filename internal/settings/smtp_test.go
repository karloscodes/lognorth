package settings

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeSMTP is a server on localhost that speaks just enough SMTP for one
// message, and keeps what the client sent after DATA.
func fakeSMTP(t *testing.T) (addr string, message chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	message = make(chan string, 1)

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		reply := func(line string) { conn.Write([]byte(line + "\r\n")) }
		reply("220 fake")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			switch command := strings.ToUpper(strings.Fields(line + " x")[0]); command {
			case "EHLO", "HELO", "MAIL", "RCPT":
				reply("250 ok")
			case "DATA":
				reply("354 go on")
				var data strings.Builder
				for {
					line, err := r.ReadString('\n')
					if err != nil || line == ".\r\n" {
						break
					}
					data.WriteString(line)
				}
				message <- data.String()
				reply("250 queued")
			case "QUIT":
				reply("221 bye")
				return
			default:
				reply("502 no")
			}
		}
	}()
	return listener.Addr().String(), message
}

func TestSendMail(t *testing.T) {
	t.Run("sends a message with a date, a charset, and an encoded subject", func(t *testing.T) {
		addr, message := fakeSMTP(t)
		msg := buildMessage("alerts@example.com", "me@example.com", "checkout — new: TypeError", "TypeError: boom\nat app.ts:42")

		err := sendMail(addr, "127.0.0.1", false, nil, "alerts@example.com", "me@example.com", msg, 5*time.Second)

		if err != nil {
			t.Fatalf("sendMail: %v", err)
		}
		got := <-message
		for _, want := range []string{
			"Subject: =?utf-8?q?checkout_=E2=80=94_new:_TypeError?=\r\n",
			"Content-Type: text/plain; charset=utf-8\r\n",
			"MIME-Version: 1.0\r\n",
			"Date: ",
			"Message-ID: <",
			"TypeError: boom\r\nat app.ts:42",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("message lacks %q:\n%s", want, got)
			}
		}
	})

	t.Run("gives up on a server that never answers", func(t *testing.T) {
		listener, _ := net.Listen("tcp", "127.0.0.1:0")
		defer listener.Close()
		go func() {
			conn, err := listener.Accept()
			if err == nil {
				defer conn.Close()
				time.Sleep(5 * time.Second)
			}
		}()
		start := time.Now()

		err := sendMail(listener.Addr().String(), "127.0.0.1", false, nil, "a@example.com", "b@example.com", []byte("x"), 200*time.Millisecond)

		if err == nil || time.Since(start) > 2*time.Second {
			t.Errorf("err = %v after %s, want an error within the timeout", err, time.Since(start))
		}
	})

	t.Run("starts with TLS on the port of implicit TLS", func(t *testing.T) {
		listener, _ := net.Listen("tcp", "127.0.0.1:0")
		defer listener.Close()
		first := make(chan byte, 1)
		go func() {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			b := make([]byte, 1)
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			if _, err := conn.Read(b); err == nil {
				first <- b[0]
			}
		}()

		sendMail(listener.Addr().String(), "127.0.0.1", true, nil, "a@example.com", "b@example.com", []byte("x"), time.Second)

		// 0x16 opens a TLS handshake record: the client spoke TLS first,
		// instead of waiting for a greeting that never comes on port 465.
		select {
		case b := <-first:
			if b != 0x16 {
				t.Errorf("first byte %#x, want a TLS handshake (0x16)", b)
			}
		case <-time.After(3 * time.Second):
			t.Error("the client sent nothing")
		}
	})
}

func TestImplicitTLS(t *testing.T) {
	if !implicitTLS(465) || implicitTLS(587) || implicitTLS(25) {
		t.Error("want implicit TLS on 465 only")
	}
}
