package smtp

import (
	"bytes"
	"time"

	"talent-rodeo/assets"
	"talent-rodeo/internal/funcs"

	"github.com/wneessen/go-mail"

	htmlTemplate "html/template"
	textTemplate "text/template"
)

const defaultTimeout = 10 * time.Second

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

type Mailer struct {
	client mail.Client
	from   string
}

func NewMailer(host string, port int, username, password, from string) (*Mailer, error) {
	client, err := mail.NewClient(host, mail.WithTimeout(defaultTimeout), mail.WithSMTPAuth(mail.SMTPAuthLogin), mail.WithPort(port), mail.WithUsername(username), mail.WithPassword(password))
	if err != nil {
		return nil, err
	}

	mailer := &Mailer{
		client: *client,
		from:   from,
	}

	return mailer, nil
}

func (m *Mailer) Send(recipient, subject, plainBody, htmlBody string) error {
	msg := mail.NewMsg()
	if err := msg.To(recipient); err != nil {
		return err
	}
	if err := msg.From(m.from); err != nil {
		return err
	}

	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, plainBody)
	if htmlBody != "" {
		msg.AddAlternativeString(mail.TypeTextHTML, htmlBody)
	}

	for i := 1; i <= 3; i++ {
		err := m.client.DialAndSend(msg)
		if nil == err {
			return nil
		}
		if i != 3 {
			time.Sleep(2 * time.Second)
		}
	}
	return nil
}

func (m *Mailer) SendFromTemplate(recipient string, data any, patterns ...string) error {
	for i := range patterns {
		patterns[i] = "emails/" + patterns[i]
	}

	ts, err := textTemplate.New("").Funcs(funcs.TemplateFuncs).ParseFS(assets.EmbeddedFiles, patterns...)
	if err != nil {
		return err
	}

	subjectBuf := new(bytes.Buffer)
	err = ts.ExecuteTemplate(subjectBuf, "subject", data)
	if err != nil {
		return err
	}

	plainBodyBuf := new(bytes.Buffer)
	err = ts.ExecuteTemplate(plainBodyBuf, "plainBody", data)
	if err != nil {
		return err
	}

	var htmlBodyBuf bytes.Buffer
	if ts.Lookup("htmlBody") != nil {
		// We must re-parse for html/template to handle HTML correctly.
		htmlTS, err := htmlTemplate.New("").Funcs(funcs.TemplateFuncs).ParseFS(assets.EmbeddedFiles, patterns...)
		if err != nil {
			return err
		}
		err = htmlTS.ExecuteTemplate(&htmlBodyBuf, "htmlBody", data)
		if err != nil {
			return err
		}
	}

	return m.Send(recipient, subjectBuf.String(), plainBodyBuf.String(), htmlBodyBuf.String())
}
