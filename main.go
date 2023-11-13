package main

import (
	"io"
	"log"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
)

const (
	ServerAddress = "www.example.com"
	Username      = "mail@example.com"
	Password      = "password"
)

type MessageEntry struct {
	From    string
	To      string
	Date    time.Time
	Content string
}

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
	// seqSet.AddNum(mbox.Messages)
	seqSet.AddRange(mbox.Messages - 2, mbox.Messages)

	// Get the whole message body
	var section imap.BodySectionName
	items := []imap.FetchItem{section.FetchItem()}

	messages := make(chan *imap.Message, 10)
	go func() {
		if err := c.Fetch(seqSet, items, messages); err != nil {
			log.Fatal(err)
		}
	}()
	
	var databaseEntry []MessageEntry

	// msg := <-messages
	for msg := range messages {
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

		log.Println("================================> message read: ")
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
				contentType, _, err := h.ContentType()
				if err != nil {
					log.Fatal(err)
				}
				if contentType == "text/plain" {
					// This is the message's plain text
					b, _ := io.ReadAll(p.Body)
					log.Println("Got plain text: %v", string(b))
				}
				// b, _ := io.ReadAll(p.Body)
				// log.Println("Got text: %v", string(b))
			case *mail.AttachmentHeader:
				// This is an attachment
				filename, _ := h.Filename()
				log.Println("Got attachment: %v", filename)
			}
		}
	}
	// }
	
	log.Println("databaseEntry", databaseEntry)
	log.Println("Done!")
}