package main

import (
	"io"
	"log"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
)

const (
	ServerAddress = "www@example.com"
	Username      = "example@gmail.com"
	Password      = "example app password"
)

func main() {
	log.Println("Connecting to server...")

	// Connect to server
	c, err := client.DialTLS(ServerAddress, nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Connected")

	// Don't forget to logout
	defer c.Logout()

	// Login
	if err := c.Login(Username, Password); err != nil {
		log.Fatal(err)
	}
	log.Println("Logged in")
	mbox, err := c.Select("INBOX", false)
	if err != nil {
		log.Fatal(err)
	}

	// Get the last message
	if mbox.Messages == 0 {
		log.Fatal("No message in mailbox")
	}
	seqSet := new(imap.SeqSet)
	seqSet.AddNum(mbox.Messages)
	// seqSet.AddRange(1, mbox.Messages)

	// Get the whole message body
	var section imap.BodySectionName
	items := []imap.FetchItem{section.FetchItem()}

	messages := make(chan *imap.Message, 10)
	go func() {
		if err := c.Fetch(seqSet, items, messages); err != nil {
			log.Fatal(err)
		}
	}()
	
	msg := <-messages
	// for msg := range messages {
	if msg == nil {
		log.Fatal("Server didn't returned message")
	}

	r := msg.GetBody(&section)
	if r == nil {
		log.Fatal("Server didn't returned message body")
	}

	// Create a new mail reader
	mr, err := mail.CreateReader(r)
	if err != nil {
		log.Fatal(err)
	}

	// Print some info about the message
	header := mr.Header
	if date, err := header.Date(); err == nil {
		log.Println("Date:", date)
	}
	if from, err := header.AddressList("From"); err == nil {
		log.Println("From:", from)
	}
	if to, err := header.AddressList("To"); err == nil {
		log.Println("To:", to)
	}
	if subject, err := header.Subject(); err == nil {
		log.Println("Subject:", subject)
	}

	// Process each message's part
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		}

		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			// This is the message's text (can be plain-text or HTML)
			b, _ := io.ReadAll(p.Body)
			log.Println("Got text: %v", string(b))
		case *mail.AttachmentHeader:
			// This is an attachment
			filename, _ := h.Filename()
			log.Println("Got attachment: %v", filename)
		}
	}
	// }
	
	// List mailboxes
	// mailboxes := make(chan *imap.MailboxInfo, 10)
	// done := make(chan error, 1)
	// go func () {
	// 	done <- c.List("", "*", mailboxes)
	// }()

	// log.Println("Mailboxes:")
	// for m := range mailboxes {
	// 	log.Println("* " + m.Name)
	// }

	// if err := <-done; err != nil {
	// 	log.Fatal(err)
	// }

	// // Select INBOX
	// mbox, err := c.Select("INBOX", false)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// log.Println("Flags for INBOX:", mbox.Flags)
	// log.Println("Number of messages INBOX:", mbox.Messages)

	// // Get the last 4 messages
	// from := uint32(1)
	// to := mbox.Messages
	// if mbox.Messages > 3 {
	// 	// We're using unsigned integers here, only subtract if the result is > 0
	// 	from = mbox.Messages - 3
	// }
	// seqset := new(imap.SeqSet)
	// seqset.AddRange(from, to)

	// messages := make(chan *imap.Message, 10)
	// done = make(chan error, 1)
	// go func() {
	// 	done <- c.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope}, messages)
	// }()

	// log.Println("Last 4 messages:")
	// for msg := range messages {
	// 	log.Println("*Subject: ", msg.Envelope.Subject)
	// 	log.Println("*Date: ", msg.Envelope.From[0].Address())
	// 	log.Println("*Date: ", msg.Envelope.To[0].Address())
	// 	log.Println("*Date: ", msg.Envelope.Date)
	// }

	// if err := <-done; err != nil {
	// 	log.Fatal(err)
	// }

	// log.Println("Done!")
}