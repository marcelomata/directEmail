// Package directEmail support direct send email from selected interface include SOCKS5 proxy server
package directEmail

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"golang.org/x/net/proxy"
	"net"
	"net/smtp"
	"net/url"
	"strings"
)

// A Email contains options for send email.
type Email struct {
	// Ip contains local IP address wich use for send email
	// if blank use default interface
	// if use socks5 proxy "socks://123.124.125.126:8080"
	// and with auth "socks://user:password@123.124.125.126:8080"
	Ip string
	// Host is host name
	// if blank use DNS resolv for field fill
	Host string
	// Port SMTP server port
	Port uint16
	// MapIp use for translate local IP to global if NAT
	// if use Socks server translate IP SOCKS server to real IP
	MapIp map[string]string
	// FromEmail sender email (Required)
	FromEmail string
	// FromName sender name
	FromName string
	// ToEmail recipient email (Required)
	ToEmail string
	// ToName recipient name
	ToName string
	// Subject email subject
	Subject string

	headers     []string
	textPlain   []byte
	textHTML    []byte
	attachments [][]byte
	raw         bytes.Buffer
	bodyLenght  int
}

const debugIs = false

// New returns a new Email instance for create and send email
func New() Email {
	return Email{Port: 25}
}

// SendThroughServer send email from SMTP server
func (self *Email) SendThroughServer(host string, port uint16, username, password string) error {
	self.Port = port

	dialFunc, err := self.dialFunction()
	debug("Dialer selected, now dial to server\n")

	conn, err := dialFunc("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", self.Port)))
	if err != nil {
		debug("Not connected\n")
		return err
	}
	debug("Connected\n")

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}

	auth := smtp.PlainAuth(
		"",
		username,
		password,
		host,
	)

	if err := client.Hello("localhost"); err != nil {
		return err
	}

	debug("Connected, now send email\n")
	return self.send(auth, host, client)
}

// Send email directly
func (self *Email) Send() error {
	var err error

	self.cleanEmail()

	server, err := self.domainFromEmail(self.ToEmail)
	if err != nil {
		return errors.New("553 Bad ToEmail")
	}

	c := &Connect{}
	c.hostname = self.Host
	c.mapIP = self.MapIp
	c.iface = self.Ip
	c.portSMTP = int(self.Port)
	client, err := c.newClient(server, true)
	if err != nil {
		return errors.New("421 Connect to server error: " + err.Error())
	}

	return self.send(nil, "", client)
}

func (self *Email) cleanEmail() {
	self.ToEmail = strings.TrimSpace(self.ToEmail)
	self.FromEmail = strings.TrimSpace(self.FromEmail)
}

var testHookStartTLS func(*tls.Config)

// Send sending email message
func (self *Email) send(auth smtp.Auth, host string, client *smtp.Client) error {
	var err error

	if auth != nil {
		if ok, _ := client.Extension("STARTTLS"); ok {
			config := &tls.Config{ServerName: host}
			if testHookStartTLS != nil {
				testHookStartTLS(config)
			}
			if err = client.StartTLS(config); err != nil {
				return err
			}
		}
		if auth != nil {
			if err = client.Auth(auth); err != nil {
				return err
			}
		}
	}

	if err := client.Mail(self.FromEmail); err != nil {
		return err
	}

	if err := client.Rcpt(self.ToEmail); err != nil {
		return err
	}

	w, err := client.Data()
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(w, self.raw.String())
	if err != nil {
		return err
	}

	err = w.Close()
	if err != nil {
		return err
	}

	return client.Quit()

}

type conn func(network, address string) (net.Conn, error)

func (self *Email) dialFunction() (conn, error) {
	var dialFunc conn

	if self.Ip == "" {
		iface := net.Dialer{}
		dialFunc = iface.Dial
		debug("Dial function is default network interface\n")
	} else {
		if strings.ToLower(self.Ip[0:8]) == "socks://" {
			u, err := url.Parse(self.Ip)
			if err != nil {
				return nil, errors.New(fmt.Sprintf("Error parse socks: %s", err.Error()))
			}
			var iface proxy.Dialer
			if u.User != nil {
				auth := proxy.Auth{}
				auth.User = u.User.Username()
				auth.Password, _ = u.User.Password()
				iface, err = proxy.SOCKS5("tcp", u.Host, &auth, proxy.FromEnvironment())
				if err != nil {
					return dialFunc, err
				}
			} else {
				iface, err = proxy.SOCKS5("tcp", u.Host, nil, proxy.FromEnvironment())
				if err != nil {
					return dialFunc, err
				}
			}
			self.Ip = u.Host
			dialFunc = iface.Dial
			debug("Dial function is socks proxy from ", self.Ip[8:], "\n")
		} else {
			addr := &net.TCPAddr{
				IP: net.ParseIP(self.Ip),
			}
			iface := net.Dialer{LocalAddr: addr}
			dialFunc = iface.Dial
			debug("Dial function is ", addr.String(), " network interface\n")
		}
	}

	return dialFunc, nil
}
