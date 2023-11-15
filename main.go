package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	_ "github.com/emersion/go-message/charset"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
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
    gin.SetMode(gin.ReleaseMode)
    router := gin.New()
    router.GET("/processEmail", fetchEmailsHandler)

	log.Println("Listening and serving HTTP on localhost:8080")
    router.Run("localhost:8080")
}

func fetchEmailsHandler(_c *gin.Context) {
	var imapserver string
	var mailPlatform string
	var useremail string
	var password string
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("Enter mail platform (Gmail/Outlook): ")
	// fmt.Scan(&mailPlatform)
	scanner.Scan()
	mailPlatform = scanner.Text()

	if mailPlatform == "Gmail" {
		imapserver = viperEnvVariable("GMAIL_IMAP_SERVER")
	} else if mailPlatform == "Outlook" {
		imapserver = viperEnvVariable("OUTLOOK_IMAP_SERVER")
	}

	fmt.Print("Enter your email address: ")
	scanner.Scan()
	useremail = scanner.Text()

	fmt.Print("Enter your password: ")
	scanner.Scan()
	password = scanner.Text()
	
	log.Println("Processing emails...")

	log.Println("Connecting to server...")

	// Connect to server
	c, err := imapclient.DialTLS(imapserver, nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Connected")

	// Don't forget to logout
	defer c.Close()

	// Login
	if err := c.Login(useremail, password).Wait(); err != nil {
		log.Fatalf("failed to login: %v", err)
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
	var sentMessageEntries []MessageEntry
	if strings.Contains(imapserver, "gmail") {
		sentMessageEntries = processEmails(c, "[Gmail]/Sent Mail", true)
	} else if strings.Contains(imapserver, "outlook") {
		sentMessageEntries = processEmails(c, "Sent", true)
	}
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
			if entry.To == useremail {
				name = entry.FromName
				receivedEmails++
			} else if entry.From == useremail || entry.isSent {
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
					if entry.To == useremail {
						databaseEntries[i].ReceivedEmails++
						if databaseEntries[i].Name == "" {
							databaseEntries[i].Name = entry.FromName
						}
					} else if entry.From == useremail{
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

func processEmails(c *imapclient.Client, mailboxName string, isSent bool) []MessageEntry {
	log.Println("Fetching " + mailboxName + " messages...")
	mbox, err := c.Select(mailboxName, nil).Wait()
	if err != nil {
		log.Fatalf("failed to select INBOX: %v", err)
	}

	// Get the whole message body
	seqSet := imap.SeqSetRange(1, mbox.NumMessages)
	fetchOptions := &imap.FetchOptions{
		Flags:    true,
		Envelope: true,
		BodySection: []*imap.FetchItemBodySection{
			{Specifier: imap.PartSpecifierHeader},
		},
	}
	messages, err := c.Fetch(seqSet, fetchOptions).Collect()
	if err != nil {
		log.Fatalf("failed to fetch first message in INBOX: %v", err)
	}
	var messageEntries []MessageEntry

	// msg := <-messages
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]

		if msg == nil {
			log.Fatal("Server didn't returned message")
		}

		date := msg.Envelope.Date
		
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

		from := msg.Envelope.From
		if err != nil {
			log.Fatal(err)
		}
		to := msg.Envelope.To
		if err != nil {
			log.Fatal(err)
		}

		// subject, err := header.Subject();
		// if err != nil {
		// 	log.Fatal(err)
		// }
		entry := MessageEntry{
			Date:    	 date,
			From:    	 from[0].Mailbox + "@" + from[0].Host,
			FromName:    from[0].Name,
			To:      	 to[0].Mailbox + "@" + to[0].Host,
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