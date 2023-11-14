package main

import (
	"encoding/csv"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	_ "github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

var (
	SERVERADDRESS = viperEnvVariable("OUTLOOK_IMAP_SERVER")
	USEREMAIL      = viperEnvVariable("OUTLOOK_EMAIL_ADDRESS")
	PASSWORD      = viperEnvVariable("OUTLOOK_PASSWORD")
)

var MONTH_LIMIT = viperEnvVariable("MONTH_LIMIT")

type MessageEntry struct {
	From	string
	FromName	string
	To      string
	ToName string
	Date    time.Time
	isSent bool
}

type DatabaseEntry struct {
	Name    string
	Email      string
	SentEmails int
	ReceivedEmails int
	LastDate    time.Time
	// Date    time.Time
}

func viperEnvVariable(key string) string {
	viper.SetConfigFile(".env")
	err := viper.ReadInConfig()
  
	if err != nil {
	  log.Fatalf("Error while reading config file %s", err)
	}
	value, ok := viper.Get(key).(string)
	if !ok {
	  log.Fatalf("Invalid type assertion")
	}
	return value
  }

func main() {
    router := gin.Default()
    router.GET("/processEmail", fetchEmailsHandler)

    router.Run("localhost:8080")
}

func fetchEmailsHandler(_c *gin.Context) {
	log.Println("Processing emails...")

	log.Println("Connecting to server...")

	// Connect to server
	c, err := client.DialTLS(SERVERADDRESS, nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Connected")

	// Don't forget to logout
	defer c.Logout()

	// Login
	if err := c.Login(USEREMAIL, PASSWORD); err != nil {
		log.Fatal(err)
	}
	log.Println("Logged in")


	// // TODO: List mailboxes
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
	var receivedMessageEntries = processEmails(c, "INBOX", false)
	// var sentMessageEntries = processEmails(c, "[Gmail]/Sent Mail", true)
	var sentMessageEntries = processEmails(c, "Sent", true)
	var messageEntries []MessageEntry
	messageEntries = append(messageEntries, sentMessageEntries...)
	messageEntries = append(messageEntries, receivedMessageEntries...)
	var databaseEntries []DatabaseEntry

	for _, entry := range messageEntries {
		email := ""
		if(entry.isSent) {
			email = entry.To
		} else {
			email = entry.From
		}
		if !emailExistsInDatabase(email, databaseEntries) {
			sentEmails := 0
			receivedEmails := 0
			name := ""
			if entry.To == USEREMAIL {
				name = entry.FromName
				receivedEmails++
			} else if entry.From == USEREMAIL || entry.isSent {
				name = entry.ToName
				sentEmails++
			} else {
				receivedEmails++
			}

			databaseEntry := DatabaseEntry{
				Name:            name,
				Email:           email,
				SentEmails:	     sentEmails,
				ReceivedEmails:	 receivedEmails,
				LastDate:		 entry.Date,
				// Date:			 time.Now(),
			}
			databaseEntries = append(databaseEntries, databaseEntry)
		} else {
			for i := range databaseEntries {
				if databaseEntries[i].Email == entry.From {
					if entry.To == USEREMAIL {
						databaseEntries[i].ReceivedEmails++
						if databaseEntries[i].Name == "" {
							databaseEntries[i].Name = entry.FromName
						}
					} else if entry.From == USEREMAIL{
						databaseEntries[i].SentEmails++
					} else{
						databaseEntries[i].ReceivedEmails++
					}
	
					if entry.Date.After(databaseEntries[i].LastDate) {
						databaseEntries[i].LastDate = entry.Date
					}
				}
			}
		}
	}

	if err := exportToCSV(databaseEntries); err != nil {
		log.Fatal(err)
	}

	
	log.Println("Done!")
}

func processEmails(c *client.Client, mailboxName string, isSent bool) []MessageEntry {
	mbox, err := c.Select(mailboxName, false)
	if err != nil {
		log.Fatal(err)
	}
	if mbox.Messages == 0 {
		log.Fatal("No sent message")
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddRange(1, mbox.Messages)

	// Get the whole message body
	var section imap.BodySectionName
	items := []imap.FetchItem{section.FetchItem()}
	messages := make(chan *imap.Message, 10)
	go func() {
		log.Println("Fetching " + mailboxName + " messages...")
		if err := c.Fetch(seqSet, items, messages); err != nil {
			log.Fatal(err)
		}
	}()
	var messageSlice []*imap.Message
	for msg := range messages {
		messageSlice = append(messageSlice, msg)
	}
	var messageEntries []MessageEntry

	// msg := <-messages
	for i := len(messageSlice) - 1; i >= 0; i-- {
		msg := messageSlice[i]

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

		header := mr.Header

		date, err := header.Date();
		
		if err != nil {
			log.Fatal(err)
		}
		difference := time.Now().Sub(date)
		months := difference.Hours() / (24 * 365.25 / 12)
		month_limit, err := strconv.ParseFloat(MONTH_LIMIT, 64)
		if err != nil {
			log.Fatal("Error:", err)
		}
		if months >= month_limit {
			break
		}

		from, err := header.AddressList("From");
		if err != nil {
			log.Fatal(err)
		}
		to, err := header.AddressList("To");
		if err != nil {
			log.Fatal(err)
		}

		// subject, err := header.Subject();
		// if err != nil {
		// 	log.Fatal(err)
		// }
		entry := MessageEntry{
			Date:    	 date,
			From:    	 from[0].Address,
			FromName:    from[0].Name,
			To:      	 to[0].Address,
			ToName:      to[0].Name,
			isSent:   	 isSent,
		}

		messageEntries = append(messageEntries, entry)

		//TODO: Content part
		// // Process each message's part
		// for {
		// 	p, err := mr.NextPart()
		// 	if err == io.EOF {
		// 		break
		// 	} else if err != nil {
		// 		log.Fatal(err)
		// 	}
	
		// 	switch h := p.Header.(type) {
		// 	case *mail.InlineHeader:
		// 		// This is the message's text (can be plain-text or HTML)
		// 		contentType, _, err := h.ContentType()
		// 		if err != nil {
		// 			log.Fatal(err)
		// 		}
		// 		if contentType == "text/plain" {
		// 			// This is the message's plain text
		// 			b, _ := io.ReadAll(p.Body)
		// 			// log.Println("Got plain text: %v", string(b))
		// 			entry := MessageEntry{
		// 				Date:    date,
		// 				From:    from[0].Address,
		// 				To:      to[0].Address,
		// 				Content: string(b),
		// 			}
		// 			messageEntries = append(messageEntries, entry)
		// 		}
		// 		// b, _ := io.ReadAll(p.Body)
		// 		// log.Println("Got text: %v", string(b))
		// 	case *mail.AttachmentHeader:
		// 		// This is an attachment
		// 		filename, _ := h.Filename()
		// 		log.Println("Got attachment: %v", filename)
		// 	}
		// }
	}
	return messageEntries
}

func emailExistsInDatabase(email string, entries []DatabaseEntry) bool {
	for _, obj := range entries {
        if obj.Email == email {
            return true
        }
    }
    return false
}

func exportToCSV(entries []DatabaseEntry) error {
	file, err := os.Create("messages.csv")
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Name", "Email", "SentEmails", "ReceivedEmails", "LastDate"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write data
	for _, entry := range entries {
		row := []string{
			entry.Name,
			entry.Email,
			strconv.Itoa(entry.SentEmails),
			strconv.Itoa(entry.ReceivedEmails),
			entry.LastDate.Format("2006-01-02 15:04:05"),
			// entry.Date.Format("2006-01-02 15:04:05"),
			// entry.Content,
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}